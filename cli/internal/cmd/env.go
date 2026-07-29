package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/chaitanyabankanhal/ai-gateway/cli/internal/auth"
	"github.com/chaitanyabankanhal/ai-gateway/cli/internal/client"
	cliconfig "github.com/chaitanyabankanhal/ai-gateway/cli/internal/config"
	"github.com/spf13/cobra"
)

// provider maps an SDK to the environment variables that point it at the
// gateway proxy. baseSuffix is appended to the configured proxy base URL.
type provider struct {
	keyVar     string
	baseVar    string
	baseSuffix string
	note       string
}

// providers is the registry of supported SDKs for `aigw env` / `aigw run`.
var providers = map[string]provider{
	"openai": {
		keyVar:     "OPENAI_API_KEY",
		baseVar:    "OPENAI_BASE_URL",
		baseSuffix: "/v1",
		note:       "OpenAI SDK (openai-python, openai-node, ...)",
	},
	"anthropic": {
		keyVar:     "ANTHROPIC_API_KEY",
		baseVar:    "ANTHROPIC_BASE_URL",
		baseSuffix: "", // SDK appends /v1/messages itself
		note:       "Anthropic SDK / Claude SDK",
	},
	"generic": {
		keyVar:     "AIGW_API_KEY",
		baseVar:    "AIGW_BASE_URL",
		baseSuffix: "/v1",
		note:       "any other / custom SDK",
	},
}

// providerNames returns the registry keys sorted, for help text and errors.
func providerNames() []string {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

var (
	envRefresh   bool
	envWriteFile string
)

// resolveProvider looks up a provider by name with a helpful error.
func resolveProvider(name string) (provider, error) {
	p, ok := providers[strings.ToLower(name)]
	if !ok {
		return provider{}, fmt.Errorf("unknown provider %q (supported: %s)",
			name, strings.Join(providerNames(), ", "))
	}
	return p, nil
}

// providerEnv resolves the env-var pair (key + base URL) for a provider:
// it ensures a cached gateway key exists and reads the proxy base URL.
func providerEnv(ctx context.Context, p provider, refresh bool) (key string, base string, err error) {
	cfg, err := cliconfig.Load()
	if err != nil {
		return "", "", err
	}
	proxy := strings.TrimRight(cfg.Proxy, "/")
	if proxy == "" {
		return "", "", fmt.Errorf("no proxy URL configured — set one with `aigw config set proxy <url>`")
	}

	tenant, err := resolveTenant()
	if err != nil {
		return "", "", err
	}
	c, _, err := authedClient()
	if err != nil {
		return "", "", err
	}
	apiKey, err := ensureProxyKey(ctx, c, tenant, refresh)
	if err != nil {
		return "", "", err
	}
	return apiKey, proxy + p.baseSuffix, nil
}

// ensureProxyKey returns a plaintext gateway key for the tenant, reusing the
// cached one when present. With refresh=true (or no cache) it mints a new key
// via the admin API and caches its plaintext for future invocations.
func ensureProxyKey(ctx context.Context, c *client.Client, tenant string, refresh bool) (string, error) {
	if !refresh {
		cached, err := auth.LoadProxyKey(tenant)
		switch {
		case err == nil:
			return cached.Key, nil
		case auth.ErrProxyKeyNotCached(err):
			// fall through to mint a new key
		default:
			return "", err
		}
	}

	created, err := c.CreateKey(ctx, tenant, client.CreateKeyRequest{Name: "aigw-cli"})
	if err != nil {
		return "", err
	}
	if err := auth.SaveProxyKey(tenant, auth.ProxyKey{
		ID:   created.ID,
		Name: created.Name,
		Key:  created.Key,
	}); err != nil {
		return "", fmt.Errorf("cache proxy key: %w", err)
	}
	return created.Key, nil
}

var envCmd = &cobra.Command{
	Use:   "env <provider>",
	Short: "Print shell exports pointing an SDK at the gateway",
	Long: "Print `export` statements that point an SDK's environment variables\n" +
		"at the AI Gateway proxy, using a cached gateway API key.\n\n" +
		"Load them into the current shell with:\n" +
		"  eval \"$(aigw env openai)\"\n\n" +
		"Providers: " + strings.Join(providerNames(), ", "),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := resolveProvider(args[0])
		if err != nil {
			return err
		}
		key, base, err := providerEnv(cmd.Context(), p, envRefresh)
		if err != nil {
			return err
		}
		if envWriteFile != "" {
			if err := writeEnvFile(envWriteFile, map[string]string{p.keyVar: key, p.baseVar: base}); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Wrote %s, %s to %s\n", p.keyVar, p.baseVar, envWriteFile)
			return nil
		}
		printf(cmd, "export %s=%s\n", p.keyVar, shellQuote(key))
		printf(cmd, "export %s=%s\n", p.baseVar, shellQuote(base))
		return nil
	},
}

var runCmd = &cobra.Command{
	Use:   "run <provider> -- <command> [args...]",
	Short: "Run a command with an SDK's env vars pointed at the gateway",
	Long: "Run a child command with the SDK environment variables set so its\n" +
		"requests flow through the AI Gateway proxy. The variables are scoped\n" +
		"to the child process only — the current shell is untouched.\n\n" +
		"  aigw run openai -- python app.py\n" +
		"  aigw run anthropic -- claude\n\n" +
		"Providers: " + strings.Join(providerNames(), ", "),
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dash := cmd.ArgsLenAtDash()
		if dash != 1 {
			return fmt.Errorf("usage: aigw run <provider> -- <command> [args...]")
		}
		p, err := resolveProvider(args[0])
		if err != nil {
			return err
		}
		childArgs := args[dash:]
		if len(childArgs) == 0 {
			return fmt.Errorf("no command given after `--`")
		}

		key, base, err := providerEnv(cmd.Context(), p, envRefresh)
		if err != nil {
			return err
		}

		// #nosec G204 -- childArgs is what the operator typed after `--`; running
		// their command with provider env vars injected is the whole point of
		// `aigw run`. There is no privilege boundary to cross here.
		child := exec.CommandContext(cmd.Context(), childArgs[0], childArgs[1:]...)
		child.Env = append(os.Environ(), p.keyVar+"="+key, p.baseVar+"="+base)
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := child.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				os.Exit(exit.ExitCode())
			}
			return fmt.Errorf("run %s: %w", childArgs[0], err)
		}
		return nil
	},
}

// shellQuote single-quotes a value for safe `eval` in POSIX shells.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// writeEnvFile upserts the given KEY=value pairs into a dotenv-style file,
// replacing existing lines for those keys and appending the rest.
// path comes from the operator's own --write flag, so it is deliberately
// unrestricted — the CLI runs with their privileges and writes where they ask.
func writeEnvFile(path string, vars map[string]string) error {
	var lines []string
	// #nosec G304,G703 -- operator-supplied destination, see above.
	if existing, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(existing), "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	remaining := make(map[string]string, len(vars))
	for k, v := range vars {
		remaining[k] = v
	}
	out := make([]string, 0, len(lines)+len(vars))
	for _, line := range lines {
		key, _, found := strings.Cut(line, "=")
		if v, ok := remaining[strings.TrimSpace(key)]; found && ok {
			out = append(out, key+"="+v)
			delete(remaining, strings.TrimSpace(key))
			continue
		}
		out = append(out, line)
	}
	for _, k := range sortedKeys(remaining) {
		out = append(out, k+"="+remaining[k])
	}
	// #nosec G306,G703 -- operator-supplied destination; 0600 because the file
	// holds a gateway API key.
	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o600)
}

// sortedKeys returns a map's keys in sorted order, for deterministic output.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	envCmd.Flags().BoolVar(&envRefresh, "refresh", false, "mint a fresh gateway key instead of reusing the cached one")
	envCmd.Flags().StringVar(&envWriteFile, "write", "", "write the vars to a dotenv file instead of printing exports")
	runCmd.Flags().BoolVar(&envRefresh, "refresh", false, "mint a fresh gateway key instead of reusing the cached one")
}

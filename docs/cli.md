# CLI (`aigw`)

`aigw` is the governance and cost-control CLI for AI Gateway. It exists to solve
one problem:

> Developers, QA, and agents all need LLM access. The usual answer — copy the
> company provider key into everyone's dotfile — means one shared bill with no
> idea who spent what, no per-person cap, no policy, no revocation, and a
> long-lived secret sitting on every laptop.

`aigw` replaces the shared key with a **login**. A team member runs `aigw login`,
authenticates through your IdP, and the CLI mints a tenant-scoped gateway key on
demand and injects it into whatever environment variables their SDK already
reads — so their tool routes through the gateway with zero code changes.

The provider key never leaves the gateway. What the company gets in return:

- **Attribution** — every call is tagged to a tenant, agent, or user, so spend
  is broken down per team and per person instead of one opaque invoice.
- **Control** — revoke one person's access without rotating the shared key for
  everyone; policy and quotas apply centrally.
- **Blast radius** — no provider key on any developer machine to leak.

---

## Install

```sh
cd cli
make build      # -> bin/aigw
make install    # -> $GOBIN/aigw
```

---

## Configure

```sh
aigw config set endpoint http://localhost:8081   # admin API base URL
aigw config set proxy    http://localhost:8080   # proxy base URL SDKs hit
aigw config set tenant   1                       # default tenant for key commands
```

Stored in `~/.config/aigw/config.yaml`. Environment overrides: `AIGW_ENDPOINT`,
`AIGW_PROXY`, `AIGW_TENANT`.

---

## Log in

```sh
aigw login                          # pick a provider interactively
aigw login --provider google        # or microsoft
aigw whoami                         # current identity
aigw token                          # print the JWT, for curl/scripts
aigw logout                         # clear session + cached gateway key
```

`aigw login` starts a loopback HTTP listener, opens your browser at the
gateway's OIDC login URL with `cli_redirect` pointed back at that listener, and
captures the minted JWT once SSO completes. The gateway only accepts loopback
`cli_redirect` URLs, which prevents token exfiltration.

The session JWT is stored in the OS keyring (macOS Keychain, Windows Credential
Manager, libsecret). On systems without one it falls back to an encrypted file
backend at `~/.config/aigw/keyring`; set `AIGW_KEYRING_PASSPHRASE` for
non-interactive use such as CI.

::: warning SSO required
`aigw login` is SSO-only. At least one OIDC provider must be configured on the
gateway — see [SSO](/sso). Without SSO, mint keys from the admin UI's
[API Keys](/api-keys) page or the admin API.
:::

---

## Key injection

This is the part that matters for governance. `aigw env` and `aigw run` point an
SDK at the gateway by setting the variables that SDK already reads.

```sh
eval "$(aigw env openai)"          # mutate the current shell
aigw run anthropic -- claude       # scope the vars to one child process only
aigw env generic --write .env      # upsert vars into a dotenv file
aigw env openai --refresh          # mint a fresh key, replacing the cached one
```

| Provider | Variables set | Base URL |
|---|---|---|
| `openai` | `OPENAI_API_KEY`, `OPENAI_BASE_URL` | `<proxy>/v1` |
| `anthropic` | `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL` | `<proxy>` |
| `generic` | `AIGW_API_KEY`, `AIGW_BASE_URL` | `<proxy>/v1` |

On first use the CLI mints a gateway key named `aigw-cli` and caches its
plaintext in the OS keyring, per tenant, reusing it on later runs. `--refresh`
forces a new one. `aigw logout` drops it.

Prefer `aigw run` over `eval "$(aigw env …)"` where you can: the credentials
live only in that child process's environment, not in your shell history or
every subsequent command.

### What the operator gets

| Property | How |
|---|---|
| Attribution | Keys are tenant-scoped, and optionally agent- or user-scoped |
| Revocation | `aigw keys revoke`, or the admin UI — the Redis cache expires within `API_KEY_CACHE_TTL_SEC` |
| Policy | Every call passes the [guardrails](/guardrails) engine before reaching a provider |
| Visibility | Every call is traced — see [Traces](/traces) |
| Blast radius | Provider keys live only on the gateway, never on developer machines |

---

## Tenants and keys

```sh
aigw tenants list [--json]

aigw keys list   [--tenant <id>] [--json]
aigw keys create [--tenant <id>] --name <name> [--agent <id>] [--user <id>]
aigw keys revoke [--tenant <id>] --id <keyID>
```

`--tenant` defaults to `aigw config set tenant`. Created keys print their
plaintext **once** — store it immediately. Listing never returns plaintext, only
the prefix and suffix.

Scope keys as narrowly as the work allows: an agent-scoped key cannot
impersonate another agent, because the gateway ignores a client-supplied
`X-Agent-Id` when the key already names one.

---

## In CI

```sh
export AIGW_ENDPOINT=https://gateway.internal:8081
export AIGW_TENANT=3
export AIGW_KEYRING_PASSPHRASE=$CI_KEYRING_PASSPHRASE

aigw run openai -- pytest tests/
```

Interactive SSO does not work in CI. Either provision a long-lived key for the
job through the admin API and set `OPENAI_API_KEY` / `OPENAI_BASE_URL` directly,
or run the CLI with a pre-seeded keyring.

---

## Command reference

| Command | Purpose |
|---|---|
| `aigw config get \| set <key> [value]` | Read or write `endpoint`, `proxy`, `tenant` |
| `aigw login [--provider google\|microsoft]` | Browser SSO login |
| `aigw whoami` | Show the logged-in identity |
| `aigw token` | Print the stored JWT |
| `aigw logout` | Clear the session and cached key |
| `aigw tenants list` | List visible tenants |
| `aigw keys list \| create \| revoke` | Manage tenant API keys |
| `aigw env <provider>` | Print shell exports pointing an SDK at the gateway |
| `aigw run <provider> -- <cmd>` | Run a command with those variables set |
| `aigw version` | Print the CLI version |

---

## See also

- [Quickstart](/quickstart) — stand up a gateway
- [Integration Guide](/integration) — SDK, IDE, and header details
- [API Keys](/api-keys) — scoping and rotation in the admin UI
- [SSO](/sso) — configure Google or Microsoft login

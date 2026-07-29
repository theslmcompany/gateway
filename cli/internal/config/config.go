// Package config manages the aigw CLI's on-disk configuration
// (~/.config/aigw/config.yaml) with AIGW_* environment-variable overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// DefaultEndpoint is the admin API base URL assumed when none is configured.
const DefaultEndpoint = "http://localhost:8081"

// DefaultProxy is the proxy (LLM gateway) base URL assumed when none is
// configured. SDKs send their requests here, not to the admin API.
const DefaultProxy = "http://localhost:8080"

// settableKeys are the config keys `aigw config set` accepts.
var settableKeys = map[string]bool{
	"endpoint": true, // admin API base URL
	"proxy":    true, // proxy base URL SDKs point at
	"tenant":   true, // default tenant ID for key commands
}

// validKeys is the human-readable list of settable keys, for error messages.
const validKeys = "endpoint, proxy, tenant"

// Config is the resolved CLI configuration.
type Config struct {
	Endpoint string
	Proxy    string
	Tenant   string
}

// Dir returns the aigw config directory, creating nothing.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(base, "aigw"), nil
}

// FilePath returns the absolute path of config.yaml.
func FilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func newViper() (*viper.Viper, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)
	v.SetDefault("endpoint", DefaultEndpoint)
	v.SetDefault("proxy", DefaultProxy)
	v.SetDefault("tenant", "")
	v.SetEnvPrefix("AIGW") // AIGW_ENDPOINT, AIGW_PROXY, AIGW_TENANT
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}
	return v, nil
}

// Load resolves the configuration from file, env, and defaults.
func Load() (*Config, error) {
	v, err := newViper()
	if err != nil {
		return nil, err
	}
	return &Config{
		Endpoint: v.GetString("endpoint"),
		Proxy:    v.GetString("proxy"),
		Tenant:   v.GetString("tenant"),
	}, nil
}

// Get returns the resolved value of a single config key.
func Get(key string) (string, error) {
	v, err := newViper()
	if err != nil {
		return "", err
	}
	if !settableKeys[key] {
		return "", fmt.Errorf("unknown config key %q (valid: %s)", key, validKeys)
	}
	return v.GetString(key), nil
}

// Set writes a config key to config.yaml, creating the directory if needed.
func Set(key, value string) error {
	if !settableKeys[key] {
		return fmt.Errorf("unknown config key %q (valid: %s)", key, validKeys)
	}
	v, err := newViper()
	if err != nil {
		return err
	}
	v.Set(key, value)
	dir, err := Dir()
	if err != nil {
		return err
	}
	// 0700: this directory also holds the encrypted-file keyring backend, so it
	// must not be readable by other local users.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	path, _ := FilePath()
	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

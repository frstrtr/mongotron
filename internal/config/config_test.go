package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testYAML = `
database:
  mongodb:
    database: from_file
webhooks:
  subscriptionSecret: "${TEST_MT_SUB_SECRET}"
  porto:
    enabled: true
    baseUrl: "${TEST_MT_PORTO_URL:http://fallback:8000}"
    webhookPath: "/v1/webhooks/mongotron/transfer"
    operationPath: "/v1/webhooks/mongotron/operation"
    webhookSecret: "${TEST_MT_SECRET}"
    network: "tron-nile"
`

func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfigFileFromArgs(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"--config", "/etc/a.yaml"}, "/etc/a.yaml"},
		{[]string{"-config", "b.yaml"}, "b.yaml"},
		{[]string{"--verbose", "--config=/c.yaml"}, "/c.yaml"},
		{[]string{"-config=d.yaml"}, "d.yaml"},
		{[]string{"--config"}, ""},
		{[]string{"--configuration", "x"}, ""},
	}
	for _, c := range cases {
		if got := configFileFromArgs(c.args); got != c.want {
			t.Errorf("configFileFromArgs(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestLoadFileExpandsWebhookSecretsFromEnv(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "explicit.yaml", testYAML)
	t.Setenv("TEST_MT_SECRET", "porto-secret")
	t.Setenv("TEST_MT_SUB_SECRET", "sub-secret")
	t.Setenv("TEST_MT_PORTO_URL", "http://203.0.113.10:8000")

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.ConfigFile != path {
		t.Errorf("ConfigFile = %q, want %q", cfg.ConfigFile, path)
	}
	p := cfg.Webhooks.Porto
	if !p.Enabled || p.BaseURL != "http://203.0.113.10:8000" || p.WebhookSecret != "porto-secret" {
		t.Errorf("unexpected porto config: %+v", p)
	}
	if p.OperationPath != "/v1/webhooks/mongotron/operation" || p.Network != "tron-nile" {
		t.Errorf("unexpected porto paths: %+v", p)
	}
	if cfg.Webhooks.SubscriptionSecret != "sub-secret" {
		t.Errorf("SubscriptionSecret = %q", cfg.Webhooks.SubscriptionSecret)
	}
	if cfg.Database.MongoDB.Database != "from_file" {
		t.Errorf("database = %q, want from_file", cfg.Database.MongoDB.Database)
	}
}

func TestLoadFileUnsetSecretIsEmptyAndDefaultApplies(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "explicit.yaml", testYAML)
	t.Setenv("TEST_MT_SECRET", "")
	t.Setenv("TEST_MT_PORTO_URL", "")

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Webhooks.Porto.WebhookSecret != "" {
		t.Errorf("unset secret must expand to empty, got %q", cfg.Webhooks.Porto.WebhookSecret)
	}
	if cfg.Webhooks.Porto.BaseURL != "http://fallback:8000" {
		t.Errorf("BaseURL = %q, want the ${VAR:default} default", cfg.Webhooks.Porto.BaseURL)
	}
}

func TestLoadFileNestedEnvOverride(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "explicit.yaml", testYAML)
	t.Setenv("MONGOTRON_DATABASE_MONGODB_DATABASE", "from_env")

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Database.MongoDB.Database != "from_env" {
		t.Errorf("database = %q, want from_env (MONGOTRON_DATABASE_MONGODB_DATABASE)", cfg.Database.MongoDB.Database)
	}
}

func TestLoadFileMissingExplicitFileFails(t *testing.T) {
	if _, err := LoadFile(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("an explicitly named config file that does not exist must be an error")
	}
}

func TestLoadFileEnvSelectsConfig(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "via-env.yaml", testYAML)
	t.Setenv("MONGOTRON_CONFIG", path)

	cfg, err := LoadFile("")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.ConfigFile != path {
		t.Errorf("ConfigFile = %q, want %q (from MONGOTRON_CONFIG)", cfg.ConfigFile, path)
	}
}

// The committed configs must take the Porto webhook secret and URL from the
// environment: a literal secret in a (public) repo is leaked, and a hard-coded
// host name silently breaks delivery when the API moves.
func TestRepoConfigsTakeWebhookSecretFromEnv(t *testing.T) {
	for _, name := range []string{"mongotron.yaml", "mongotron-oplx2.yaml"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "configs", name)
			t.Setenv("MONGOTRON_WEBHOOK_SECRET", "")
			t.Setenv("PORTO_API_URL", "")
			cfg, err := LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile(%s): %v", path, err)
			}
			if cfg.Webhooks.Porto.WebhookSecret != "" || cfg.Webhooks.Porto.BaseURL != "" {
				t.Fatalf("%s: porto secret/baseUrl must come only from the environment", name)
			}

			t.Setenv("MONGOTRON_WEBHOOK_SECRET", "from-env")
			t.Setenv("PORTO_API_URL", "http://203.0.113.10:8000")
			cfg, err = LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile(%s): %v", path, err)
			}
			if cfg.Webhooks.Porto.WebhookSecret != "from-env" || cfg.Webhooks.Porto.BaseURL != "http://203.0.113.10:8000" {
				t.Fatalf("%s: env values not applied: url=%q", name, cfg.Webhooks.Porto.BaseURL)
			}
			if cfg.Webhooks.Porto.OperationPath != "/v1/webhooks/mongotron/operation" {
				t.Fatalf("%s: operationPath = %q", name, cfg.Webhooks.Porto.OperationPath)
			}
			if cfg.Webhooks.Porto.UsagePath != "/v1/webhooks/mongotron/usage" || !cfg.Monitor.SharedScanner {
				t.Fatalf("%s: usagePath = %q, sharedScanner = %v", name, cfg.Webhooks.Porto.UsagePath, cfg.Monitor.SharedScanner)
			}
		})
	}
}

func TestMonitorAndUsageDefaults(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "explicit.yaml", testYAML)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	m := cfg.Monitor
	if !m.SharedScanner || m.PollInterval != 3*time.Second || m.CatchUpWindow != 1200 {
		t.Errorf("unexpected monitor defaults: %+v", m)
	}
	p := cfg.Webhooks.Porto
	if !p.UsageEnabled || p.UsagePath != "/v1/webhooks/mongotron/usage" {
		t.Errorf("unexpected usage defaults: enabled=%v path=%q", p.UsageEnabled, p.UsagePath)
	}
	if len(p.UsageWalletTypes) != 1 || p.UsageWalletTypes[0] != "keep_level" {
		t.Errorf("usageWalletTypes = %v, want [keep_level]", p.UsageWalletTypes)
	}
}

func TestMonitorAndUsageFromFileAndEnv(t *testing.T) {
	const yaml = `
monitor:
  sharedScanner: false
  pollInterval: 5s
  catchUpWindow: 600
webhooks:
  porto:
    enabled: true
    usageEnabled: false
    usagePath: "/custom/usage"
    usageWalletTypes: ["keep_level", "general"]
`
	path := writeConfig(t, t.TempDir(), "usage.yaml", yaml)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Monitor.SharedScanner || cfg.Monitor.PollInterval != 5*time.Second || cfg.Monitor.CatchUpWindow != 600 {
		t.Errorf("monitor from file: %+v", cfg.Monitor)
	}
	p := cfg.Webhooks.Porto
	if p.UsageEnabled || p.UsagePath != "/custom/usage" || len(p.UsageWalletTypes) != 2 || p.UsageWalletTypes[1] != "general" {
		t.Errorf("usage from file: %+v", p)
	}

	t.Setenv("MONGOTRON_MONITOR_SHAREDSCANNER", "true")
	t.Setenv("MONGOTRON_WEBHOOKS_PORTO_USAGEWALLETTYPES", "keep_level,invoice")
	cfg, err = LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if !cfg.Monitor.SharedScanner {
		t.Error("MONGOTRON_MONITOR_SHAREDSCANNER=true not applied")
	}
	if got := cfg.Webhooks.Porto.UsageWalletTypes; len(got) != 2 || got[0] != "keep_level" || got[1] != "invoice" {
		t.Errorf("usageWalletTypes from env = %v", got)
	}
}

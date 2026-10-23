package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for MongoTron
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Blockchain BlockchainConfig
	Monitor    MonitorConfig
	WorkerPool WorkerPoolConfig
	Logging    LoggingConfig
	Metrics    MetricsConfig
	Webhooks   WebhooksConfig
	API        APIConfig
	Security   SecurityConfig

	// ConfigFile is the configuration file actually loaded ("" = none, defaults/env only).
	ConfigFile string `mapstructure:"-"`
}

type ServerConfig struct {
	Host           string
	Port           int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	MaxHeaderBytes int
	TLS            TLSConfig
}

type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

type DatabaseConfig struct {
	MongoDB MongoDBConfig
}

type MongoDBConfig struct {
	URI         string
	Database    string
	Collections CollectionsConfig
}

type CollectionsConfig struct {
	Addresses    string
	Transactions string
	Events       string
	Webhooks     string
}

type BlockchainConfig struct {
	Tron TronConfig
}

type TronConfig struct {
	Node struct {
		Host   string
		Port   int
		UseTLS bool
	}
	Connection struct {
		Timeout         time.Duration
		MaxRetries      int
		BackoffInterval time.Duration
		KeepAlive       time.Duration
	}
}

// MonitorConfig controls how watched addresses read blocks.
type MonitorConfig struct {
	// SharedScanner (default true): one scanner fetches and parses each block once and
	// fans it out to every watched address. False: one polling loop per watched address
	// (one block fetch per block per address, the behaviour before the scanner).
	SharedScanner bool `yaml:"sharedScanner"`
	// PollInterval is how often the head is polled (default 3s).
	PollInterval time.Duration `yaml:"pollInterval"`
	// CatchUpWindow (blocks, default 1200): at startup, watched addresses at most this far
	// behind the newest one are caught up together by the scanner; one further behind
	// catches up on its own first.
	CatchUpWindow int64 `yaml:"catchUpWindow"`
}

type WorkerPoolConfig struct {
	Workers                 int
	QueueSize               int
	JobTimeout              time.Duration
	GracefulShutdownTimeout time.Duration
}

type LoggingConfig struct {
	Level  string
	Format string
	Output string
}

type MetricsConfig struct {
	Prometheus struct {
		Enabled bool
		Port    int
		Path    string
	}
}

type WebhooksConfig struct {
	Delivery struct {
		Timeout       time.Duration
		MaxConcurrent int
		RetryAttempts int
	}
	Porto PortoConfig // Porto API webhook configuration

	// SubscriptionSecret signs generic per-subscription webhook posts with the same
	// headers as the Porto client (X-MongoTron-Signature, -Signature-V2, -Timestamp).
	// Supports ${VAR} / ${VAR:default} expansion. Empty = per-subscription posts are unsigned.
	SubscriptionSecret string `yaml:"subscriptionSecret"`
}

// PortoConfig holds configuration for Porto API integration
type PortoConfig struct {
	Enabled       bool   `yaml:"enabled"`
	BaseURL       string `yaml:"baseUrl"`
	WebhookPath   string `yaml:"webhookPath"`   // Path for webhook endpoint (e.g., /v1/webhooks/mongotron/transfer)
	OperationPath string `yaml:"operationPath"` // Path for operation events (default /v1/webhooks/mongotron/operation)
	WebhookSecret string `yaml:"webhookSecret"`
	Network       string `yaml:"network"` // "tron-mainnet" or "tron-nile"

	// UsageEnabled (default true) turns energy_usage events on for the wallet types in
	// UsageWalletTypes.
	UsageEnabled bool `yaml:"usageEnabled"`
	// UsagePath is the path energy_usage events are posted to
	// (default /v1/webhooks/mongotron/usage).
	UsagePath string `yaml:"usagePath"`
	// UsageWalletTypes lists the wallet types whose watched addresses produce
	// energy_usage events (default ["keep_level"]; "*" = every wallet type).
	UsageWalletTypes []string `yaml:"usageWalletTypes"`
}

type APIConfig struct {
	REST struct {
		Enabled bool
		Prefix  string
	}
	WebSocket struct {
		Enabled bool
		Path    string
	}
	GRPC struct {
		Enabled bool
		Port    int
	}
}

type SecurityConfig struct {
	JWT struct {
		Secret     string
		Expiration string
		Issuer     string
	}
	RateLimiting struct {
		Enabled           bool
		RequestsPerMinute int
		Burst             int
	}
}

// Load loads configuration from file and environment variables.
//
// The file is the one named by a --config/-config command-line flag, else by the
// MONGOTRON_CONFIG environment variable, else the first "mongotron.{json,toml,yaml,yml}"
// found in ./configs or the working directory. An explicitly named file must exist.
func Load() (*Config, error) {
	return LoadFile(configFileFromArgs(os.Args[1:]))
}

// LoadFile loads configuration from path, or searches the default locations when
// path is empty (see Load).
func LoadFile(path string) (*Config, error) {
	v := viper.New()

	if path == "" {
		path = strings.TrimSpace(os.Getenv("MONGOTRON_CONFIG"))
	}
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("mongotron")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	// Environment variables: MONGOTRON_<SECTION>_<KEY>, e.g. MONGOTRON_WEBHOOKS_PORTO_BASEURL
	v.SetEnvPrefix("MONGOTRON")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok || path != "" {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Set defaults
	setDefaults(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	cfg.ConfigFile = v.ConfigFileUsed()

	// Expand environment variables in webhook config
	cfg.Webhooks.Porto.BaseURL = expandEnvVar(cfg.Webhooks.Porto.BaseURL)
	cfg.Webhooks.Porto.WebhookSecret = expandEnvVar(cfg.Webhooks.Porto.WebhookSecret)
	cfg.Webhooks.SubscriptionSecret = expandEnvVar(cfg.Webhooks.SubscriptionSecret)

	return &cfg, nil
}

// configFileFromArgs returns the value of a --config/-config flag
// ("--config path" or "--config=path"), or "" when absent.
func configFileFromArgs(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		for _, name := range []string{"--config", "-config"} {
			if a == name && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(a, name+"=") {
				return strings.TrimPrefix(a, name+"=")
			}
		}
	}
	return ""
}

// expandEnvVar expands environment variables in a string
// Supports ${VAR} and ${VAR:default} syntax
func expandEnvVar(s string) string {
	// Pattern matches ${VAR} or ${VAR:default}
	re := regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		envVar := parts[1]
		defaultVal := ""
		if len(parts) >= 3 {
			defaultVal = parts[2]
		}
		if val := os.Getenv(envVar); val != "" {
			return val
		}
		return defaultVal
	})
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "120s")

	// Monitor defaults: one shared block scanner for all watched addresses
	v.SetDefault("monitor.sharedScanner", true)
	v.SetDefault("monitor.pollInterval", "3s")
	v.SetDefault("monitor.catchUpWindow", 1200)

	// Porto energy_usage events (sent only for the listed wallet types)
	v.SetDefault("webhooks.porto.usageEnabled", true)
	v.SetDefault("webhooks.porto.usagePath", "/v1/webhooks/mongotron/usage")
	v.SetDefault("webhooks.porto.usageWalletTypes", []string{"keep_level"})

	// Database defaults
	v.SetDefault("database.mongodb.uri", "mongodb://localhost:27017")
	v.SetDefault("database.mongodb.database", "mongotron")

	// Worker pool defaults
	v.SetDefault("worker_pool.workers", 1000)
	v.SetDefault("worker_pool.queue_size", 100000)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
}

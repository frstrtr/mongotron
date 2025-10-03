package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for MongoTron
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Blockchain BlockchainConfig
	WorkerPool WorkerPoolConfig
	Logging    LoggingConfig
	Metrics    MetricsConfig
	Webhooks   WebhooksConfig
	API        APIConfig
	Security   SecurityConfig
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

type WorkerPoolConfig struct {
	Workers                  int
	QueueSize                int
	JobTimeout               time.Duration
	GracefulShutdownTimeout  time.Duration
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
		Enabled            bool
		RequestsPerMinute  int
		Burst              int
	}
}

// Load loads configuration from file and environment variables
func Load() (*Config, error) {
	v := viper.New()

	// Set config file paths
	v.SetConfigName("mongotron")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	// Environment variables
	v.AutomaticEnv()
	v.SetEnvPrefix("MONGOTRON")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Set defaults
	setDefaults(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "120s")

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

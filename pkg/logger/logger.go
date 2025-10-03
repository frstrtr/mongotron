package logger

import (
	"os"

	"github.com/frstrtr/mongotron/internal/config"
	"github.com/rs/zerolog"
)

// New creates a new structured logger
func New(cfg config.LoggingConfig) zerolog.Logger {
	// Set global log level
	switch cfg.Level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Create logger
	var logger zerolog.Logger

	if cfg.Format == "json" {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		output := zerolog.ConsoleWriter{Out: os.Stdout}
		logger = zerolog.New(output).With().Timestamp().Logger()
	}

	return logger
}

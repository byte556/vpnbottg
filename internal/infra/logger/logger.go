package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup — настраивает глобальный логгер.
func Setup(format, level string) {
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
	zerolog.TimeFieldFormat = time.RFC3339

	var w io.Writer = os.Stdout
	if format == "console" {
		w = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"}
	}

	log.Logger = zerolog.New(w).With().Timestamp().Logger()
}

// L — короткий хелпер: logger.L().Info().Msg("...")
func L() *zerolog.Logger { return &log.Logger }

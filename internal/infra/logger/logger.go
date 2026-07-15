package logger

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// L — короткий хелпер: logger.L().Info().Msg("...")
func L() *zerolog.Logger { return &log.Logger }

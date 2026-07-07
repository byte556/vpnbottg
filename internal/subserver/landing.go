package subserver

import (
	_ "embed"
	"html/template"
	"net/http"
	"strings"

	"vpnbottg/internal/infra/logger"
)

//go:embed landing.html
var landingHTML string

var landingTmpl = template.Must(template.New("landing").Parse(landingHTML))

// landingData — данные корневого лендинга.
type landingData struct {
	BotURL     template.URL // ссылка на Telegram-бота (пусто = кнопка скрыта)
	Support    string       // контакт поддержки, напр. "@byttte"
	SupportURL template.URL // tg://... или https://t.me/... для ссылки
}

// handleLanding отдаёт корневую страницу-лендинг на GET /.
func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	log := logger.L().With().Str("handler", "landing").Logger()

	data := landingData{
		Support: s.support,
	}
	if s.botURL != "" {
		data.BotURL = template.URL(s.botURL)
	}
	if s.support != "" {
		data.SupportURL = template.URL("https://t.me/" + strings.TrimPrefix(s.support, "@"))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := landingTmpl.Execute(w, data); err != nil {
		log.Error().Err(err).Msg("landing: template execute failed")
		return
	}
	log.Info().Msg("landing: served")
}

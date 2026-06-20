package yookassa

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	tele "gopkg.in/telebot.v3"
)

type notification struct {
	Type   string  `json:"type"`
	Event  string  `json:"event"`
	Object Payment `json:"object"`
}

func Webhook(bot *tele.Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var n notification
		if err := json.Unmarshal(body, &n); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if n.Event != "payment.succeeded" {
			w.WriteHeader(http.StatusOK)
			return
		}

		tgIDStr := n.Object.Metadata["tg_id"]
		if tgIDStr == "" {
			w.WriteHeader(http.StatusOK)
			return
		}

		var tgID int64
		fmt.Sscanf(tgIDStr, "%d", &tgID)

		// TODO: выдать VPN конфиг пользователю
		bot.Send(&tele.User{ID: tgID}, "✅ Оплата прошла! Ваш конфиг VPN готов.")

		w.WriteHeader(http.StatusOK)
	}
}

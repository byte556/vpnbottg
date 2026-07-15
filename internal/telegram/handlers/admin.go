package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/repository"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// AdminPromptInput sends the action-input prompt with a cancel button.
// Delegates from callbacks that set AdminAction before calling this.
func AdminPromptInput(promptKey string) tele.HandlerFunc {
	return func(c tele.Context) error {
		return editOrFresh(c, texts.T(promptKey), keyboard.AdminCancelKeyboard())
	}
}

func AdminMenu(c tele.Context) error {
	return c.EditOrSend(texts.T("admin.menu"), keyboard.AdminMenu())
}

func AdminStats(stats repository.Stats) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "AdminStats").Int64("admin_id", c.Sender().ID).Logger()

		s, err := stats.GetBotStats(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("GetBotStats failed")
			return c.EditOrSend(texts.T("admin.err_stats"), keyboard.AdminBackKeyboard())
		}

		text := texts.T("admin.stats", map[string]any{
			"TotalUsers":       s.TotalUsers,
			"ActiveSubs":       s.ActiveSubs,
			"TotalRevenue":     s.TotalRevenue,
			"TodayRevenue":     s.TodayRevenue,
			"OrphanedPayments": s.OrphanedPayments,
		})
		log.Info().Int("users", s.TotalUsers).Int("active_subs", s.ActiveSubs).Msg("AdminStats: ok")
		return c.EditOrSend(text, keyboard.AdminBackKeyboard())
	}
}

func AdminSubs(stats repository.Stats) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "AdminSubs").Int64("admin_id", c.Sender().ID).Logger()

		subs, err := stats.GetActiveSubs(context.Background(), 30)
		if err != nil {
			log.Error().Err(err).Msg("GetActiveSubs failed")
			return c.EditOrSend(texts.T("admin.err_subs"), keyboard.AdminBackKeyboard())
		}
		if len(subs) == 0 {
			return c.EditOrSend(texts.T("admin.no_subs"), keyboard.AdminBackKeyboard())
		}

		rows := make([]string, 0, len(subs))
		for _, s := range subs {
			name := formatName(s.Username, s.FirstName)
			until := time.Unix(s.ExpiresAt, 0).Format("02.01.06")
			rows = append(rows, fmt.Sprintf("%s · %d уст. · до %s", name, s.DeviceLimit, until))
		}
		text := texts.T("admin.subs", map[string]any{
			"Count": len(subs),
			"List":  strings.Join(rows, "\n"),
		})
		log.Info().Int("count", len(subs)).Msg("AdminSubs: ok")
		return c.EditOrSend(text, keyboard.AdminBackKeyboard())
	}
}

func AdminPayments(stats repository.Stats) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "AdminPayments").Int64("admin_id", c.Sender().ID).Logger()

		pays, err := stats.GetRecentPayments(context.Background(), 20)
		if err != nil {
			log.Error().Err(err).Msg("GetRecentPayments failed")
			return c.EditOrSend(texts.T("admin.err_payments"), keyboard.AdminBackKeyboard())
		}
		if len(pays) == 0 {
			return c.EditOrSend(texts.T("admin.no_payments"), keyboard.AdminBackKeyboard())
		}

		rows := make([]string, 0, len(pays))
		for _, p := range pays {
			name := formatName(p.Username, p.FirstName)
			rows = append(rows, fmt.Sprintf("%s · %d ₽ · %s назад", name, p.Amount, timeAgo(p.CreatedAt)))
		}
		text := texts.T("admin.payments", map[string]any{
			"List": strings.Join(rows, "\n"),
		})
		log.Info().Int("count", len(pays)).Msg("AdminPayments: ok")
		return c.EditOrSend(text, keyboard.AdminBackKeyboard())
	}
}

func AdminOrphaned(stats repository.Stats) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "AdminOrphaned").Logger()

		list, err := stats.GetOrphanedPayments(context.Background(), 20)
		if err != nil {
			log.Error().Err(err).Msg("GetOrphanedPayments failed")
			return c.EditOrSend(texts.T("admin.err_generic"), keyboard.AdminBackKeyboard())
		}
		if len(list) == 0 {
			return c.EditOrSend(texts.T("admin.no_orphaned"), keyboard.AdminBackKeyboard())
		}

		m := &tele.ReplyMarkup{}
		var rows []tele.Row
		textLines := make([]string, 0, len(list))
		for _, p := range list {
			name := formatName(p.Username, p.FirstName)
			when := timeAgo(p.CreatedAt)
			textLines = append(textLines, fmt.Sprintf("%s · %d ₽ · %s назад", name, p.Amount, when))
			btnLabel := fmt.Sprintf("🔄 %s", name)
			rows = append(rows, m.Row(m.Data(btnLabel, keyboard.AdminReissue, p.ProviderPaymentID)))
		}
		rows = append(rows, m.Row(m.Data(texts.T("admin.buttons.back"), keyboard.AdminBack)))
		m.Inline(rows...)

		text := texts.T("admin.orphaned", map[string]any{
			"Count": len(list),
			"List":  strings.Join(textLines, "\n"),
		})
		log.Info().Int("count", len(list)).Msg("AdminOrphaned: ok")
		return c.EditOrSend(text, m)
	}
}

// AdminCharts рендерит текстовый bar-chart выручки за последние 7 дней.
func AdminCharts(stats repository.Stats) tele.HandlerFunc {
	return func(c tele.Context) error {
		log := logger.L().With().Str("func", "AdminCharts").Logger()

		list, err := stats.GetDailyRevenue(context.Background(), 7)
		if err != nil {
			log.Error().Err(err).Msg("GetDailyRevenue failed")
			return c.EditOrSend(texts.T("admin.err_generic"), keyboard.AdminBackKeyboard())
		}
		if len(list) == 0 {
			return c.EditOrSend(texts.T("admin.charts.no_data"), keyboard.AdminBackKeyboard())
		}

		var maxRev int64
		for _, d := range list {
			if d.Revenue > maxRev {
				maxRev = d.Revenue
			}
		}

		const barWidth = 10
		lines := make([]string, 0, len(list))
		var total int64
		for _, d := range list {
			total += d.Revenue
			filled := 0
			if maxRev > 0 {
				filled = int(float64(d.Revenue) / float64(maxRev) * barWidth)
			}
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
			lines = append(lines, fmt.Sprintf("%s  %s  %d ₽", d.Day, bar, d.Revenue))
		}

		avg := total / int64(len(list))
		log.Info().Int64("total", total).Msg("AdminCharts: ok")
		return c.EditOrSend(texts.T("admin.charts.text", map[string]any{
			"Chart": strings.Join(lines, "\n"),
			"Total": total,
			"Avg":   avg,
		}), keyboard.AdminBackKeyboard())
	}
}

func formatName(username, firstName string) string {
	if username != "" {
		return "@" + username
	}
	return firstName
}

func timeAgo(ts int64) string {
	d := time.Now().Unix() - ts
	switch {
	case d < 60:
		return fmt.Sprintf("%d сек", d)
	case d < 3600:
		return fmt.Sprintf("%d мин", d/60)
	case d < 86400:
		return fmt.Sprintf("%d ч", d/3600)
	default:
		return fmt.Sprintf("%d дн", d/86400)
	}
}

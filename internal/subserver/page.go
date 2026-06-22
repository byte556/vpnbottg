package subserver

import (
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vpnbottg/internal/infra/logger"

	qrcode "github.com/skip2/go-qrcode"
)

//go:embed page.html
var pageHTML string

var pageTmpl = template.Must(template.New("sub_page").Parse(pageHTML))

const giB = 1024 * 1024 * 1024

// clientButton — одна кнопка «подключить в <клиент>».
// Список рендерится циклом, поэтому новых клиентов можно добавлять без правки вёрстки.
type clientButton struct {
	Label   string
	Href    template.URL
	Primary bool
}

// subStatus — состояние подписки для шапки страницы (best-effort).
type subStatus struct {
	Known      bool
	Active     bool
	ExpiryText string // напр. "22.07.2026"; пусто = бессрочно
	UsedGB     string // напр. "12.3"
	TotalGB    string // напр. "100"; пусто при безлимите
	Unlimited  bool
	BarWidth   template.CSS // "width:42%" для прогресс-бара
}

type pageData struct {
	SubURL  string // сырая ссылка подписки (текст + QR)
	Clients []clientButton
	QR      template.URL // data:image/png;base64,... с QR-кодом
	Status  subStatus
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	subID := r.PathValue("sub_id")
	log := logger.L().With().Str("handler", "page").Str("sub_id", subID).Logger()

	if subID == "" {
		http.NotFound(w, r)
		return
	}

	subURL := s.subURL(subID)

	png, err := qrcode.Encode(subURL, qrcode.Medium, 512)
	if err != nil {
		log.Error().Err(err).Msg("page: qr encode failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	qrURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	data := pageData{
		SubURL: subURL,
		QR:     template.URL(qrURI),
		Clients: []clientButton{
			{Label: "⚡ Подключить в Happ", Href: template.URL("happ://add/" + subURL), Primary: true},
		},
		Status: s.fetchStatus(r.Context(), subID),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTmpl.Execute(w, data); err != nil {
		log.Error().Err(err).Msg("page: template execute failed")
		return
	}
	log.Info().Msg("page: served")
}

// fetchStatus тянет из панели заголовок Subscription-Userinfo и собирает статус.
// Любая ошибка → Known=false, страница просто не показывает блок статуса.
func (s *Server) fetchStatus(ctx context.Context, subID string) subStatus {
	url := fmt.Sprintf(s.upstreamTemplate, subID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return subStatus{}
	}
	// UA подписочного клиента — чтобы панель отдала заголовок Subscription-Userinfo.
	req.Header.Set("User-Agent", "v2rayN/6.0")

	resp, err := s.http.Do(req)
	if err != nil {
		return subStatus{}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	up, down, total, expire, ok := parseUserinfo(resp.Header.Get("Subscription-Userinfo"))
	if !ok {
		return subStatus{}
	}

	st := subStatus{
		Known:     true,
		Active:    expire == 0 || expire > time.Now().Unix(),
		Unlimited: total <= 0,
		UsedGB:    formatGB(up + down),
	}
	if expire > 0 {
		st.ExpiryText = time.Unix(expire, 0).Format("02.01.2006")
	}
	if !st.Unlimited {
		st.TotalGB = formatGB(total)
		pct := int((float64(up+down) / float64(total)) * 100)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		st.BarWidth = template.CSS(fmt.Sprintf("width:%d%%", pct))
	}
	return st
}

// parseUserinfo разбирает заголовок вида
// "upload=0; download=10737418240; total=107374182400; expire=1718900000".
func parseUserinfo(h string) (up, down, total, expire int64, ok bool) {
	if h == "" {
		return 0, 0, 0, 0, false
	}
	for _, part := range strings.Split(h, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, _ := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		switch strings.TrimSpace(kv[0]) {
		case "upload":
			up = v
		case "download":
			down = v
		case "total":
			total = v
		case "expire":
			expire = v
		}
		ok = true
	}
	return up, down, total, expire, ok
}

// formatGB форматирует байты в ГБ с одним знаком после запятой.
func formatGB(bytes int64) string {
	return strconv.FormatFloat(float64(bytes)/giB, 'f', 1, 64)
}

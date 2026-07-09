package subserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseUserinfo(t *testing.T) {
	up, down, total, expire, ok := parseUserinfo("upload=10; download=20; total=100; expire=1718900000")
	if !ok || up != 10 || down != 20 || total != 100 || expire != 1718900000 {
		t.Errorf("parseUserinfo got up=%d down=%d total=%d expire=%d ok=%v", up, down, total, expire, ok)
	}
	if _, _, _, _, ok := parseUserinfo(""); ok {
		t.Error("parseUserinfo(\"\") should report ok=false")
	}
}

func TestHandlePage(t *testing.T) {
	// Заглушка панели: отдаёт Subscription-Userinfo, как реальный sub-сервис.
	// download=25 ГБ, total=100 ГБ → 25%; expire=4102444800 (2100 год) → активна.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo",
			"upload=0; download=26843545600; total=107374182400; expire=4102444800")
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte")

	req := httptest.NewRequest("GET", "/p/abc123", nil)
	req.SetPathValue("sub_id", "abc123")
	rec := httptest.NewRecorder()

	s.handlePage(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	wantSubURL := "https://vpn.example.com:8081/sub/abc123"
	checks := map[string]string{
		wantSubURL:                 "sub url",
		"happ://add/" + wantSubURL: "happ deep link",
		"data:image/png;base64,":   "embedded QR code",
		"Активна до":               "active status line",
		"25.0 ГБ · безлимит":       "traffic usage (unlimited)",
	}
	for substr, what := range checks {
		if !strings.Contains(body, substr) {
			t.Errorf("page missing %s (%q)", what, substr)
		}
	}
	// happ:// must survive html/template URL sanitization (no #ZgotmplZ).
	if strings.Contains(body, "ZgotmplZ") {
		t.Error("happ deep link or style was neutralized by template sanitizer")
	}
}

func TestHandleLanding(t *testing.T) {
	s := New("http://unused/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	s.handleLanding(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	checks := map[string]string{
		"https://t.me/vexa_bot": "telegram bot link",
		"Открыть в Telegram":    "cta button",
		"https://t.me/byttte":   "support link (без @)",
		"@byttte":               "support handle",
	}
	for substr, what := range checks {
		if !strings.Contains(body, substr) {
			t.Errorf("landing missing %s (%q)", what, substr)
		}
	}
	// Ссылки не должны быть нейтрализованы санитайзером html/template.
	if strings.Contains(body, "ZgotmplZ") {
		t.Error("landing link was neutralized by template sanitizer")
	}
}

// Без bot_url кнопка «Открыть в Telegram» не рендерится.
func TestHandleLandingNoBotURL(t *testing.T) {
	s := New("http://unused/%s", "https://vpn.example.com:8081", "", "")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	s.handleLanding(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Открыть в Telegram") {
		t.Error("cta button must be hidden when bot_url is empty")
	}
}

// Happ-клиент получает конфиг, а заголовки идентификации устройства (x-hwid и др.)
// прокидываются в панель для нативного HWID-учёта.
func TestHandleSubForwardsHWID(t *testing.T) {
	var gotHWID, gotOS string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHWID = r.Header.Get("x-hwid")
		gotOS = r.Header.Get("x-device-os")
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte")

	req := httptest.NewRequest("GET", "/sub/abc123", nil)
	req.SetPathValue("sub_id", "abc123")
	req.Header.Set("x-hwid", "HWID-XYZ")
	req.Header.Set("x-device-os", "iOS")
	req.Header.Set("User-Agent", "Happ/1.0")
	rec := httptest.NewRecorder()

	s.handleSub(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "config-body" {
		t.Fatalf("status=%d body=%q, want 200/config-body", rec.Code, rec.Body.String())
	}
	if gotHWID != "HWID-XYZ" || gotOS != "iOS" {
		t.Fatalf("upstream got x-hwid=%q x-device-os=%q, want HWID-XYZ/iOS", gotHWID, gotOS)
	}
}

// Unified endpoint: Happ UA → конфиг (text/plain), браузерный UA → HTML-страница.
func TestUnifiedEndpointHappGetsConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte")

	req := httptest.NewRequest("GET", "/sub/abc123", nil)
	req.SetPathValue("sub_id", "abc123")
	req.Header.Set("User-Agent", "Happ/1.0 (iPhone; iOS 17)")
	req.Header.Set("x-hwid", "HWID-UNI")
	rec := httptest.NewRecorder()

	s.handleUnified(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "config-body" {
		t.Fatalf("body = %q, want config-body", rec.Body.String())
	}
}

func TestUnifiedEndpointBrowserGetsPage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo",
			"upload=0; download=0; total=0; expire=4102444800")
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte")

	req := httptest.NewRequest("GET", "/sub/abc123", nil)
	req.SetPathValue("sub_id", "abc123")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	rec := httptest.NewRecorder()

	s.handleUnified(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "happ://add/") {
		t.Fatal("browser should get HTML page with happ deep link")
	}
}

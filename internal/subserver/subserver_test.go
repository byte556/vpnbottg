package subserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (Linux; Android 14)":                        "Android",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)": "iOS",
		"Happ/1.0 (iPad)": "iOS",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64)":       "Windows",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)": "macOS",
		"Keenetic/3.7":                   "router",
		"curl/8.0 (x86_64-pc-linux-gnu)": "Linux",
		"":                               "unknown",
	}
	for ua, want := range cases {
		if got := detectPlatform(ua); got != want {
			t.Errorf("detectPlatform(%q) = %q, want %q", ua, got, want)
		}
	}
}

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
	// Заглушка панели: отдаёт Subscription-Userinfo, как реальный 3X-UI sub-сервис.
	// download=25 ГБ, total=100 ГБ → 25%; expire=4102444800 (2100 год) → активна.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo",
			"upload=0; download=26843545600; total=107374182400; expire=4102444800")
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte", nil)

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
	s := New("http://unused/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte", nil)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	s.handleLanding(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	checks := map[string]string{
		"https://t.me/vexa_bot":  "telegram bot link",
		"Открыть в Telegram":     "cta button",
		"https://t.me/byttte":    "support link (без @)",
		"@byttte":                "support handle",
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
	s := New("http://unused/%s", "https://vpn.example.com:8081", "", "", nil)

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

// Устройство в пределах лимита: x-hwid есть, authorize=allowed → конфиг отдаётся,
// platform берётся из x-device-os, UA сохраняется.
func TestHandleSubAllowsDevice(t *testing.T) {
	upstreamHit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	repo := &fakeDeviceRepo{allowed: true}
	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte", repo)

	req := httptest.NewRequest("GET", "/sub/abc123", nil)
	req.SetPathValue("sub_id", "abc123")
	req.Header.Set("x-hwid", "HWID-XYZ")
	req.Header.Set("x-device-os", "iOS")
	req.Header.Set("User-Agent", "Happ/1.0")
	rec := httptest.NewRecorder()

	s.handleSub(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !upstreamHit || rec.Body.String() != "config-body" {
		t.Fatalf("config must be proxied; hit=%v body=%q", upstreamHit, rec.Body.String())
	}
	if repo.deviceID != "HWID-XYZ" || repo.platform != "iOS" || repo.userAgent != "Happ/1.0" {
		t.Fatalf("authorized device=%q ua=%q platform=%q, want HWID-XYZ/Happ/1.0/iOS",
			repo.deviceID, repo.userAgent, repo.platform)
	}
}

// Новое устройство сверх лимита: authorize=denied → 403, конфиг НЕ отдаётся.
func TestHandleSubBlocksOverLimit(t *testing.T) {
	upstreamHit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte", &fakeDeviceRepo{allowed: false})

	req := httptest.NewRequest("GET", "/sub/abc123", nil)
	req.SetPathValue("sub_id", "abc123")
	req.Header.Set("x-hwid", "HWID-NEW")
	rec := httptest.NewRecorder()

	s.handleSub(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if upstreamHit {
		t.Fatal("upstream must NOT be called when device is over limit")
	}
}

// Без x-hwid идентифицировать устройство нечем → 403, конфиг НЕ отдаётся.
func TestHandleSubRequiresHWID(t *testing.T) {
	upstreamHit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	repo := &fakeDeviceRepo{allowed: true}
	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte", repo)

	req := httptest.NewRequest("GET", "/sub/abc123", nil)
	req.SetPathValue("sub_id", "abc123")
	rec := httptest.NewRecorder()

	s.handleSub(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if upstreamHit || repo.called {
		t.Fatal("no x-hwid: must not authorize or call upstream")
	}
}

// Ошибка БД при проверке → fail-open: конфиг всё равно отдаётся.
func TestHandleSubFailOpenOnDBError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("config-body"))
	}))
	defer upstream.Close()

	s := New(upstream.URL+"/sub/%s", "https://vpn.example.com:8081", "https://t.me/vexa_bot", "@byttte", &fakeDeviceRepo{err: errors.New("db down")})

	req := httptest.NewRequest("GET", "/sub/abc123", nil)
	req.SetPathValue("sub_id", "abc123")
	req.Header.Set("x-hwid", "HWID-XYZ")
	rec := httptest.NewRecorder()

	s.handleSub(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (fail-open)", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "config-body" {
		t.Fatalf("body = %q, want config proxied on db error", rec.Body.String())
	}
}

type fakeDeviceRepo struct {
	allowed   bool
	isNew     bool
	err       error
	called    bool
	deviceID  string
	userAgent string
	platform  string
}

func (f *fakeDeviceRepo) AuthorizeDeviceConnection(ctx context.Context, subID, deviceID, userAgent, platform string) (bool, bool, error) {
	f.called = true
	f.deviceID = deviceID
	f.userAgent = userAgent
	f.platform = platform
	return f.allowed, f.isNew, f.err
}

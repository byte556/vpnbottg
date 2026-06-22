package subserver

import (
	"net/http"
	"strings"
)

// hwid возвращает аппаратный идентификатор устройства из заголовка x-hwid,
// который шлёт клиент Happ. Пусто — устройство не идентифицировано.
func hwid(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("x-hwid"))
}

// platform определяет платформу: сначала заголовок x-device-os от Happ,
// иначе — эвристика по User-Agent.
func platform(r *http.Request) string {
	if os := strings.TrimSpace(r.Header.Get("x-device-os")); os != "" {
		return normalizeOS(os)
	}
	return detectPlatform(r.UserAgent())
}

func normalizeOS(os string) string {
	switch strings.ToLower(os) {
	case "ios":
		return "iOS"
	case "android":
		return "Android"
	case "windows":
		return "Windows"
	case "macos", "mac os", "mac":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return os
	}
}

// detectPlatform определяет платформу клиента по User-Agent.
// Порядок проверок важен: iOS-агенты содержат "Mac OS X", роутеры — "Linux".
func detectPlatform(ua string) string {
	s := strings.ToLower(ua)
	switch {
	case strings.Contains(s, "android"):
		return "Android"
	case strings.Contains(s, "iphone"), strings.Contains(s, "ipad"),
		strings.Contains(s, "ipod"), strings.Contains(s, "ios"):
		return "iOS"
	case strings.Contains(s, "windows"):
		return "Windows"
	case strings.Contains(s, "mac os"), strings.Contains(s, "macos"),
		strings.Contains(s, "macintosh"), strings.Contains(s, "darwin"):
		return "macOS"
	case strings.Contains(s, "openwrt"), strings.Contains(s, "router"),
		strings.Contains(s, "keenetic"), strings.Contains(s, "mikrotik"),
		strings.Contains(s, "asuswrt"), strings.Contains(s, "padavan"):
		return "router"
	case strings.Contains(s, "linux"):
		return "Linux"
	default:
		return "unknown"
	}
}

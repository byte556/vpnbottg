// Package assets встраивает карточки-обложки экранов (PNG 1080×1080) в бинарь
// и отдаёт их как *tele.Photo для отправки/редактирования сообщений бота.
//
// Карточки генерируются из assets/promo/gen.js и копируются в cards/.
// Имя карточки = логический экран (см. cardFiles), а не имя файла.
package assets

import (
	"bytes"
	"embed"

	tele "gopkg.in/telebot.v3"
)

//go:embed cards/*.png
var cardsFS embed.FS

// cardFiles сопоставляет логический экран с файлом карточки.
var cardFiles = map[string]string{
	"menu_main":  "menu_main.png",  // гостевое меню
	"subscriber": "subscriber.png", // меню подписчика
	"buy":        "buy.png",        // конструктор тарифа
	"trial":      "trial.png",      // пробный период
	"devices":    "devices.png",    // мои устройства
	"settings":   "settings.png",   // мой тариф
	"promo":      "promo.png",      // ввод промокода
	"payment":    "payment.png",    // ожидание оплаты
	"activating": "activating.png", // оплата получена, активируем VPN
	"success":    "success.png",    // VPN активирован
	"help":       "help.png",       // помощь
	"invite":     "invite.png",     // пригласить друга
	"expired":    "expired.png",    // подписка истекла
}

// Photo возвращает *tele.Photo карточки экрана с заданной подписью,
// либо nil, если карточки нет (тогда вызывающий откатывается на текст).
func Photo(name, caption string) *tele.Photo {
	file, ok := cardFiles[name]
	if !ok {
		return nil
	}
	data, err := cardsFS.ReadFile("cards/" + file)
	if err != nil {
		return nil
	}
	return &tele.Photo{
		File:    tele.FromReader(bytes.NewReader(data)),
		Caption: caption,
	}
}

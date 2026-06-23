package handlers

import (
	"strings"

	"vpnbottg/internal/telegram/assets"

	tele "gopkg.in/telebot.v3"
)

// captionLimit — лимит Telegram на длину подписи к медиа (символов).
const captionLimit = 1024

// screen рендерит экран с карточкой-обложкой: фото (card) + текст как подпись.
// Переходы «на месте»:
//
//	фото → фото : редактирование медиа (без мигания)
//	текст → фото: старое текстовое сообщение удаляется, шлётся новое фото
//
// Откат на обычный текстовый экран (editOrFresh), если карточки нет или подпись
// длиннее лимита Telegram (иначе sendPhoto/editMedia вернёт ошибку).
func screen(c tele.Context, card, caption string, opts ...interface{}) error {
	photo := assets.Photo(card, caption)
	if photo == nil || len([]rune(caption)) > captionLimit {
		return editOrFresh(c, caption, opts...)
	}
	opts = sendOptsFirst(opts)

	// Текущее сообщение уже фото → редактируем медиа на месте.
	if cb := c.Callback(); cb != nil && cb.Message != nil && cb.Message.Photo != nil {
		err := c.Edit(photo, opts...)
		if err != nil && isNotModified(err) {
			return nil
		}
		return err
	}
	// Текущее сообщение текстовое (или это команда/нижняя кнопка) — фото из него
	// редактированием не получить: удаляем исходное (если это callback) и шлём фото.
	if c.Callback() != nil {
		_ = c.Delete()
	}
	return c.Send(photo, opts...)
}

// editOrFresh рендерит экран, редактируя текущее сообщение «на месте» для
// текстовых переходов (меню → подэкран → назад). Если триггерное сообщение —
// фото (экран QR-кода «Мой конфиг»), его нельзя превратить в текст редактированием,
// поэтому фото удаляется и отправляется новое сообщение.
//
// Также проглатывает ошибку Telegram «message is not modified», возникающую при
// повторном нажатии noop-кнопок (➕/➖ с тем же значением).
func editOrFresh(c tele.Context, what interface{}, opts ...interface{}) error {
	opts = sendOptsFirst(opts)
	if cb := c.Callback(); cb != nil && cb.Message != nil && cb.Message.Photo != nil {
		_ = c.Delete()
		return c.Send(what, opts...)
	}
	err := c.EditOrSend(what, opts...)
	if err != nil && isNotModified(err) {
		return nil
	}
	return err
}

// sendOptsFirst переставляет *tele.SendOptions перед остальными опциями.
// Это нужно из-за особенности telebot: extractOptions при встрече *SendOptions
// заменяет весь набор опций целиком (opts = sendOpts.copy()), затирая ранее
// выставленный ReplyMarkup. Если клавиатуру передали до SendOptions — она
// пропадёт, и сообщение уйдёт без кнопок. Ставим SendOptions первым, чтобы
// ReplyMarkup, идущий следом, сохранился.
func sendOptsFirst(opts []interface{}) []interface{} {
	var sendOpts, rest []interface{}
	for _, o := range opts {
		if _, ok := o.(*tele.SendOptions); ok {
			sendOpts = append(sendOpts, o)
		} else {
			rest = append(rest, o)
		}
	}
	if len(sendOpts) == 0 {
		return opts
	}
	return append(sendOpts, rest...)
}

func isNotModified(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "not modified") || strings.Contains(msg, "are exactly the same")
}

package handlers

import (
	"strings"

	tele "gopkg.in/telebot.v3"
)

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

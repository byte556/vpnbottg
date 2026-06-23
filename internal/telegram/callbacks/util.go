package callbacks

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

func isNotModified(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "not modified") || strings.Contains(msg, "exactly the same")
}

package handlers

import (
	"context"
	"time"

	"vpnbottg/internal/models"
	"vpnbottg/internal/service"
	"vpnbottg/internal/telegram/assets"
	"vpnbottg/internal/telegram/keyboard"
	"vpnbottg/internal/telegram/texts"

	tele "gopkg.in/telebot.v3"
)

// Screen — единое декларативное описание экрана бота: карточка-обложка, готовый
// текст и клавиатура. Один и тот же экран рендерится двумя способами:
//
//   - Render — интерактивно, редактированием текущего сообщения (юзер нажал кнопку);
//   - Push   — свежим сообщением по userID (асинхронное событие вебхука, где
//     tele.Context недоступен).
//
// Так определение экрана (карточка + текст + кнопки) живёт в одном месте, а не
// дублируется между handlers и обработчиком вебхука.
type Screen struct {
	Card   string            // имя карточки-обложки (пусто — без фото)
	Text   string            // готовый текст или подпись к фото
	Markup *tele.ReplyMarkup // клавиатура (nil — без кнопок)
	HTML   bool              // слать с ParseMode HTML
}

// opts собирает опции отправки в порядке, безопасном для telebot (SendOptions
// должен идти перед ReplyMarkup — см. sendOptsFirst).
func (s Screen) opts() []interface{} {
	var opts []interface{}
	if s.HTML {
		opts = append(opts, &tele.SendOptions{ParseMode: tele.ModeHTML})
	}
	if s.Markup != nil {
		opts = append(opts, s.Markup)
	}
	return sendOptsFirst(opts)
}

// Render рендерит экран в текущем сообщении (edit-in-place) через screen/editOrFresh.
func (s Screen) Render(c tele.Context) error {
	if s.Card != "" {
		return screen(c, s.Card, s.Text, s.opts()...)
	}
	return editOrFresh(c, s.Text, s.opts()...)
}

// Push отправляет экран свежим сообщением по userID. Карточка-обложка ставится,
// только если она есть и подпись укладывается в лимит Telegram; иначе шлётся текст.
func (s Screen) Push(bot *tele.Bot, userID int64) error {
	to := &tele.User{ID: userID}
	if s.Card != "" && len([]rune(s.Text)) <= captionLimit {
		if photo := assets.Photo(s.Card, s.Text); photo != nil {
			_, err := bot.Send(to, photo, s.opts()...)
			return err
		}
	}
	_, err := bot.Send(to, s.Text, s.opts()...)
	return err
}

// ---- Каталог экранов ----

// provisionSuccessScreen — экран «подписка выдана».
func provisionSuccessScreen() Screen {
	return Screen{
		Card:   "success",
		Text:   texts.T("provision.success"),
		Markup: keyboard.ProvisionSuccessKeyboard(),
		HTML:   true,
	}
}

// subscriberMenuScreen — главное меню активного подписчика.
func subscriberMenuScreen(sub *models.Subscription) Screen {
	daysLeft := max(0, int(time.Until(time.Unix(sub.ExpiresAt, 0)).Hours()/24))
	return Screen{
		Card:   "subscriber",
		Text:   texts.T("menu.subscriber.text", map[string]any{"DaysLeft": daysLeft}),
		Markup: keyboard.SubscriberMenu(),
	}
}

// deviceAddonScreen — уведомление об успешном добавлении устройств.
func deviceAddonScreen(devices int) Screen {
	return Screen{Text: texts.T("addon.device_success", map[string]any{"Devices": devices})}
}

// referralRewardScreen — уведомление рефереру о начисленном кешбэке.
func referralRewardScreen(name string, amount int64) Screen {
	return Screen{
		Text: texts.T("referral.reward", map[string]any{"Name": name, "Amount": amount}),
		HTML: true,
	}
}

// ---- Push-обёртки для обработчика вебхука (tele.Context недоступен) ----

// PushProvisionSuccess шлёт экран «подписка выдана» свежим сообщением.
func PushProvisionSuccess(bot *tele.Bot, userID int64) error {
	return provisionSuccessScreen().Push(bot, userID)
}

// PushSubscriberMenu подгружает активную подписку и шлёт меню подписчика.
func PushSubscriberMenu(bot *tele.Bot, subSvc *service.Subscription, userID int64) error {
	sub, err := subSvc.GetActive(context.Background(), userID)
	if err != nil {
		return err
	}
	return subscriberMenuScreen(sub).Push(bot, userID)
}

// PushDeviceAddonSuccess шлёт уведомление о добавленных устройствах.
func PushDeviceAddonSuccess(bot *tele.Bot, userID int64, devices int) error {
	return deviceAddonScreen(devices).Push(bot, userID)
}

// PushReferralReward шлёт рефереру уведомление о кешбэке.
func PushReferralReward(bot *tele.Bot, userID int64, name string, amount int64) error {
	return referralRewardScreen(name, amount).Push(bot, userID)
}

// PushMessage шлёт простое текстовое сообщение (ошибки провижининга и т.п.).
func PushMessage(bot *tele.Bot, userID int64, text string) error {
	return Screen{Text: text}.Push(bot, userID)
}

// PushCard шлёт произвольный экран-карточку (фото + подпись + клавиатура) свежим
// сообщением. Используется сервисом напоминаний, у которого нет tele.Context.
// Откат на текст — если карточки нет (см. Screen.Push).
func PushCard(bot *tele.Bot, userID int64, card, text string, markup *tele.ReplyMarkup) error {
	return Screen{Card: card, Text: text, Markup: markup, HTML: true}.Push(bot, userID)
}

package session

import (
	"math"
	"sync"
	"vpnbottg/internal/config"
)

type ConstructorState struct {
	devices int
	months  int

	// manage=true — конструктор открыт для управления активной подпиской
	// (продление/изменение), а не для новой покупки. Тогда цена считается
	// с учётом уже оплаченного: продление срока — по целевому числу устройств,
	// доплата за добавленные устройства — пропорционально оставшимся дням.
	manage      bool
	baseDevices int // текущий лимит устройств активной подписки
	daysLeft    int // осталось дней активной подписки
}

func (s *ConstructorState) GetDevices() int  { return s.devices }
func (s *ConstructorState) GetMonths() int   { return s.months }
func (s *ConstructorState) IsManage() bool   { return s.manage }
func (s *ConstructorState) BaseDevices() int { return s.baseDevices }
func (s *ConstructorState) DaysLeft() int    { return s.daysLeft }

func (s *ConstructorState) SetMonths(m int) { s.months = m }

// SetManage переводит конструктор в режим управления активной подпиской,
// фиксируя текущий лимит устройств и остаток дней (для пропорциональной цены).
func (s *ConstructorState) SetManage(baseDevices, daysLeft int) {
	s.manage = true
	if baseDevices < 1 {
		baseDevices = 1
	}
	s.baseDevices = baseDevices
	if daysLeft < 0 {
		daysLeft = 0
	}
	s.daysLeft = daysLeft
}

// ResetManage возвращает конструктор в режим новой покупки.
func (s *ConstructorState) ResetManage() {
	s.manage = false
	s.baseDevices = 0
	s.daysLeft = 0
}

// SetDevices задаёт число устройств (не ниже 1). Используется при открытии
// конструктора для продления — предзаполняем текущим лимитом подписки.
func (s *ConstructorState) SetDevices(d int) {
	if d < 1 {
		d = 1
	}
	s.devices = d
}

// AddDevices изменяет число устройств.
func (s *ConstructorState) AddDevices(v int) {
	s.devices += v
	if s.devices < 1 {
		s.devices = 1
	}
}

// CalcPrice — стоимость к оплате.
//
// Режим новой покупки (manage=false):
//
//	(база + доп.устройства × цена) × месяцы × скидка
//
// Режим управления активной подпиской (manage=true) складывает:
//   - продление срока (если months>0) — полная цена плана по целевому числу устройств;
//   - доплату за добавленные устройства на оставшиеся дни — пропорционально
//     (devices−baseDevices) × цена_за_устройство × daysLeft/30.
//
// Уменьшение числа устройств бесплатно (деньги не возвращаются).
func (s *ConstructorState) CalcPrice() int {
	cfg := config.Cfg.Bot.Constructor

	if !s.manage {
		return int(math.Round(s.planPrice(s.devices, s.months)))
	}

	total := 0.0
	if s.months > 0 {
		total += s.planPrice(s.devices, s.months)
	}
	if extra := s.devices - s.baseDevices; extra > 0 && s.daysLeft > 0 {
		total += float64(extra) * float64(cfg.PricePerDevice) * float64(s.daysLeft) / 30.0
	}
	return int(math.Round(total))
}

// planPrice — полная цена плана (база + доп.устройства) × месяцы × скидка за срок.
func (s *ConstructorState) planPrice(devices, months int) float64 {
	cfg := config.Cfg.Bot.Constructor
	discount, ok := cfg.MonthDiscounts[months]
	if !ok {
		discount = 1.0
	}
	base := float64(cfg.PriceBase) + float64(devices-1)*float64(cfg.PricePerDevice)
	return base * float64(months) * discount
}

// UpgradePrice — доплата за добавленные устройства на оставшиеся дни (без продления).
// В режиме новой покупки — 0.
func (s *ConstructorState) UpgradePrice() int {
	if !s.manage {
		return 0
	}
	cfg := config.Cfg.Bot.Constructor
	if extra := s.devices - s.baseDevices; extra > 0 && s.daysLeft > 0 {
		return int(math.Round(float64(extra) * float64(cfg.PricePerDevice) * float64(s.daysLeft) / 30.0))
	}
	return 0
}

// RenewPrice — цена продления срока по целевому числу устройств (без доплаты за апгрейд).
func (s *ConstructorState) RenewPrice() int {
	if s.months <= 0 {
		return 0
	}
	return int(math.Round(s.planPrice(s.devices, s.months)))
}

func (s *ConstructorState) CalcPricePerMonth() int {
	if s.months == 0 {
		return 0
	}
	return int(math.Round(float64(s.CalcPrice()) / float64(s.months)))
}

func (s *ConstructorState) CalcPricePerDay() int {
	days := s.months * 30
	if days == 0 {
		return 0
	}
	v := int(math.Round(float64(s.CalcPrice()) / float64(days)))
	if v < 1 {
		return 1
	}
	return v
}

// CalcSavings — сколько экономит по сравнению с ежемесячной оплатой без скидки.
// В режиме управления подпиской не применяется (0).
func (s *ConstructorState) CalcSavings() int {
	if s.manage || s.months <= 1 {
		return 0
	}
	cfg := config.Cfg.Bot.Constructor
	base := float64(cfg.PriceBase) + float64(s.devices-1)*float64(cfg.PricePerDevice)
	fullPrice := base * float64(s.months)
	return int(math.Round(fullPrice - float64(s.RenewPrice())))
}

// AdminAction — ожидаемый ввод от администратора ("find_user", "broadcast", "refund", "delete_user").
// Пусто = обычный режим.
type Session struct {
	PaymentID   string
	PaymentURL  string
	Constructor ConstructorState
	AdminAction string

	// Промокод-скидка, применённая к покупке в конструкторе (списывается после оплаты).
	PromoCode        string
	PromoDiscountPct int
	// AwaitPromo — ждём от пользователя ввод промокода следующим сообщением.
	AwaitPromo bool
}

// ApplyDiscount применяет активную промо-скидку к цене (округление вниз).
func (s *Session) ApplyDiscount(price int) int {
	if s.PromoDiscountPct <= 0 {
		return price
	}
	return price * (100 - s.PromoDiscountPct) / 100
}

type Store struct {
	mu   sync.Mutex
	data map[int64]*Session
}

var store = &Store{data: make(map[int64]*Session)}

func GetStore() *Store { return store }

func (s *Store) Get(tgID int64) Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.data[tgID]; ok {
		return *sess
	}
	cfg := config.Cfg.Bot.Constructor
	def := Session{
		Constructor: ConstructorState{
			devices: max(1, cfg.DefaultDevices),
			months:  1,
		},
	}
	s.data[tgID] = &def
	return def
}

func (s *Store) Save(tgID int64, sess Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := sess
	s.data[tgID] = &cp
}

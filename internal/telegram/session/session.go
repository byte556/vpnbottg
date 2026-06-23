package session

import (
	"math"
	"sync"
	"vpnbottg/internal/config"
)

type ConstructorState struct {
	gb      int
	devices int
	months  int
}

func (s *ConstructorState) GetGB() int      { return s.gb }
func (s *ConstructorState) GetDevices() int { return s.devices }
func (s *ConstructorState) GetMonths() int  { return s.months }

func (s *ConstructorState) SetMonths(m int) { s.months = m }

func (s *ConstructorState) AddGB(step int) {
	s.gb += step
	minGB := config.Cfg.Bot.Constructor.MinGB
	if minGB <= 0 {
		minGB = 10
	}
	if s.gb < minGB {
		s.gb = minGB
	}
}

// AddDevices изменяет число устройств. Трафик (GB) управляется независимо.
func (s *ConstructorState) AddDevices(v int) {
	s.devices += v
	if s.devices < 1 {
		s.devices = 1
	}
}

// CalcPrice: (GB × цена_за_ГБ + устройства × цена_за_устройство) × месяцы × скидка
func (s *ConstructorState) CalcPrice() int {
	cfg := config.Cfg.Bot.Constructor
	discount, ok := cfg.MonthDiscounts[s.months]
	if !ok {
		discount = 1.0
	}
	base := float64(s.gb)*float64(cfg.PricePerGB) + float64(s.devices)*float64(cfg.PricePerDevice)
	return int(math.Round(base * float64(s.months) * discount))
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
func (s *ConstructorState) CalcSavings() int {
	cfg := config.Cfg.Bot.Constructor
	base := float64(s.gb)*float64(cfg.PricePerGB) + float64(s.devices)*float64(cfg.PricePerDevice)
	fullPrice := base * float64(s.months) // без скидки
	return int(math.Round(fullPrice - float64(s.CalcPrice())))
}

// AdminAction — ожидаемый ввод от администратора ("find_user", "broadcast", "refund", "delete_user").
// Пусто = обычный режим.
type Session struct {
	PaymentID    string
	PaymentURL   string
	Constructor  ConstructorState
	AddOnDevices int
	AddOnGB      int
	AdminAction  string
}

func (s *Session) AddonDevicesPrice() int {
	return s.AddOnDevices * config.Cfg.Bot.Constructor.PricePerDevice
}

func (s *Session) AddonGBPrice() int {
	return s.AddOnGB * config.Cfg.Bot.Constructor.PricePerGB
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
	defaultGB := cfg.DefaultGB
	if defaultGB <= 0 {
		defaultGB = 30
	}
	def := Session{
		Constructor: ConstructorState{
			gb:      defaultGB,
			devices: max(1, cfg.DefaultDevices),
			months:  1,
		},
		AddOnDevices: 1,
		AddOnGB:      max(10, cfg.GBStep),
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

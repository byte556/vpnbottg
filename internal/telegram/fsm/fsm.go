package fsm

import (
	"math"
	"sort"
	"sync"
	"vpnbottg/internal/config"
)

type State string

const StateMenu State = "menu"
const StateConstructor State = "constructor"

type ConstructorState struct {
	planGB  int
	devices int
	months  int
}

func (s *ConstructorState) CalcPrice() int {
	gb := float64(s.planGB)
	devices := float64(s.devices)

	cfg := config.Cfg.Bot.Constructor

	wl := cfg.BaseMultiplier

	monthDiscount, ok := cfg.MonthDiscounts[s.months]
	if !ok {
		monthDiscount = 1.0
	}

	price := (wl*gb + cfg.FixPrice) * devices * float64(s.months) * monthDiscount
	return int(math.Round(price))
}
func (s *ConstructorState) GetMonths() int { return s.months }
func (s *ConstructorState) NextMonths() {
	allowed := sortedMonths()
	for i, v := range allowed {
		if v == s.months {
			if i+1 < len(allowed) {
				s.months = allowed[i+1]
			}
			return
		}
	}
	s.months = allowed[0]
}

func (s *ConstructorState) PrevMonths() {
	allowed := sortedMonths()
	for i, v := range allowed {
		if v == s.months {
			if i-1 >= 0 {
				s.months = allowed[i-1]
			}
			return
		}
	}
	s.months = allowed[0]
}

func sortedMonths() []int {
	discounts := config.Cfg.Bot.Constructor.MonthDiscounts
	allowed := make([]int, 0, len(discounts))
	for k := range discounts {
		allowed = append(allowed, k)
	}
	sort.Ints(allowed)
	return allowed
}
func (s *ConstructorState) GetPlanGB() int  { return s.planGB }
func (s *ConstructorState) GetDevices() int { return s.devices }

func (s *ConstructorState) AddPlanGB(v int) {
	s.planGB += v
	if s.planGB < 0 {
		s.planGB = 0
	}
}

func (s *ConstructorState) AddDevices(v int) {
	s.devices += v
	if s.devices < 1 {
		s.devices = 1
	}
}

type Session struct {
	PaymentID   string
	State       State
	Constructor ConstructorState
}

// Store — потокобезопасное хранилище сессий.
type Store struct {
	mu   sync.Mutex
	data map[int64]*Session
}

var store = &Store{
	data: make(map[int64]*Session),
}

func GetStore() *Store {
	return store
}

func (s *Store) Get(tgID int64) Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.data[tgID]; ok {
		return *sess
	}

	def := Session{
		State: StateMenu,
		Constructor: ConstructorState{
			planGB:  30,
			devices: 1,
			months:  1,
		},
	}
	s.data[tgID] = &def
	return def
}

// Save — сохранить (возможно изменённую) сессию.
func (s *Store) Save(tgID int64, sess Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := sess
	s.data[tgID] = &cp
}

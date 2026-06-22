package service

import (
	"context"
	"errors"
	"fmt"
	"vpnbottg/internal/config"
	"vpnbottg/internal/infra/logger"
	"vpnbottg/internal/models"
	"vpnbottg/internal/repository"
)

type User struct {
	users repository.Users
	audit repository.Audit
}

func NewUserService(users repository.Users, audit repository.Audit) *User {
	return &User{users: users, audit: audit}
}

// EnsureUser создаёт юзера если не существует, обновляет username/first_name.
// Если передан referrerID — записывает реферала (один раз).
func (s *User) EnsureUser(ctx context.Context, tgID int64, username, firstName string, referrerID *int64) error {
	log := logger.L().With().Int64("tg_id", tgID).Logger()

	u := &models.User{
		TgID:       tgID,
		Username:   username,
		FirstName:  firstName,
		ReferredBy: referrerID,
	}
	if err := s.users.UpsertUser(ctx, u); err != nil {
		log.Error().Err(err).Msg("ensureUser: upsert failed")
		return fmt.Errorf("ensureUser: %w", err)
	}
	log.Debug().Msg("ensureUser: ok")
	return nil
}

// GetUser возвращает юзера или ErrNotFound.
func (s *User) GetUser(ctx context.Context, tgID int64) (*models.User, error) {
	u, err := s.users.GetUser(ctx, tgID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("getUser: %w", err)
	}
	return u, nil
}
func (s *User) IsAdmin(userID int64) bool {
	adminID := config.Cfg.Bot.AdminID
	return adminID != 0 && userID == adminID
}

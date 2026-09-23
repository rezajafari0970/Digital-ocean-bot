package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"time"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUnauthorized = errors.New("unauthorized")

type Store interface {
	FindUser(context.Context, string) (User, error)
	CreateSession(context.Context, Session) error
	FindSession(context.Context, []byte) (Session, User, error)
	DeleteSession(context.Context, string) error
}
type Service struct {
	Store Store
	TTL   time.Duration
}

func (s Service) Login(ctx context.Context, username, password string) (string, Principal, error) {
	u, err := s.Store.FindUser(ctx, username)
	if err != nil || !u.Enabled || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", Principal{}, ErrInvalidCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Principal{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	h := sha256.Sum256([]byte(token))
	ttl := s.TTL
	if ttl <= 0 {
		ttl = 4 * time.Hour
	}
	session := Session{UserID: u.ID, TokenHash: h[:], ExpiresAt: time.Now().UTC().Add(ttl)}
	if err := s.Store.CreateSession(ctx, session); err != nil {
		return "", Principal{}, err
	}
	return token, Principal{UserID: u.ID, Username: u.Username, Role: u.Role}, nil
}

func (s Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthorized
	}
	h := sha256.Sum256([]byte(token))
	session, u, err := s.Store.FindSession(ctx, h[:])
	if err != nil || !u.Enabled || time.Now().UTC().After(session.ExpiresAt) {
		return Principal{}, ErrUnauthorized
	}
	return Principal{UserID: u.ID, Username: u.Username, Role: u.Role, SessionID: session.ID}, nil
}
func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", ErrInvalidCredentials
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

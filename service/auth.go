package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"task-service/entity"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 72
	maxEmailLen    = 254
)

var dummyPasswordHash = []byte("$2a$10$t1TRIStvSk8NIKsrcUb31uME5jflYMf5zMdsaP71OValzmS6N7nia")

type UserRepository interface {
	Create(ctx context.Context, user entity.User) (entity.User, error)
	GetByEmail(ctx context.Context, email string) (entity.User, error)
}

type AuthService struct {
	users     UserRepository
	jwtSecret []byte
	tokenTTL  time.Duration
	logger    *slog.Logger
	now       func() time.Time
}

func NewAuthService(users UserRepository, jwtSecret []byte, tokenTTL time.Duration, logger *slog.Logger) *AuthService {
	return &AuthService{
		users:     users,
		jwtSecret: jwtSecret,
		tokenTTL:  tokenTTL,
		logger:    logger,
		now:       time.Now,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password string) (entity.User, error) {
	email = normalizeEmail(email)
	if err := validateEmail(email); err != nil {
		return entity.User{}, err
	}
	if err := validatePassword(password); err != nil {
		return entity.User{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return entity.User{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.users.Create(ctx, entity.User{Email: email, PasswordHash: string(hash)})
	if err != nil {
		return entity.User{}, err
	}

	s.logger.InfoContext(ctx, "user registered", "user_id", user.ID)
	return user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (entity.Token, error) {
	user, err := s.users.GetByEmail(ctx, normalizeEmail(email))
	if err != nil {
		if errors.Is(err, entity.ErrUserNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
			s.logger.WarnContext(ctx, "login failed", "reason", "unknown email")
			return entity.Token{}, entity.ErrInvalidCredentials
		}
		return entity.Token{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		s.logger.WarnContext(ctx, "login failed", "reason", "wrong password", "user_id", user.ID)
		return entity.Token{}, entity.ErrInvalidCredentials
	}

	now := s.now()
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(user.ID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return entity.Token{}, fmt.Errorf("sign token: %w", err)
	}

	s.logger.InfoContext(ctx, "user logged in", "user_id", user.ID)
	return entity.Token{AccessToken: signed, ExpiresIn: s.tokenTTL}, nil
}

func (s *AuthService) ParseToken(token string) (int64, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return s.jwtSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil {
		return 0, entity.ErrInvalidToken
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, entity.ErrInvalidToken
	}
	return userID, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmail(email string) error {
	if email == "" || len(email) > maxEmailLen {
		return &entity.ValidationError{Message: "email is invalid"}
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return &entity.ValidationError{Message: "email is invalid"}
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < minPasswordLen || len(password) > maxPasswordLen {
		return &entity.ValidationError{
			Message: fmt.Sprintf("password must be between %d and %d bytes", minPasswordLen, maxPasswordLen),
		}
	}
	return nil
}

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"task-service/entity"
)

var testSecret = []byte("test-secret-that-is-at-least-32-bytes-long")

func newTestAuthService(t *testing.T) (*AuthService, *MockUserRepository) {
	repo := NewMockUserRepository(t)
	return NewAuthService(repo, testSecret, time.Hour, discardLogger), repo
}

func TestAuthService_Register(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		setup    func(repo *MockUserRepository)
		wantErr  error
	}{
		{
			name:     "email is normalized and password is hashed",
			email:    "  User@Example.COM ",
			password: "password123",
			setup: func(repo *MockUserRepository) {
				repo.EXPECT().
					Create(mock.Anything, mock.MatchedBy(func(u entity.User) bool {
						return u.Email == "user@example.com" &&
							bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("password123")) == nil
					})).
					RunAndReturn(func(_ context.Context, u entity.User) (entity.User, error) {
						u.ID = 1
						return u, nil
					})
			},
		},
		{
			name:     "email already taken",
			email:    "user@example.com",
			password: "password123",
			setup: func(repo *MockUserRepository) {
				repo.EXPECT().Create(mock.Anything, mock.Anything).Return(entity.User{}, entity.ErrEmailTaken)
			},
			wantErr: entity.ErrEmailTaken,
		},
		{name: "empty email", email: "", password: "password123", wantErr: errValidation},
		{name: "email without domain", email: "user@", password: "password123", wantErr: errValidation},
		{name: "email with display name", email: "User <user@example.com>", password: "password123", wantErr: errValidation},
		{name: "short password", email: "user@example.com", password: "short", wantErr: errValidation},
		{name: "password longer than 72 bytes", email: "user@example.com", password: strings.Repeat("a", 73), wantErr: errValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestAuthService(t)
			if tt.setup != nil {
				tt.setup(repo)
			}

			user, err := svc.Register(context.Background(), tt.email, tt.password)

			requireError(t, err, tt.wantErr)
			if tt.wantErr == nil {
				assert.Equal(t, int64(1), user.ID)
				assert.Equal(t, "user@example.com", user.Email)
			}
		})
	}
}

func TestAuthService_Login(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	require.NoError(t, err)
	user := entity.User{ID: 7, Email: "user@example.com", PasswordHash: string(hash)}
	errDB := errors.New("db is down")

	tests := []struct {
		name     string
		email    string
		password string
		setup    func(repo *MockUserRepository)
		wantErr  error
	}{
		{
			name:     "valid credentials, email is normalized",
			email:    "USER@example.com",
			password: "password123",
			setup: func(repo *MockUserRepository) {
				repo.EXPECT().GetByEmail(mock.Anything, "user@example.com").Return(user, nil)
			},
		},
		{
			name:     "wrong password",
			email:    "user@example.com",
			password: "wrong-password",
			setup: func(repo *MockUserRepository) {
				repo.EXPECT().GetByEmail(mock.Anything, "user@example.com").Return(user, nil)
			},
			wantErr: entity.ErrInvalidCredentials,
		},
		{
			name:     "unknown email",
			email:    "nobody@example.com",
			password: "password123",
			setup: func(repo *MockUserRepository) {
				repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").Return(entity.User{}, entity.ErrUserNotFound)
			},
			wantErr: entity.ErrInvalidCredentials,
		},
		{
			name:     "repository failure is not hidden",
			email:    "user@example.com",
			password: "password123",
			setup: func(repo *MockUserRepository) {
				repo.EXPECT().GetByEmail(mock.Anything, "user@example.com").Return(entity.User{}, errDB)
			},
			wantErr: errDB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestAuthService(t)
			tt.setup(repo)

			token, err := svc.Login(context.Background(), tt.email, tt.password)

			requireError(t, err, tt.wantErr)
			if tt.wantErr != nil {
				return
			}
			assert.Equal(t, time.Hour, token.ExpiresIn)

			userID, err := svc.ParseToken(token.AccessToken)
			require.NoError(t, err)
			assert.Equal(t, user.ID, userID)
		})
	}
}

func TestAuthService_ParseToken_Rejects(t *testing.T) {
	now := time.Now()
	svc, _ := newTestAuthService(t)
	svc.now = func() time.Time { return now }

	sign := func(method jwt.SigningMethod, key any, claims jwt.RegisteredClaims) string {
		t.Helper()
		s, err := jwt.NewWithClaims(method, claims).SignedString(key)
		require.NoError(t, err)
		return s
	}

	validClaims := jwt.RegisteredClaims{
		Subject:   "1",
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "garbage", token: "not-a-token"},
		{name: "wrong secret", token: sign(jwt.SigningMethodHS256, []byte("another-secret-that-is-32-bytes-long!!"), validClaims)},
		{name: "alg none", token: sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, validClaims)},
		{name: "other hmac alg", token: sign(jwt.SigningMethodHS512, testSecret, validClaims)},
		{name: "expired", token: sign(jwt.SigningMethodHS256, testSecret, jwt.RegisteredClaims{
			Subject:   "1",
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Minute)),
		})},
		{name: "without expiration", token: sign(jwt.SigningMethodHS256, testSecret, jwt.RegisteredClaims{Subject: "1"})},
		{name: "non numeric subject", token: sign(jwt.SigningMethodHS256, testSecret, jwt.RegisteredClaims{
			Subject:   "admin",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ParseToken(tt.token)

			require.ErrorIs(t, err, entity.ErrInvalidToken)
		})
	}
}

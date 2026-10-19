package entity

import "time"

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Token struct {
	AccessToken string
	ExpiresIn   time.Duration
}

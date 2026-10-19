package entity

import "errors"

var (
	ErrTaskNotFound       = errors.New("task not found")
	ErrUserNotFound       = errors.New("user not found")
	ErrAssigneeNotFound   = errors.New("assignee not found")
	ErrForbidden          = errors.New("only the task creator can modify it")
	ErrEmailTaken         = errors.New("email is already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

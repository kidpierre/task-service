package entity

import "time"

type Status string

const (
	StatusCreated    Status = "created"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusOverdue    Status = "overdue"
)

func (s Status) Valid() bool {
	switch s {
	case StatusCreated, StatusInProgress, StatusDone, StatusOverdue:
		return true
	}
	return false
}

type Task struct {
	ID          int64
	Title       string
	Description string
	Status      Status
	Deadline    time.Time
	CreatorID   int64
	AssigneeID  int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TaskCreate struct {
	Title       string
	Description string
	Deadline    time.Time
	AssigneeID  *int64
}

type TaskUpdate struct {
	Title       *string
	Description *string
	Deadline    *time.Time
	Status      *Status
	AssigneeID  *int64
}

type TaskFilter struct {
	Status       *Status
	AssigneeID   *int64
	DeadlineFrom *time.Time
	DeadlineTo   *time.Time
	Limit        int
	Offset       int
}

type TaskPage struct {
	Items  []Task
	Total  int
	Limit  int
	Offset int
}

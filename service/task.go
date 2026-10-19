package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"task-service/entity"
)

const (
	DefaultPageLimit  = 20
	MaxPageLimit      = 100
	maxTitleLen       = 200
	maxDescriptionLen = 5000
)

type TaskRepository interface {
	Create(ctx context.Context, task entity.Task) (entity.Task, error)
	GetByID(ctx context.Context, id int64) (entity.Task, error)
	List(ctx context.Context, filter entity.TaskFilter) ([]entity.Task, int, error)
	Update(ctx context.Context, id int64, upd entity.TaskUpdate) (entity.Task, error)
	Delete(ctx context.Context, id int64) error
	MarkOverdue(ctx context.Context, now time.Time) (int64, error)
}

type TaskService struct {
	tasks  TaskRepository
	logger *slog.Logger
	now    func() time.Time
}

func NewTaskService(tasks TaskRepository, logger *slog.Logger) *TaskService {
	return &TaskService{
		tasks:  tasks,
		logger: logger,
		now:    time.Now,
	}
}

func (s *TaskService) Create(ctx context.Context, userID int64, in entity.TaskCreate) (entity.Task, error) {
	title := strings.TrimSpace(in.Title)
	if err := validateTitle(title); err != nil {
		return entity.Task{}, err
	}
	if err := validateDescription(in.Description); err != nil {
		return entity.Task{}, err
	}
	if in.Deadline.IsZero() {
		return entity.Task{}, &entity.ValidationError{Message: "deadline is required"}
	}
	if !in.Deadline.After(s.now()) {
		return entity.Task{}, &entity.ValidationError{Message: "deadline must be in the future"}
	}

	assigneeID := userID
	if in.AssigneeID != nil {
		if *in.AssigneeID <= 0 {
			return entity.Task{}, &entity.ValidationError{Message: "assignee_id must be positive"}
		}
		assigneeID = *in.AssigneeID
	}

	task, err := s.tasks.Create(ctx, entity.Task{
		Title:       title,
		Description: in.Description,
		Status:      entity.StatusCreated,
		Deadline:    in.Deadline.UTC(),
		CreatorID:   userID,
		AssigneeID:  assigneeID,
	})
	if err != nil {
		return entity.Task{}, err
	}

	s.logger.InfoContext(ctx, "task created", "task_id", task.ID, "user_id", userID)
	return task, nil
}

func (s *TaskService) Get(ctx context.Context, id int64) (entity.Task, error) {
	return s.tasks.GetByID(ctx, id)
}

func (s *TaskService) List(ctx context.Context, filter entity.TaskFilter) (entity.TaskPage, error) {
	if filter.Status != nil && !filter.Status.Valid() {
		return entity.TaskPage{}, &entity.ValidationError{Message: "status is invalid"}
	}
	if filter.DeadlineFrom != nil && filter.DeadlineTo != nil && filter.DeadlineFrom.After(*filter.DeadlineTo) {
		return entity.TaskPage{}, &entity.ValidationError{Message: "deadline_from must not be after deadline_to"}
	}
	if filter.Offset < 0 {
		return entity.TaskPage{}, &entity.ValidationError{Message: "offset must not be negative"}
	}
	if filter.Limit <= 0 {
		filter.Limit = DefaultPageLimit
	}
	if filter.Limit > MaxPageLimit {
		filter.Limit = MaxPageLimit
	}

	tasks, total, err := s.tasks.List(ctx, filter)
	if err != nil {
		return entity.TaskPage{}, err
	}

	return entity.TaskPage{Items: tasks, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *TaskService) Update(ctx context.Context, userID, id int64, upd entity.TaskUpdate) (entity.Task, error) {
	task, err := s.tasks.GetByID(ctx, id)
	if err != nil {
		return entity.Task{}, err
	}
	if task.CreatorID != userID {
		return entity.Task{}, entity.ErrForbidden
	}

	upd, err = normalizeUpdate(upd)
	if err != nil {
		return entity.Task{}, err
	}

	updated, err := s.tasks.Update(ctx, id, upd)
	if err != nil {
		return entity.Task{}, err
	}

	s.logger.InfoContext(ctx, "task updated", "task_id", id, "user_id", userID)
	return updated, nil
}

func (s *TaskService) Delete(ctx context.Context, userID, id int64) error {
	task, err := s.tasks.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if task.CreatorID != userID {
		return entity.ErrForbidden
	}

	if err := s.tasks.Delete(ctx, id); err != nil {
		return err
	}

	s.logger.InfoContext(ctx, "task deleted", "task_id", id, "user_id", userID)
	return nil
}

func (s *TaskService) MarkOverdue(ctx context.Context) (int64, error) {
	return s.tasks.MarkOverdue(ctx, s.now())
}

func normalizeUpdate(upd entity.TaskUpdate) (entity.TaskUpdate, error) {
	if upd.Title == nil && upd.Description == nil && upd.Deadline == nil && upd.Status == nil && upd.AssigneeID == nil {
		return entity.TaskUpdate{}, &entity.ValidationError{Message: "no fields to update"}
	}

	if upd.Title != nil {
		title := strings.TrimSpace(*upd.Title)
		if err := validateTitle(title); err != nil {
			return entity.TaskUpdate{}, err
		}
		upd.Title = &title
	}
	if upd.Description != nil {
		if err := validateDescription(*upd.Description); err != nil {
			return entity.TaskUpdate{}, err
		}
	}
	if upd.Deadline != nil {
		if upd.Deadline.IsZero() {
			return entity.TaskUpdate{}, &entity.ValidationError{Message: "deadline is invalid"}
		}
		deadline := upd.Deadline.UTC()
		upd.Deadline = &deadline
	}
	if upd.Status != nil {
		if !upd.Status.Valid() {
			return entity.TaskUpdate{}, &entity.ValidationError{Message: "status is invalid"}
		}
		if *upd.Status == entity.StatusOverdue {
			return entity.TaskUpdate{}, &entity.ValidationError{Message: "status overdue is set automatically"}
		}
	}
	if upd.AssigneeID != nil && *upd.AssigneeID <= 0 {
		return entity.TaskUpdate{}, &entity.ValidationError{Message: "assignee_id must be positive"}
	}
	return upd, nil
}

func validateTitle(title string) error {
	if title == "" {
		return &entity.ValidationError{Message: "title is required"}
	}
	if utf8.RuneCountInString(title) > maxTitleLen {
		return &entity.ValidationError{Message: fmt.Sprintf("title must be at most %d characters", maxTitleLen)}
	}
	return nil
}

func validateDescription(description string) error {
	if utf8.RuneCountInString(description) > maxDescriptionLen {
		return &entity.ValidationError{Message: fmt.Sprintf("description must be at most %d characters", maxDescriptionLen)}
	}
	return nil
}

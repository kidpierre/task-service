package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"task-service/entity"
)

const (
	creatorID int64 = 1
	otherID   int64 = 2
	taskID    int64 = 10
)

var (
	testNow       = time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	errValidation = errors.New("validation error")
)

func newTestTaskService(t *testing.T) (*TaskService, *MockTaskRepository) {
	repo := NewMockTaskRepository(t)
	svc := NewTaskService(repo, discardLogger)
	svc.now = func() time.Time { return testNow }
	return svc, repo
}

func existingTask() entity.Task {
	return entity.Task{
		ID:          taskID,
		Title:       "Write report",
		Description: "Quarterly report",
		Status:      entity.StatusCreated,
		Deadline:    testNow.Add(48 * time.Hour),
		CreatorID:   creatorID,
		AssigneeID:  creatorID,
	}
}

func storedTask(task entity.Task) func(repo *MockTaskRepository) {
	return func(repo *MockTaskRepository) {
		repo.EXPECT().GetByID(mock.Anything, taskID).Return(task, nil)
	}
}

func ptr[T any](v T) *T {
	return &v
}

func requireError(t *testing.T, err, want error) {
	t.Helper()
	if want == nil {
		require.NoError(t, err)
		return
	}
	if errors.Is(want, errValidation) {
		var ve *entity.ValidationError
		require.ErrorAs(t, err, &ve)
		return
	}
	require.ErrorIs(t, err, want)
}

func TestTaskService_Create(t *testing.T) {
	deadline := testNow.Add(time.Hour)

	tests := []struct {
		name    string
		in      entity.TaskCreate
		saved   entity.Task
		wantErr error
	}{
		{
			name: "assignee defaults to creator",
			in:   entity.TaskCreate{Title: "  Write report  ", Deadline: deadline},
			saved: entity.Task{
				Title:      "Write report",
				Status:     entity.StatusCreated,
				Deadline:   deadline,
				CreatorID:  creatorID,
				AssigneeID: creatorID,
			},
		},
		{
			name: "explicit assignee",
			in:   entity.TaskCreate{Title: "Write report", Deadline: deadline, AssigneeID: ptr(otherID)},
			saved: entity.Task{
				Title:      "Write report",
				Status:     entity.StatusCreated,
				Deadline:   deadline,
				CreatorID:  creatorID,
				AssigneeID: otherID,
			},
		},
		{
			name: "deadline is stored in UTC",
			in:   entity.TaskCreate{Title: "t", Deadline: deadline.In(time.FixedZone("UTC+3", 3*60*60))},
			saved: entity.Task{
				Title:      "t",
				Status:     entity.StatusCreated,
				Deadline:   deadline,
				CreatorID:  creatorID,
				AssigneeID: creatorID,
			},
		},
		{name: "empty title", in: entity.TaskCreate{Title: "   ", Deadline: deadline}, wantErr: errValidation},
		{name: "too long title", in: entity.TaskCreate{Title: strings.Repeat("a", maxTitleLen+1), Deadline: deadline}, wantErr: errValidation},
		{name: "too long description", in: entity.TaskCreate{Title: "t", Description: strings.Repeat("a", maxDescriptionLen+1), Deadline: deadline}, wantErr: errValidation},
		{name: "missing deadline", in: entity.TaskCreate{Title: "t"}, wantErr: errValidation},
		{name: "deadline in the past", in: entity.TaskCreate{Title: "t", Deadline: testNow.Add(-time.Hour)}, wantErr: errValidation},
		{name: "non positive assignee", in: entity.TaskCreate{Title: "t", Deadline: deadline, AssigneeID: ptr(int64(0))}, wantErr: errValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			if tt.wantErr == nil {
				repo.EXPECT().Create(mock.Anything, tt.saved).
					RunAndReturn(func(_ context.Context, task entity.Task) (entity.Task, error) {
						task.ID = taskID
						return task, nil
					})
			}

			task, err := svc.Create(context.Background(), creatorID, tt.in)

			requireError(t, err, tt.wantErr)
			if tt.wantErr == nil {
				assert.Equal(t, taskID, task.ID)
			}
		})
	}
}

func TestTaskService_Create_ReturnsRepositoryError(t *testing.T) {
	svc, repo := newTestTaskService(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(entity.Task{}, entity.ErrAssigneeNotFound)

	_, err := svc.Create(context.Background(), creatorID, entity.TaskCreate{
		Title:      "Write report",
		Deadline:   testNow.Add(time.Hour),
		AssigneeID: ptr(int64(999)),
	})

	require.ErrorIs(t, err, entity.ErrAssigneeNotFound)
}

func TestTaskService_Get(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
	}{
		{name: "found"},
		{name: "not found", repoErr: entity.ErrTaskNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			repo.EXPECT().GetByID(mock.Anything, taskID).Return(existingTask(), tt.repoErr)

			task, err := svc.Get(context.Background(), taskID)

			requireError(t, err, tt.repoErr)
			if tt.repoErr == nil {
				assert.Equal(t, existingTask(), task)
			}
		})
	}
}

func TestTaskService_Update(t *testing.T) {
	assigned := existingTask()
	assigned.AssigneeID = otherID
	deadline := time.Date(2026, 2, 1, 15, 0, 0, 0, time.FixedZone("UTC+3", 3*60*60))

	tests := []struct {
		name    string
		userID  int64
		upd     entity.TaskUpdate
		setup   func(repo *MockTaskRepository)
		wantErr error
	}{
		{
			name:   "only passed fields are sent to repository",
			userID: creatorID,
			upd:    entity.TaskUpdate{Title: ptr("  New title  "), Deadline: &deadline},
			setup: func(repo *MockTaskRepository) {
				repo.EXPECT().GetByID(mock.Anything, taskID).Return(existingTask(), nil)
				repo.EXPECT().Update(mock.Anything, taskID, entity.TaskUpdate{
					Title:    ptr("New title"),
					Deadline: ptr(deadline.UTC()),
				}).Return(existingTask(), nil)
			},
		},
		{
			name:   "status and assignee",
			userID: creatorID,
			upd:    entity.TaskUpdate{Status: ptr(entity.StatusInProgress), AssigneeID: ptr(otherID)},
			setup: func(repo *MockTaskRepository) {
				repo.EXPECT().GetByID(mock.Anything, taskID).Return(existingTask(), nil)
				repo.EXPECT().Update(mock.Anything, taskID, entity.TaskUpdate{
					Status:     ptr(entity.StatusInProgress),
					AssigneeID: ptr(otherID),
				}).Return(existingTask(), nil)
			},
		},
		{
			name:    "not creator",
			userID:  otherID,
			upd:     entity.TaskUpdate{Title: ptr("hacked")},
			setup:   storedTask(existingTask()),
			wantErr: entity.ErrForbidden,
		},
		{
			name:    "assignee is not allowed to update",
			userID:  otherID,
			upd:     entity.TaskUpdate{Status: ptr(entity.StatusDone)},
			setup:   storedTask(assigned),
			wantErr: entity.ErrForbidden,
		},
		{
			name:   "task not found",
			userID: creatorID,
			upd:    entity.TaskUpdate{Title: ptr("x")},
			setup: func(repo *MockTaskRepository) {
				repo.EXPECT().GetByID(mock.Anything, taskID).Return(entity.Task{}, entity.ErrTaskNotFound)
			},
			wantErr: entity.ErrTaskNotFound,
		},
		{
			name:   "assignee does not exist",
			userID: creatorID,
			upd:    entity.TaskUpdate{AssigneeID: ptr(int64(999))},
			setup: func(repo *MockTaskRepository) {
				repo.EXPECT().GetByID(mock.Anything, taskID).Return(existingTask(), nil)
				repo.EXPECT().Update(mock.Anything, taskID, mock.Anything).Return(entity.Task{}, entity.ErrAssigneeNotFound)
			},
			wantErr: entity.ErrAssigneeNotFound,
		},
		{name: "no fields", userID: creatorID, upd: entity.TaskUpdate{}, setup: storedTask(existingTask()), wantErr: errValidation},
		{name: "empty title", userID: creatorID, upd: entity.TaskUpdate{Title: ptr(" ")}, setup: storedTask(existingTask()), wantErr: errValidation},
		{name: "unknown status", userID: creatorID, upd: entity.TaskUpdate{Status: ptr(entity.Status("archived"))}, setup: storedTask(existingTask()), wantErr: errValidation},
		{name: "manual overdue", userID: creatorID, upd: entity.TaskUpdate{Status: ptr(entity.StatusOverdue)}, setup: storedTask(existingTask()), wantErr: errValidation},
		{name: "zero deadline", userID: creatorID, upd: entity.TaskUpdate{Deadline: ptr(time.Time{})}, setup: storedTask(existingTask()), wantErr: errValidation},
		{name: "non positive assignee", userID: creatorID, upd: entity.TaskUpdate{AssigneeID: ptr(int64(-1))}, setup: storedTask(existingTask()), wantErr: errValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			tt.setup(repo)

			_, err := svc.Update(context.Background(), tt.userID, taskID, tt.upd)

			requireError(t, err, tt.wantErr)
		})
	}
}

func TestTaskService_Delete(t *testing.T) {
	tests := []struct {
		name    string
		userID  int64
		setup   func(repo *MockTaskRepository)
		wantErr error
	}{
		{
			name:   "creator deletes",
			userID: creatorID,
			setup: func(repo *MockTaskRepository) {
				repo.EXPECT().GetByID(mock.Anything, taskID).Return(existingTask(), nil)
				repo.EXPECT().Delete(mock.Anything, taskID).Return(nil)
			},
		},
		{
			name:    "not creator",
			userID:  otherID,
			setup:   storedTask(existingTask()),
			wantErr: entity.ErrForbidden,
		},
		{
			name:   "task not found",
			userID: creatorID,
			setup: func(repo *MockTaskRepository) {
				repo.EXPECT().GetByID(mock.Anything, taskID).Return(entity.Task{}, entity.ErrTaskNotFound)
			},
			wantErr: entity.ErrTaskNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			tt.setup(repo)

			err := svc.Delete(context.Background(), tt.userID, taskID)

			requireError(t, err, tt.wantErr)
		})
	}
}

func TestTaskService_List(t *testing.T) {
	tests := []struct {
		name      string
		filter    entity.TaskFilter
		wantLimit int
		wantErr   error
	}{
		{name: "default limit", filter: entity.TaskFilter{}, wantLimit: DefaultPageLimit},
		{name: "custom limit", filter: entity.TaskFilter{Limit: 5, Offset: 10}, wantLimit: 5},
		{name: "limit is capped", filter: entity.TaskFilter{Limit: MaxPageLimit + 1}, wantLimit: MaxPageLimit},
		{
			name: "filters are passed to repository",
			filter: entity.TaskFilter{
				Status:       ptr(entity.StatusDone),
				AssigneeID:   ptr(otherID),
				DeadlineFrom: ptr(testNow),
				DeadlineTo:   ptr(testNow.Add(time.Hour)),
				Limit:        5,
			},
			wantLimit: 5,
		},
		{name: "negative offset", filter: entity.TaskFilter{Offset: -1}, wantErr: errValidation},
		{name: "unknown status", filter: entity.TaskFilter{Status: ptr(entity.Status("archived"))}, wantErr: errValidation},
		{
			name: "inverted deadline range",
			filter: entity.TaskFilter{
				DeadlineFrom: ptr(testNow.Add(time.Hour)),
				DeadlineTo:   ptr(testNow),
			},
			wantErr: errValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			if tt.wantErr == nil {
				expected := tt.filter
				expected.Limit = tt.wantLimit
				repo.EXPECT().List(mock.Anything, expected).Return([]entity.Task{existingTask()}, 42, nil)
			}

			page, err := svc.List(context.Background(), tt.filter)

			requireError(t, err, tt.wantErr)
			if tt.wantErr == nil {
				assert.Equal(t, tt.wantLimit, page.Limit)
				assert.Equal(t, tt.filter.Offset, page.Offset)
				assert.Equal(t, 42, page.Total)
				assert.Len(t, page.Items, 1)
			}
		})
	}
}

func TestTaskService_MarkOverdue_UsesCurrentTime(t *testing.T) {
	svc, repo := newTestTaskService(t)
	repo.EXPECT().MarkOverdue(mock.Anything, testNow).Return(int64(3), nil)

	n, err := svc.MarkOverdue(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
}

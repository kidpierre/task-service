package http

import (
	"context"
	"log/slog"
	"net/http"

	"task-service/entity"
)

type AuthService interface {
	Register(ctx context.Context, email, password string) (entity.User, error)
	Login(ctx context.Context, email, password string) (entity.Token, error)
	ParseToken(token string) (int64, error)
}

type TaskService interface {
	Create(ctx context.Context, userID int64, in entity.TaskCreate) (entity.Task, error)
	Get(ctx context.Context, id int64) (entity.Task, error)
	List(ctx context.Context, filter entity.TaskFilter) (entity.TaskPage, error)
	Update(ctx context.Context, userID, id int64, upd entity.TaskUpdate) (entity.Task, error)
	Delete(ctx context.Context, userID, id int64) error
}

type Handler struct {
	auth   AuthService
	tasks  TaskService
	logger *slog.Logger
}

func New(auth AuthService, tasks TaskService, logger *slog.Logger) *Handler {
	return &Handler{
		auth:   auth,
		tasks:  tasks,
		logger: logger,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", h.health)

	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)

	mux.Handle("POST /api/v1/tasks", h.authenticate(h.createTask))
	mux.Handle("GET /api/v1/tasks", h.authenticate(h.listTasks))
	mux.Handle("GET /api/v1/tasks/{id}", h.authenticate(h.getTask))
	mux.Handle("PATCH /api/v1/tasks/{id}", h.authenticate(h.updateTask))
	mux.Handle("DELETE /api/v1/tasks/{id}", h.authenticate(h.deleteTask))

	return h.logRequests(h.recoverPanic(secureHeaders(mux)))
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

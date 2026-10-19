package http

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"task-service/entity"
)

type createTaskRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Deadline    time.Time `json:"deadline"`
	AssigneeID  *int64    `json:"assignee_id"`
}

type updateTaskRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Deadline    *time.Time `json:"deadline"`
	Status      *string    `json:"status"`
	AssigneeID  *int64     `json:"assignee_id"`
}

type taskResponse struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Deadline    time.Time `json:"deadline"`
	CreatorID   int64     `json:"creator_id"`
	AssigneeID  int64     `json:"assignee_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type taskListResponse struct {
	Items  []taskResponse `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func toTaskResponse(t entity.Task) taskResponse {
	return taskResponse{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		Deadline:    t.Deadline.UTC(),
		CreatorID:   t.CreatorID,
		AssigneeID:  t.AssigneeID,
		CreatedAt:   t.CreatedAt.UTC(),
		UpdatedAt:   t.UpdatedAt.UTC(),
	}
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	task, err := h.tasks.Create(r.Context(), userIDFromContext(r.Context()), entity.TaskCreate{
		Title:       req.Title,
		Description: req.Description,
		Deadline:    req.Deadline,
		AssigneeID:  req.AssigneeID,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toTaskResponse(task))
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	filter, err := parseTaskFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := h.tasks.List(r.Context(), filter)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	resp := taskListResponse{
		Items:  make([]taskResponse, 0, len(page.Items)),
		Total:  page.Total,
		Limit:  page.Limit,
		Offset: page.Offset,
	}
	for _, t := range page.Items {
		resp.Items = append(resp.Items, toTaskResponse(t))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskIDFromPath(w, r)
	if !ok {
		return
	}

	task, err := h.tasks.Get(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponse(task))
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskIDFromPath(w, r)
	if !ok {
		return
	}

	var req updateTaskRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	upd := entity.TaskUpdate{
		Title:       req.Title,
		Description: req.Description,
		Deadline:    req.Deadline,
		AssigneeID:  req.AssigneeID,
	}
	if req.Status != nil {
		status := entity.Status(*req.Status)
		upd.Status = &status
	}

	task, err := h.tasks.Update(r.Context(), userIDFromContext(r.Context()), id, upd)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponse(task))
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskIDFromPath(w, r)
	if !ok {
		return
	}

	if err := h.tasks.Delete(r.Context(), userIDFromContext(r.Context()), id); err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func taskIDFromPath(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return 0, false
	}
	return id, true
}

type queryError struct {
	param string
}

func (e *queryError) Error() string {
	return "invalid query parameter: " + e.param
}

func parseTaskFilter(q url.Values) (entity.TaskFilter, error) {
	var (
		f   entity.TaskFilter
		err error
	)

	if v := q.Get("status"); v != "" {
		status := entity.Status(v)
		f.Status = &status
	}
	if v := q.Get("assignee_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return f, &queryError{param: "assignee_id"}
		}
		f.AssigneeID = &id
	}
	if f.DeadlineFrom, err = parseTimeParam(q, "deadline_from"); err != nil {
		return f, err
	}
	if f.DeadlineTo, err = parseTimeParam(q, "deadline_to"); err != nil {
		return f, err
	}
	if f.Limit, err = parseIntParam(q, "limit"); err != nil {
		return f, err
	}
	if f.Offset, err = parseIntParam(q, "offset"); err != nil {
		return f, err
	}

	return f, nil
}

func parseTimeParam(q url.Values, name string) (*time.Time, error) {
	v := q.Get(name)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, &queryError{param: name}
	}
	return &t, nil
}

func parseIntParam(q url.Values, name string) (int, error) {
	v := q.Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, &queryError{param: name}
	}
	return n, nil
}

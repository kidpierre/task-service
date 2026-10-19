package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"task-service/entity"
)

const maxBodyBytes = 1 << 20

var errorStatuses = []struct {
	err    error
	status int
}{
	{entity.ErrTaskNotFound, http.StatusNotFound},
	{entity.ErrForbidden, http.StatusForbidden},
	{entity.ErrEmailTaken, http.StatusConflict},
	{entity.ErrInvalidCredentials, http.StatusUnauthorized},
	{entity.ErrAssigneeNotFound, http.StatusUnprocessableEntity},
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *entity.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusBadRequest, ve.Message)
		return
	}

	for _, e := range errorStatuses {
		if errors.Is(err, e.err) {
			writeError(w, e.status, e.err.Error())
			return
		}
	}

	h.logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid request body")
}

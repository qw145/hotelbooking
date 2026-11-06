package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/qw145/hotelbooking/service"
)

const (
	CodeValidation         = "validation_failed"
	CodeNotFound           = "not_found"
	CodeRoomUnavailable    = "room_unavailable"
	CodeNoAvailability     = "no_availability"
	CodeCapacityExceeded   = "capacity_exceeded"
	CodeConflict           = "conflict"
	CodeInternal           = "internal_error"
	CodeNotImplemented     = "not_implemented"
	CodeMethodNotAllowed   = "method_not_allowed"
	CodePayloadTooLarge    = "payload_too_large"
	CodeServiceUnavailable = "service_unavailable"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	// Code is a stable identifier clients can branch on, e.g. "room_unavailable".
	Code string `json:"code"`
	// Message explains what went wrong, in terms safe to show an end user.
	Message string `json:"message"`
	// Details lists per-field problems for validation failures.
	Details []string `json:"details,omitempty"`
}

// writeJSON serialises v and writes it with the given status.
func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	body, err := json.Marshal(v)
	if err != nil {
		log.Error("failed to encode response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"code":"internal_error","message":"failed to encode response"}}`))
		return
	}

	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		log.Debug("failed to write response body", "error", err)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	message := err.Error()
	status, code := func(err error) (status int, code string) {
		switch {
		case errors.Is(err, service.ErrNotFound):
			return http.StatusNotFound, CodeNotFound
		case errors.Is(err, service.ErrCapacityExceeded):
			return http.StatusUnprocessableEntity, CodeCapacityExceeded
		case errors.Is(err, service.ErrRoomUnavailable):
			return http.StatusConflict, CodeRoomUnavailable
		case errors.Is(err, service.ErrNoAvailability):
			return http.StatusConflict, CodeNoAvailability
		case errors.Is(err, service.ErrReferenceConflict):
			return http.StatusConflict, CodeConflict
		case errors.Is(err, service.ErrValidation):
			return http.StatusBadRequest, CodeValidation
		default:
			return http.StatusInternalServerError, CodeInternal
		}
	}(err)

	if status >= http.StatusInternalServerError {
		log.ErrorContext(r.Context(), "request failed",
			"error", err, "status", status, "code", code, "path", r.URL.Path)
	} else {
		log.DebugContext(r.Context(), "request rejected",
			"error", err, "status", status, "code", code, "path", r.URL.Path)
	}

	writeJSON(w, log, status, ErrorResponse{Error: ErrorBody{
		Code:    code,
		Message: message,
	}})
}

func ValidationError(details []string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeValidation,
			Message: "one or more fields are invalid",
			Details: details,
		},
	}
}

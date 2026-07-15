package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Status is the small fixed set of outcome codes carried in every APIResponse.
// Zero means success; a non-zero value classifies a failure, grouped into
// ranges by category (100s client/validation, 200s auth, 300s resource-state,
// 400s capacity/concurrency, 500s server) so new codes can slot into the right
// category without renumbering existing ones. This is richer than a bare
// success/fail bool: callers can branch on the failure class without
// string-matching the error message.
type Status int

const (
	StatusOK      Status = 0
	StatusFailure Status = 1 // generic/unclassified failure

	// 100-199: client/validation
	StatusInvalidRequest      Status = 100 // malformed/invalid request (validation failure)
	StatusUnsupportedResource Status = 101 // unsupported resource type
	StatusMethodNotAllowed    Status = 102 // wrong HTTP method for this route

	// 200-299: auth
	StatusUnauthorized Status = 200 // missing/invalid credentials or token
	StatusForbidden    Status = 201 // authenticated but not permitted

	// 300-399: resource-state
	StatusNotFound           Status = 300 // resource does not exist
	StatusConflict           Status = 301 // resource already exists / state conflict
	StatusInfraAlreadyExists Status = 302 // reserved: infra-specific conflict, not yet emitted
	StatusMountCooldown      Status = 303 // reserved: mount attempted during cooldown, not yet emitted

	// 400-499: capacity/concurrency
	StatusCapacityRace         Status = 400 // reserved: lost race on capacity allocation, not yet emitted
	StatusInsufficientCapacity Status = 401 // reserved: not enough capacity to satisfy request, not yet emitted

	// 500-599: server
	StatusInternalError Status = 500 // internal/unexpected error
	StatusUnavailable   Status = 501 // reserved: transient/retry-safe failure, not yet emitted
)

// httpStatus maps a Status to the HTTP status code used for the user/auth
// endpoints, which answer with a real HTTP status per Status (see WriteError).
// Non-user endpoints always answer HTTP 200 regardless of Status (see
// WriteMethodError).
func (s Status) httpStatus() int {
	switch s {
	case StatusOK:
		return http.StatusOK
	case StatusFailure:
		return http.StatusBadRequest
	case StatusInvalidRequest:
		return http.StatusBadRequest
	case StatusUnsupportedResource:
		return http.StatusBadRequest
	case StatusMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case StatusUnauthorized:
		return http.StatusUnauthorized
	case StatusForbidden:
		return http.StatusForbidden
	case StatusNotFound:
		return http.StatusNotFound
	case StatusConflict:
		return http.StatusConflict
	case StatusInfraAlreadyExists:
		return http.StatusConflict
	case StatusMountCooldown:
		return http.StatusTooManyRequests
	case StatusCapacityRace:
		return http.StatusConflict
	case StatusInsufficientCapacity:
		return http.StatusInsufficientStorage
	case StatusInternalError:
		return http.StatusInternalServerError
	case StatusUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// APIResponse is the unified response envelope for the REST API. Every endpoint
// returns this shape:
//
//   - Status is the verdict: 0 on success, non-zero on failure, grouped by
//     range (see the Status constants for what each code means).
//   - Error carries a human-readable message when Status is non-zero (omitted
//     on success).
//   - Payload carries the method-specific result on success (omitted on error).
//
// For non-user (infra/vdev/resource) endpoints the HTTP status is always 200
// and Status is the sole verdict: a method-level error is reported as
// {status:!=0, error:"..."} with HTTP 200, never an HTTP error code. The
// user/auth endpoints still use real HTTP status codes (see WriteError), in
// addition to the same Status code in the body.
type APIResponse[T any] struct {
	Status  Status `json:"status"`
	Error   string `json:"error,omitempty"`
	Payload *T     `json:"payload,omitempty"`
}

// ErrorResponse is the payload-less form of the envelope, used to write errors
// (no payload) and to decode an error body without committing to a payload type.
// It is wire-compatible with APIResponse[T] on the error path ({status:!=0,
// error:"..."}; payload omitted).
type ErrorResponse struct {
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}

// WriteDataStatus writes a success envelope ({status:0, payload:{...}}) with
// an explicit HTTP status code. Used by the user/auth endpoints that return a
// non-200 success status (e.g. 201 Created).
func WriteDataStatus[T any](w http.ResponseWriter, httpStatus int, payload T) {
	WriteJSON(w, httpStatus, APIResponse[T]{Status: StatusOK, Payload: &payload})
}

// WriteData writes a 200 success envelope carrying payload
// ({status:0, payload:{...}}). Use for endpoint success responses.
func WriteData[T any](w http.ResponseWriter, payload T) {
	WriteDataStatus(w, http.StatusOK, payload)
}

// WriteMethodError writes a 200 envelope carrying a method (business) error
// ({status:code, error:"..."}) classified by code. The method ran and reported
// failure; the verdict travels in the body, not the HTTP status. Use for
// non-user endpoints — they never emit an HTTP error code. (Contrast
// WriteError, which sets a real status code for the user/auth endpoints and
// for protocol rejections.)
func WriteMethodError(w http.ResponseWriter, code Status, format string, args ...any) {
	WriteJSON(w, http.StatusOK, ErrorResponse{
		Status: code,
		Error:  fmt.Sprintf(format, args...),
	})
}

// WriteJSON serializes v as JSON and writes it with the given HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Header/status already sent; nothing actionable remains but to log
		// upstream. Return so callers don't double-write.
		return
	}
}

// WriteError writes a generic {status:code, error:msg} body, using code's
// mapped HTTP status code (see Status.httpStatus). Handlers that need to
// populate endpoint-specific fields should build their typed response with
// the desired Status and call WriteJSON instead.
func WriteError(w http.ResponseWriter, code Status, format string, args ...any) {
	WriteJSON(w, code.httpStatus(), ErrorResponse{
		Status: code,
		Error:  fmt.Sprintf(format, args...),
	})
}

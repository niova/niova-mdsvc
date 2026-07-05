package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// APIResponse is the unified response envelope for the REST API. Every endpoint
// returns this shape:
//
//   - Success is the verdict.
//   - Error carries a human-readable message when Success is false (omitted on
//     success).
//   - Payload carries the method-specific result on success (omitted on error).
//
// For non-user (infra/vdev/resource) endpoints the HTTP status is always 200 and
// Success is the sole verdict: a method-level error is reported as
// {success:false, error:"..."} with HTTP 200, never an HTTP error code. The
// user/auth endpoints still use real HTTP status codes (see WriteError).
type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Payload *T     `json:"payload,omitempty"`
}

// ErrorResponse is the payload-less form of the envelope, used to write errors
// (no payload) and to decode an error body without committing to a payload type.
// It is wire-compatible with APIResponse[T] on the error path ({success:false,
// error:"..."}; payload omitted).
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// WriteDataStatus writes a success envelope ({success:true, payload:{...}}) with
// an explicit HTTP status code. Used by the user/auth endpoints that return a
// non-200 success status (e.g. 201 Created).
func WriteDataStatus[T any](w http.ResponseWriter, status int, payload T) {
	WriteJSON(w, status, APIResponse[T]{Success: true, Payload: &payload})
}

// WriteData writes a 200 success envelope carrying payload
// ({success:true, payload:{...}}). Use for endpoint success responses.
func WriteData[T any](w http.ResponseWriter, payload T) {
	WriteDataStatus(w, http.StatusOK, payload)
}

// WriteMethodError writes a 200 envelope carrying a method (business) error
// ({success:false, error:"..."}). The method ran and reported failure; the
// verdict travels in the body, not the HTTP status. Use for non-user endpoints —
// they never emit an HTTP error code. (Contrast WriteError, which sets a real
// status code for the user/auth endpoints and for protocol rejections.)
func WriteMethodError(w http.ResponseWriter, format string, args ...any) {
	WriteJSON(w, http.StatusOK, ErrorResponse{
		Success: false,
		Error:   fmt.Sprintf(format, args...),
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

// WriteError writes a generic {success:false, error:msg} body with the given
// status code. Handlers that need to populate endpoint-specific fields should
// build their typed response with Success=false and call WriteJSON instead.
func WriteError(w http.ResponseWriter, status int, format string, args ...any) {
	WriteJSON(w, status, ErrorResponse{
		Success: false,
		Error:   fmt.Sprintf(format, args...),
	})
}

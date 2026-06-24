package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ErrorResponse is the generic error body used when an endpoint-specific
// response type is not yet available (e.g. a malformed request rejected before
// dispatch). It matches the {success:false, error:"..."} shape shared by every
// endpoint's typed response.
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
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

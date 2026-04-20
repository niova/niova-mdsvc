package libctlplanefuncs

import "errors"

type CPErrCode string

const (
	ErrAuth     CPErrCode = "AUTH_ERROR" // token invalid/expired or RBAC denied
	ErrFunc     CPErrCode = "FUNC_ERROR" // business logic / handler error
	ErrInternal CPErrCode = "INTERNAL"   // transport/encoding/unexpected error
)

// CPError carries a human-readable error message from the server.
type CPError struct {
	Message string    `xml:"Message"`
	Code    CPErrCode `xml:"Code"`
}

// Pagination holds cursor-based pagination parameters for read requests and responses.
//
// In a request, SeqNo is the zero-based start offset and PageSize is the maximum
// number of top-level items to return (0 means return everything).
//
// In a response, SeqNo is the start offset for the next page and HasMore reports
// whether additional pages remain.
type Pagination struct {
	SeqNo      uint64 `xml:"SeqNo"`      // Request: start offset; Response: next-page offset
	PageSize   uint64 `xml:"PageSize"`   // Max items per page (0 = no pagination)
	HasMore    bool   `xml:"HasMore"`    // Response only: true when more pages follow
	Consistent bool   `xml:"Consistent"` // If true, the read is consistent with the last read
}

// CPReq is the unified request envelope for all control plane operations.
// It separates auth, pagination, and function-specific payload.
type CPReq struct {
	Token   string      `xml:"Token"`   // Auth JWT token
	Page    *Pagination `xml:"Page"`    // Pagination parameters (used only for list/read operations)
	Payload any         `xml:"Payload"` // Function-specific request struct (Nisd, Rack, GetReq, etc.)
}

// CPResp is the unified response envelope for all control plane operations.
type CPResp struct {
	Error   *CPError    `xml:"Error"`   // nil on success
	Page    *Pagination `xml:"Page"`    // Pagination metadata (returned on list operations)
	Payload any         `xml:"Payload"` // Function-specific response
}

// Err returns a Go error from the response, or nil on success.
func (r *CPResp) Err() error {
	if r.Error == nil {
		return nil
	}
	return errors.New(r.Error.Message)
}

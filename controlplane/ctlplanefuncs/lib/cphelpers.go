package libctlplanefuncs

import (
	pmCmn "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	funclib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/common"
)

// EncodeResponse encodes a successful CPResp with the given payload.
func EncodeResponse(payload any) (any, error) {
	return pmCmn.Encoder(pmCmn.GOB, CPResp{Payload: payload})
}

// EncodePagedResponse encodes a successful CPResp with a payload and pagination metadata.
// page must not be nil; it carries the next-page offset and HasMore flag back to the client.
func EncodePagedResponse(payload any, page *Pagination) (any, error) {
	return pmCmn.Encoder(pmCmn.GOB, CPResp{Payload: payload, Page: page})
}

func encodeErrorResp(code CPErrCode, msg string) (any, error) {
	return pmCmn.Encoder(pmCmn.GOB, CPResp{
		Error: &CPError{
			Code:    code,
			Message: msg,
		},
	})
}

// FuncError encodes a CPResp with ErrFunc for business-logic errors.
func FuncError(err error) (any, error) {
	if err == nil {
		return nil, nil
	}
	return encodeErrorResp(ErrFunc, err.Error())
}

// AuthError encodes a CPResp with ErrAuth for authentication/authorization failures.
func AuthError(err error) (any, error) {
	if err == nil {
		return nil, nil
	}
	return encodeErrorResp(ErrAuth, err.Error())
}

// InternalError encodes a CPResp with ErrInternal for unexpected internal failures.
func InternalError(err error) (any, error) {
	if err == nil {
		return nil, nil
	}
	return encodeErrorResp(ErrInternal, err.Error())
}

// WPError returns a GOB-encoded FuncIntrm carrying a CPResp error.
// Write-prep functions must return FuncIntrm (not CPResp)
// ApplyFunc detects the embedded CPResp and forwards it.
func WPError(code CPErrCode, err error) (any, error) {
	cpResp := CPResp{
		Error: &CPError{
			Code:    code,
			Message: err.Error(),
		},
	}
	return pmCmn.Encoder(pmCmn.GOB, funclib.FuncIntrm{Response: cpResp})
}

// WPAuthError wraps an auth failure for use in write-prep functions.
func WPAuthError(err error) (any, error) {
	return WPError(ErrAuth, err)
}

// WPFuncError wraps a business-logic failure for use in write-prep functions.
func WPFuncError(err error) (any, error) {
	return WPError(ErrFunc, err)
}

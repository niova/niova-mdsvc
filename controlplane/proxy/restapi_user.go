package main

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

// User auth/management REST handlers. They expose the existing /func user ops
// (LoginAPI/PutUserAPI/GetUserAPI) over REST, mirroring the TiDB user handler
// skeleton (login + user CRUD) while keeping niova's {success,error} envelope
// and secret-key model. Writes (create/update) require the X-RNCUI header like
// every other niova REST write; login and reads forward the bearer token so the
// control plane can enforce RBAC.

// statusForUserMsg maps a PutUser error message (returned in UserResp.Error) to
// an HTTP status. PutUser reports business errors inside a successful CPResp
// rather than a structured CPError, so its messages are classified here.
func statusForUserMsg(msg string) int {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "not found"):
		return http.StatusNotFound
	case strings.Contains(m, "already exists"):
		return http.StatusConflict
	case strings.Contains(m, "permission denied"), strings.Contains(m, "not authorized"),
		strings.Contains(m, "only admin"), strings.Contains(m, "reserved"):
		return http.StatusForbidden
	case strings.Contains(m, "token"), strings.Contains(m, "invalid credentials"),
		strings.Contains(m, "unauthorized"):
		return http.StatusUnauthorized
	default:
		return http.StatusBadRequest
	}
}

// userDataFromResp projects a control-plane UserResp into the REST UserData.
func userDataFromResp(u userlib.UserResp) restapi.UserData {
	return restapi.UserData{
		ID:       u.UserID,
		Username: u.Username,
		Role:     u.UserRole,
		Status:   int(u.Status),
	}
}

// handleLogin backs POST /users/login (LoginAPI, read). niova authenticates with
// username + secret key; the REST password field carries the secret key.
func (handler *proxyHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.LoginRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Username) == "" || req.Password == "" {
		restapi.WriteError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	cpReq := cpLib.CPReq{Payload: cpLib.GetReq{ID: req.Username + ":" + req.Password}}
	var lr userlib.LoginResp
	cpResp, err := handler.callFunc(userlib.LoginAPI, cpReq, false, "", 0, &lr)
	if err != nil || cpErrOf(cpResp) != nil || !lr.Success {
		restapi.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	restapi.WriteJSON(w, http.StatusOK, restapi.LoginResponse{
		Success:     true,
		AccessToken: lr.AccessToken,
		TokenType:   lr.TokenType,
		ExpiresIn:   lr.ExpiresIn,
		UserID:      lr.UserID,
		Username:    lr.Username,
		Role:        lr.UserRole,
		IsAdmin:     lr.IsAdmin,
	})
}

// handleCreateUser backs POST /api/users (PutUserAPI, create). niova generates
// the secret key server-side and returns it once in secret_key. Admin-only
// (enforced by the control plane); write, so it requires X-RNCUI.
func (handler *proxyHandler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.CreateUserRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		restapi.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}
	if req.Role != "" && req.Role != userlib.DefaultUserRole && req.Role != userlib.AdminUserRole {
		restapi.WriteError(w, http.StatusBadRequest, "role must be one of: %s %s",
			userlib.DefaultUserRole, userlib.AdminUserRole)
		return
	}
	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteError(w, http.StatusBadRequest, "%s", err.Error())
		return
	}

	cpReq := cpLib.CPReq{
		Token:   tokenFromRequest(r),
		Payload: userlib.UserReq{Username: req.Username, IsAdmin: req.Role == userlib.AdminUserRole},
	}
	var resp userlib.UserResp
	cpResp, err := handler.callFunc(userlib.PutUserAPI, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	if !resp.Success {
		restapi.WriteError(w, statusForUserMsg(resp.Error), "%s", resp.Error)
		return
	}
	restapi.WriteJSON(w, http.StatusCreated, restapi.UserResponse{
		Success:   true,
		UserData:  userDataFromResp(resp),
		SecretKey: resp.SecretKey,
	})
}

// handleListUsers backs GET /api/users (GetUserAPI, all). Admin-only server-side.
func (handler *proxyHandler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	cpReq := cpLib.CPReq{Token: tokenFromRequest(r), Payload: userlib.GetReq{}}
	var users []userlib.UserResp
	cpResp, err := handler.callFunc(userlib.GetUserAPI, cpReq, false, "", 0, &users)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	out := make([]restapi.UserData, 0, len(users))
	for _, u := range users {
		out = append(out, userDataFromResp(u))
	}
	restapi.WriteJSON(w, http.StatusOK, restapi.ListUsersResponse{Success: true, Users: out})
}

// handleGetUser backs GET /api/users/{id} (GetUserAPI by id). Self-or-admin
// access is enforced by the control plane.
func (handler *proxyHandler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if uuid.Validate(id) != nil {
		restapi.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	cpReq := cpLib.CPReq{Token: tokenFromRequest(r), Payload: userlib.GetReq{UserID: id}}
	var users []userlib.UserResp
	cpResp, err := handler.callFunc(userlib.GetUserAPI, cpReq, false, "", 0, &users)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	if len(users) == 0 {
		restapi.WriteError(w, http.StatusNotFound, "user not found: %s", id)
		return
	}
	restapi.WriteJSON(w, http.StatusOK, restapi.UserResponse{
		Success:  true,
		UserData: userDataFromResp(users[0]),
	})
}

// handleUpdateUser backs PUT /api/users/{id} (PutUserAPI, update). niova supports
// changing the username and (admin only) the secret key; role/status changes are
// not supported by the control plane. Write, so it requires X-RNCUI.
func (handler *proxyHandler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPut) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if uuid.Validate(id) != nil {
		restapi.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req restapi.UpdateUserRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" && req.NewSecretKey == "" {
		restapi.WriteError(w, http.StatusBadRequest,
			"nothing to update: provide username and/or new_secret_key")
		return
	}
	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteError(w, http.StatusBadRequest, "%s", err.Error())
		return
	}

	cpReq := cpLib.CPReq{
		Token: tokenFromRequest(r),
		Payload: userlib.UserReq{
			UserID:       id,
			Username:     req.Username,
			NewSecretKey: req.NewSecretKey,
			IsUpdate:     true,
		},
	}
	var resp userlib.UserResp
	cpResp, err := handler.callFunc(userlib.PutUserAPI, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	if !resp.Success {
		restapi.WriteError(w, statusForUserMsg(resp.Error), "%s", resp.Error)
		return
	}
	restapi.WriteJSON(w, http.StatusOK, restapi.UserResponse{
		Success:   true,
		UserData:  userDataFromResp(resp),
		SecretKey: resp.SecretKey, // populated only when the secret key was changed
	})
}

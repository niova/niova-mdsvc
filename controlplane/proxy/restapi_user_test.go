package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

const testUserUUID = "550e8400-e29b-41d4-a716-446655440000"

// ---- POST /users/login ----

func TestHandleLogin_Success(t *testing.T) {
	var cap capturedCall
	reply := gobReply(t, userlib.LoginResp{
		AccessToken: "tok-1", TokenType: "Bearer", ExpiresIn: 900,
		UserID: "u1", Username: "alice", UserRole: "admin", IsAdmin: true, Success: true,
	})
	h := newFakeHandler(reply, nil, &cap)
	rr := httptest.NewRecorder()
	h.handleLogin(rr, writeReq(t, http.MethodPost, "/users/login", `{"username":"alice","password":"secret"}`, ""))
	got := decodePayload[restapi.LoginPayload](t, rr)
	if got.AccessToken != "tok-1" || got.Role != "admin" || !got.IsAdmin {
		t.Fatalf("payload = %+v", got)
	}
	// niova authenticates with username:secretKey.
	gr, ok := cap.cpReq.Payload.(cpLib.GetReq)
	if !ok || gr.ID != "alice:secret" {
		t.Fatalf("login payload = %#v", cap.cpReq.Payload)
	}
	if cap.name != userlib.LoginAPI || cap.isWrite {
		t.Fatalf("op = %q isWrite=%v", cap.name, cap.isWrite)
	}
}

func TestHandleLogin_BadCreds(t *testing.T) {
	h := newFakeHandler(gobFuncErr(t, "invalid credentials"), nil, nil)
	rr := httptest.NewRecorder()
	h.handleLogin(rr, writeReq(t, http.MethodPost, "/users/login", `{"username":"alice","password":"nope"}`, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestHandleLogin_MissingFields(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handleLogin(rr, writeReq(t, http.MethodPost, "/users/login", `{"username":"alice"}`, ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

// ---- POST /api/users ----

func TestHandleCreateUser_Success(t *testing.T) {
	var cap capturedCall
	reply := gobReply(t, userlib.UserResp{
		UserID: "u1", Username: "alice", UserRole: "user", SecretKey: "generated-key", Success: true,
	})
	h := newFakeHandler(reply, nil, &cap)
	rr := httptest.NewRecorder()
	h.handleCreateUser(rr, writeReq(t, http.MethodPost, "/api/users", `{"username":"alice"}`, "r1"))
	got := decodePayloadCode[restapi.UserPayload](t, rr, http.StatusCreated)
	if got.ID != "u1" || got.Username != "alice" || got.SecretKey != "generated-key" {
		t.Fatalf("payload = %+v", got)
	}
	ur, ok := cap.cpReq.Payload.(userlib.UserReq)
	if !ok || ur.Username != "alice" || ur.IsAdmin {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
	if cap.name != userlib.PutUserAPI || !cap.isWrite || cap.rncui != "r1" {
		t.Fatalf("captured = %+v", cap)
	}
}

func TestHandleCreateUser_AdminRole(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, userlib.UserResp{UserID: "u1", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	h.handleCreateUser(rr, writeReq(t, http.MethodPost, "/api/users", `{"username":"boss","role":"admin"}`, "r1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	ur := cap.cpReq.Payload.(userlib.UserReq)
	if !ur.IsAdmin {
		t.Fatalf("expected IsAdmin=true, got %#v", ur)
	}
}

func TestHandleCreateUser_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, userlib.UserResp{Success: true}), nil, nil)
	rr := httptest.NewRecorder()
	h.handleCreateUser(rr, writeReq(t, http.MethodPost, "/api/users", `{"username":"alice"}`, ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateUser_BadRole(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handleCreateUser(rr, writeReq(t, http.MethodPost, "/api/users", `{"username":"alice","role":"superuser"}`, "r1"))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateUser_Conflict(t *testing.T) {
	reply := gobReply(t, userlib.UserResp{Success: false, Error: "username 'alice' already exists"})
	h := newFakeHandler(reply, nil, nil)
	rr := httptest.NewRecorder()
	h.handleCreateUser(rr, writeReq(t, http.MethodPost, "/api/users", `{"username":"alice"}`, "r1"))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
}

// ---- POST /users/admin ----

func TestHandleCreateAdminUser_Success(t *testing.T) {
	var cap capturedCall
	reply := gobReply(t, userlib.UserResp{
		UserID: "admin-id", Username: "admin", UserRole: "admin", SecretKey: "admin-secret", Success: true,
	})
	h := newFakeHandler(reply, nil, &cap)
	rr := httptest.NewRecorder()
	h.handleCreateAdminUser(rr, writeReq(t, http.MethodPost, "/users/admin", `{"username":"admin"}`, "r1"))
	got := decodePayload[restapi.UserPayload](t, rr)
	if got.SecretKey != "admin-secret" {
		t.Fatalf("payload = %+v", got)
	}
	// Bootstrap carries no token; op is AdminUserAPI (write).
	if cap.name != userlib.AdminUserAPI || !cap.isWrite || cap.cpReq.Token != "" {
		t.Fatalf("captured = %+v", cap)
	}
	if ur, ok := cap.cpReq.Payload.(userlib.UserReq); !ok || ur.Username != "admin" {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
}

func TestHandleCreateAdminUser_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, userlib.UserResp{Success: true}), nil, nil)
	rr := httptest.NewRecorder()
	h.handleCreateAdminUser(rr, writeReq(t, http.MethodPost, "/users/admin", `{"username":"admin"}`, ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

// ---- GET /api/users ----

func TestHandleListUsers_Success(t *testing.T) {
	reply := gobReply(t, []userlib.UserResp{
		{UserID: "u1", Username: "alice", UserRole: "user"},
		{UserID: "u2", Username: "admin", UserRole: "admin"},
	})
	h := newFakeHandler(reply, nil, nil)
	rr := httptest.NewRecorder()
	h.handleListUsers(rr, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	got := decodePayload[restapi.ListUsersPayload](t, rr)
	if len(got.Users) != 2 || got.Users[0].ID != "u1" || got.Users[1].Role != "admin" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestHandleListUsers_ByUsername(t *testing.T) {
	var cap capturedCall
	reply := gobReply(t, []userlib.UserResp{{UserID: "u1", Username: "alice", UserRole: "user"}})
	h := newFakeHandler(reply, nil, &cap)
	rr := httptest.NewRecorder()
	h.handleListUsers(rr, httptest.NewRequest(http.MethodGet, "/api/users?username=alice", nil))
	got := decodePayload[restapi.ListUsersPayload](t, rr)
	if len(got.Users) != 1 || got.Users[0].Username != "alice" {
		t.Fatalf("payload = %+v", got)
	}
	// ?username= must forward as a GetReq username filter.
	gr, ok := cap.cpReq.Payload.(userlib.GetReq)
	if !ok || gr.Username != "alice" {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
}

// ---- GET /api/users/{id} ----

func TestHandleGetUser_Success(t *testing.T) {
	reply := gobReply(t, []userlib.UserResp{{UserID: testUserUUID, Username: "alice", UserRole: "user", SecretKey: "sk"}})
	h := newFakeHandler(reply, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/"+testUserUUID, nil)
	req.SetPathValue("id", testUserUUID)
	h.handleGetUser(rr, req)
	got := decodePayload[restapi.UserPayload](t, rr)
	// Single-entity fetch returns the decrypted secret key.
	if got.ID != testUserUUID || got.Username != "alice" || got.SecretKey != "sk" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestHandleGetUser_InvalidID(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	h.handleGetUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetUser_NotFound(t *testing.T) {
	h := newFakeHandler(gobReply(t, []userlib.UserResp{}), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/"+testUserUUID, nil)
	req.SetPathValue("id", testUserUUID)
	h.handleGetUser(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// ---- PUT /api/users/{id} ----

func TestHandleUpdateUser_Success(t *testing.T) {
	var cap capturedCall
	reply := gobReply(t, userlib.UserResp{UserID: testUserUUID, Username: "alice2", UserRole: "user", Success: true})
	h := newFakeHandler(reply, nil, &cap)
	rr := httptest.NewRecorder()
	req := writeReq(t, http.MethodPut, "/api/users/"+testUserUUID, `{"username":"alice2"}`, "r1")
	req.SetPathValue("id", testUserUUID)
	h.handleUpdateUser(rr, req)
	got := decodePayload[restapi.UserPayload](t, rr)
	if got.Username != "alice2" {
		t.Fatalf("payload = %+v", got)
	}
	ur, ok := cap.cpReq.Payload.(userlib.UserReq)
	if !ok || !ur.IsUpdate || ur.UserID != testUserUUID || ur.Username != "alice2" {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
}

func TestHandleUpdateUser_NothingToUpdate(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := writeReq(t, http.MethodPut, "/api/users/"+testUserUUID, `{}`, "r1")
	req.SetPathValue("id", testUserUUID)
	h.handleUpdateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleUpdateUser_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, userlib.UserResp{Success: true}), nil, nil)
	rr := httptest.NewRecorder()
	req := writeReq(t, http.MethodPut, "/api/users/"+testUserUUID, `{"username":"x"}`, "")
	req.SetPathValue("id", testUserUUID)
	h.handleUpdateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

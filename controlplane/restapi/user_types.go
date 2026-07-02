package restapi

// This file defines the flat snake_case DTOs for the user auth/management
// endpoints. They mirror the TiDB user handler skeleton (login + user CRUD) but
// keep niova's {success,error} envelope and adapt to niova's secret-key model:
//
//   - niova has no password field: CreateUser generates a secret key server-side
//     and returns it once (UserResponse.secret_key). Login authenticates with
//     username + that secret key (sent in the password field).
//   - UpdateUser supports changing the username and (admin only) the secret key;
//     the control plane does not support role/status changes.

// ---- POST /users/login (LoginAPI) ----

// LoginRequest is the login body. password is the niova secret key.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success     bool   `json:"success"`
	AccessToken string `json:"access_token,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	ExpiresIn   int64  `json:"expires_in,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	Role        string `json:"role,omitempty"`
	IsAdmin     bool   `json:"is_admin,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ---- POST /api/users (PutUserAPI, create) ----

// CreateUserRequest is the create body. niova generates the secret key
// server-side (returned once in UserResponse.secret_key); there is no password
// input. role ("user"|"admin") selects whether the user is created as an admin.
type CreateUserRequest struct {
	Username string `json:"username"`
	Role     string `json:"role,omitempty"`
}

// UserData is the common user projection shared by create/get/list/update.
type UserData struct {
	ID       string `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
	Status   int    `json:"status"`
}

// UserResponse is the body for create/get/update. secret_key is populated only
// when the control plane returns a plaintext key (create, or admin secret-key
// update); it is never returned by reads.
type UserResponse struct {
	Success bool `json:"success"`
	UserData
	SecretKey string `json:"secret_key,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ---- GET /api/users (GetUserAPI, list) ----

type ListUsersResponse struct {
	Success bool       `json:"success"`
	Users   []UserData `json:"users"`
	Error   string     `json:"error,omitempty"`
}

// ---- PUT /api/users/{id} (PutUserAPI, update) ----

// UpdateUserRequest is the update body. niova supports changing the username and
// (admin only) the secret key; role/status changes are not supported server-side.
type UpdateUserRequest struct {
	Username     string `json:"username,omitempty"`
	NewSecretKey string `json:"new_secret_key,omitempty"`
}

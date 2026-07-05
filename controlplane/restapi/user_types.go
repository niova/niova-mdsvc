package restapi

// This file defines the snake_case request/payload DTOs for the user
// auth/management endpoints. They mirror the TiDB user handler skeleton (login +
// user CRUD), are carried in the shared APIResponse[T] envelope, and adapt to
// niova's secret-key model:
//
//   - niova has no password field: CreateUser generates a secret key server-side
//     and returns it once (UserPayload.secret_key). Login authenticates with
//     username + that secret key (sent in the password field).
//   - UpdateUser supports changing the username and (admin only) the secret key;
//     the control plane does not support role/status changes.
//
// Unlike the non-user endpoints, the user/auth endpoints keep real HTTP status
// codes for errors (e.g. 401 on bad login credentials); only the response shape
// is unified onto the envelope.

// ---- POST /users/login (LoginAPI) ----

// LoginRequest is the login body. password is the niova secret key.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginPayload is the payload of the login envelope (APIResponse[LoginPayload]).
type LoginPayload struct {
	AccessToken string `json:"access_token,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	ExpiresIn   int64  `json:"expires_in,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	Role        string `json:"role,omitempty"`
	IsAdmin     bool   `json:"is_admin,omitempty"`
}

// ---- POST /users/admin (AdminUserAPI, bootstrap) ----

// CreateAdminUserRequest bootstraps the singleton admin user. username must be
// "admin". secret_key is optional — the control plane uses a default when empty.
// This is a niova-only endpoint (TiDB has no admin-bootstrap route); it is
// unauthenticated because it must run before any admin/token exists.
type CreateAdminUserRequest struct {
	Username  string `json:"username"`
	SecretKey string `json:"secret_key,omitempty"`
}

// ---- POST /api/users (PutUserAPI, create) ----

// CreateUserRequest is the create body. niova generates the secret key
// server-side (returned once in UserPayload.secret_key); there is no password
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

// UserPayload is the payload for create/get/update (APIResponse[UserPayload]).
// secret_key is populated only when the control plane returns a plaintext key
// (create, or admin secret-key update); it is never returned by reads.
type UserPayload struct {
	UserData
	SecretKey string `json:"secret_key,omitempty"`
}

// ---- GET /api/users (GetUserAPI, list) ----

// ListUsersPayload is the payload of the list-users envelope.
type ListUsersPayload struct {
	Users []UserData `json:"users"`
}

// ---- PUT /api/users/{id} (PutUserAPI, update) ----

// UpdateUserRequest is the update body. niova supports changing the username and
// (admin only) the secret key; role/status changes are not supported server-side.
type UpdateUserRequest struct {
	Username     string `json:"username,omitempty"`
	NewSecretKey string `json:"new_secret_key,omitempty"`
}

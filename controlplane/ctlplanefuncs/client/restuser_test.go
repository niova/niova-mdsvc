package clictlplanefuncs

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

// restLogin exercises POST /users/login over REST and returns the decoded body.
// Login is a read; the X-RNCUI header and bearer token are ignored by the handler.
func restLogin(t *testing.T, c *CliCFuncs, username, password string) restapi.LoginResponse {
	t.Helper()
	body, err := json.Marshal(restapi.LoginRequest{Username: username, Password: password})
	require.NoError(t, err)
	respBody, err := c.restPost("/users/login", body, "")
	require.NoError(t, err, "REST login should succeed")
	var lr restapi.LoginResponse
	require.NoError(t, json.Unmarshal(respBody, &lr))
	return lr
}

// TestRESTUserLifecycle drives the REST user endpoints end-to-end against a live
// cluster: admin login -> create user -> login as that user -> list -> get ->
// update. Run with AUTH_ENABLED=true so getAdminToken bootstraps the admin.
func TestRESTUserLifecycle(t *testing.T) {
	c := newClient(t) // admin-token'd (auth enabled) and ensures the admin exists

	// 1. Admin login over REST (POST /users/login).
	adminLogin := restLogin(t, c, userlib.AdminUsername, testAdminSecret)
	require.True(t, adminLogin.Success)
	require.NotEmpty(t, adminLogin.AccessToken)
	t.Logf("admin REST login ok; is_admin=%v role=%s", adminLogin.IsAdmin, adminLogin.Role)

	// 2. Create a user (POST /api/users) — niova generates the secret key.
	username := "restuser-" + uuid.NewString()[:8]
	cuBody, err := json.Marshal(restapi.CreateUserRequest{Username: username, Role: "user"})
	require.NoError(t, err)
	created, err := c.restPost("/api/users", cuBody, c.nextRncui())
	require.NoError(t, err, "REST create user should succeed")
	var cu restapi.UserResponse
	require.NoError(t, json.Unmarshal(created, &cu))
	require.True(t, cu.Success)
	require.NotEmpty(t, cu.ID)
	require.NotEmpty(t, cu.SecretKey, "create must return the generated secret key")
	t.Logf("created user %s id=%s", username, cu.ID)

	// 3. Log in as the new user with the generated key (round-trips the key).
	userLogin := restLogin(t, c, username, cu.SecretKey)
	require.True(t, userLogin.Success)
	require.NotEmpty(t, userLogin.AccessToken)

	// 4. List users (GET /api/users) — admin only; the new user must appear.
	listBody, err := c.restGet("/api/users")
	require.NoError(t, err)
	var list restapi.ListUsersResponse
	require.NoError(t, json.Unmarshal(listBody, &list))
	require.True(t, list.Success)
	found := false
	for _, u := range list.Users {
		if u.ID == cu.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "created user %s should appear in GET /api/users", cu.ID)

	// 5. Get the user by id (GET /api/users/{id}).
	getBody, err := c.restGet("/api/users/" + cu.ID)
	require.NoError(t, err)
	var got restapi.UserResponse
	require.NoError(t, json.Unmarshal(getBody, &got))
	require.True(t, got.Success)
	assert.Equal(t, cu.ID, got.ID)
	assert.Equal(t, username, got.Username)

	// 6. Update the username (PUT /api/users/{id}).
	newName := username + "r"
	upBody, err := json.Marshal(restapi.UpdateUserRequest{Username: newName})
	require.NoError(t, err)
	updated, err := c.restPut("/api/users/"+cu.ID, upBody, c.nextRncui())
	require.NoError(t, err, "REST update user should succeed")
	var up restapi.UserResponse
	require.NoError(t, json.Unmarshal(updated, &up))
	require.True(t, up.Success)
	assert.Equal(t, newName, up.Username)
	t.Logf("updated username -> %s", up.Username)
}

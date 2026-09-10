package mdsvctidbclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

type loginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	TenantUUID string `json:"tenant_uuid,omitempty"`
}

type loginResp struct {
	AccessToken string `json:"access_token,omitempty"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
}

// jwtClaims is the subset of mdsvc-tidb's JWT payload niova-ctl cares about
// for display (confirmed against a live token: {"uid","sub","role","tv",...}).
type jwtClaims struct {
	UserID   string `json:"uid"`
	Username string `json:"sub"`
	Role     string `json:"role"`
}

// decodeJWTClaims reads a JWT's payload segment without verifying its
// signature — safe here because the token just came back from our own
// server over the login call we made; this is purely for display (role,
// user id), never used to authorize anything.
func decodeJWTClaims(token string) jwtClaims {
	var claims jwtClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims
	}
	_ = json.Unmarshal(payload, &claims)
	return claims
}

// Login authenticates against mdsvc-tidb's default tenant.
func (c *Client) Login(username, secretKey string) (*userlib.LoginResp, error) {
	return c.LoginWithTenant(username, secretKey, "")
}

// LoginWithTenant authenticates against mdsvc-tidb for a specific tenant UUID.
func (c *Client) LoginWithTenant(username, secretKey, tenantUUID string) (*userlib.LoginResp, error) {
	var resp loginResp
	req := loginRequest{
		Username:   username,
		Password:   secretKey,
		TenantUUID: tenantUUID,
	}
	if err := c.post("/users/login", req, &resp); err != nil {
		return nil, err
	}
	claims := decodeJWTClaims(resp.AccessToken)
	userID := claims.UserID
	role := claims.Role
	loginUsername := username
	if claims.Username != "" {
		loginUsername = claims.Username
	}
	return &userlib.LoginResp{
		AccessToken: resp.AccessToken,
		TokenType:   "Bearer",
		UserID:      userID,
		Username:    loginUsername,
		UserRole:    role,
		ExpiresIn:   int64(resp.ExpiresIn),
		IsAdmin:     role == "admin",
		Success:     true,
	}, nil
}

type wireUser struct {
	ID       string `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
	Status   int8   `json:"account_status"`
}

type listUsersResp struct {
	Users []wireUser `json:"users"`
}

// mapAccountStatus converts mdsvc-tidb's account_status into userlib.Status.
// Confirmed against a live server: the seeded admin/admin account (active)
// comes back as account_status=1, which is the OPPOSITE numbering from
// userlib.Status (StatusActive=0) — don't decode this field directly.
func mapAccountStatus(s int8) userlib.Status {
	if s == 1 {
		return userlib.StatusActive
	}
	return userlib.StatusInactive
}

func (c *Client) ListUsers(token string, _ userlib.GetReq) ([]userlib.UserResp, error) {
	c.SetToken(token)
	var resp listUsersResp
	if err := c.get("/api/users", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]userlib.UserResp, len(resp.Users))
	for i, u := range resp.Users {
		out[i] = userlib.UserResp{
			UserID:   u.ID,
			Username: u.Username,
			UserRole: u.Role,
			Status:   mapAccountStatus(u.Status),
			Success:  true,
		}
	}
	return out, nil
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

func (c *Client) CreateUser(token string, req *userlib.UserReq) (*userlib.UserResp, error) {
	c.SetToken(token)
	role := "user"
	if req.IsAdmin {
		role = "admin"
	}
	var w wireUser
	if err := c.post("/api/users", createUserRequest{Username: req.Username, Password: req.NewSecretKey, Role: role}, &w); err != nil {
		return nil, err
	}
	return &userlib.UserResp{
		UserID:    w.ID,
		Username:  w.Username,
		SecretKey: req.NewSecretKey,
		UserRole:  w.Role,
		Status:    mapAccountStatus(w.Status),
		Success:   true,
	}, nil
}

// CreateAdminUser has no mdsvc-tidb equivalent: the admin account is
// auto-seeded at server startup, so there's no unauthenticated bootstrap
// flow — callers should always have a token by now.
func (c *Client) CreateAdminUser(req *userlib.UserReq) (*userlib.UserResp, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("mdsvctidbclient: creating a user requires being logged in as an admin against this backend (no unauthenticated bootstrap)")
	}
	return c.CreateUser(c.Token, req)
}

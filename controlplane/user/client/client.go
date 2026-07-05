package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	pmCmn "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	sd "github.com/00pauln00/niova-pumicedb/go/pkg/utils/servicediscovery"
)

const (
	defaultLogLevel  = "Info"
	defaultHTTPRetry = 10
	defaultSerfRetry = 5
)

// Config holds the configuration for a new auth client.
type Config struct {
	AppUUID          string
	RaftUUID         string
	GossipConfigPath string
	LogLevel         string
	LogFile          string
	// EncodingFormat is retained for API compatibility; the client now talks to
	// the proxy's REST API (JSON) and no longer uses this field.
	EncodingFormat pmCmn.Format
}

// Client is an authentication service client. It talks to the proxy's REST user
// endpoints (/users/login, /users/admin, /api/users…) rather than the legacy
// /func envelope.
type Client struct {
	appUUID  string
	writeSeq uint64
	sd       *sd.ServiceDiscoveryHandler
	stop     chan int
	once     sync.Once
}

// New creates and initializes a new authentication client.
func New(cfg Config) (*Client, func()) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	if cfg.LogFile == "" {
		cfg.LogFile = cpLib.DefaultLogPath()
	}

	c := &Client{
		appUUID: cfg.AppUUID,
		sd: &sd.ServiceDiscoveryHandler{
			HTTPRetry: defaultHTTPRetry,
			SerfRetry: defaultSerfRetry,
			RaftUUID:  cfg.RaftUUID,
		},
		stop: make(chan int),
	}

	log.InitXlog(cfg.LogFile, &cfg.LogLevel)
	log.Info("Starting auth client API using gossip path: ", cfg.GossipConfigPath)

	go func() {
		if err := c.sd.StartClientAPI(c.stop, cfg.GossipConfigPath); err != nil {
			log.Fatal("Error while starting auth client API: ", err)
		}
	}()

	log.Info("Successfully initialized auth client: ", cfg.AppUUID)
	tearDown := func() {
		c.once.Do(func() {
			close(c.stop)
		})
	}
	return c, tearDown
}

// ---- REST transport ----

// restHeaders builds the common headers. A non-empty token is sent as a Bearer
// token; writes additionally declare a JSON body.
func (c *Client) restHeaders(token string, write bool) map[string]string {
	h := make(map[string]string)
	if write {
		h["Content-Type"] = "application/json"
	}
	if token != "" {
		h["Authorization"] = "Bearer " + token
	}
	return h
}

// restResult turns a transport result into (body, error), converting any non-2xx
// status into an error carrying the proxy's JSON error message.
func restResult(body []byte, status int, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		var er restapi.ErrorResponse
		if json.Unmarshal(body, &er) == nil && er.Error != "" {
			return nil, fmt.Errorf("control plane returned %d: %s", status, er.Error)
		}
		return nil, fmt.Errorf("control plane returned %d: %s", status, string(body))
	}
	return body, nil
}

// nextRncui returns the next client write idempotency key (X-RNCUI), matching the
// legacy sequencing so writes stay dedup-stable per client.
func (c *Client) nextRncui() string {
	seq := atomic.AddUint64(&c.writeSeq, 1) - 1
	return fmt.Sprintf("%s:0:0:0:%d", c.appUUID, seq)
}

// decodeEnvelope unmarshals an APIResponse[T] body and returns the payload. The
// proxy reports failures either as a non-2xx status (handled by restResult before
// this is called) or, on 2xx, as {success:false, error}; this surfaces the latter
// as a Go error too.
func decodeEnvelope[T any](body []byte) (T, error) {
	var zero T
	var resp restapi.APIResponse[T]
	if err := json.Unmarshal(body, &resp); err != nil {
		return zero, err
	}
	if !resp.Success {
		if resp.Error != "" {
			return zero, fmt.Errorf("%s", resp.Error)
		}
		return zero, fmt.Errorf("control plane request failed")
	}
	if resp.Payload == nil {
		return zero, nil
	}
	return *resp.Payload, nil
}

func (c *Client) restGet(token, path string) ([]byte, error) {
	c.sd.TillReady("PROXY", 5)
	body, status, err := c.sd.RESTRequest(http.MethodGet, path, nil, c.restHeaders(token, false))
	return restResult(body, status, err)
}

// restWrite issues a POST/PUT with the required X-RNCUI header.
func (c *Client) restWrite(method, token, path string, jsonBody []byte) ([]byte, error) {
	c.sd.TillReady("PROXY", 5)
	h := c.restHeaders(token, true)
	h["X-RNCUI"] = c.nextRncui()
	body, status, err := c.sd.RESTRequest(method, path, jsonBody, h)
	return restResult(body, status, err)
}

// userRespFromREST maps a REST UserPayload envelope back to the internal
// UserResp. It is only called on a 2xx body, so success is implied.
func userRespFromREST(body []byte) (*userlib.UserResp, error) {
	p, err := decodeEnvelope[restapi.UserPayload](body)
	if err != nil {
		return nil, err
	}
	return &userlib.UserResp{
		UserID:    p.ID,
		Username:  p.Username,
		UserRole:  p.Role,
		Status:    userlib.Status(p.Status),
		SecretKey: p.SecretKey,
		Success:   true,
	}, nil
}

// ---- user operations (REST) ----

// CreateUser creates a new user (admin only). The control plane generates the
// secret key and returns it once in the response.
func (c *Client) CreateUser(token string, user *userlib.UserReq) (*userlib.UserResp, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required for creating users")
	}
	role := userlib.DefaultUserRole
	if user.IsAdmin {
		role = userlib.AdminUserRole
	}
	reqBody, err := json.Marshal(restapi.CreateUserRequest{Username: user.Username, Role: role})
	if err != nil {
		return nil, err
	}
	body, err := c.restWrite(http.MethodPost, token, "/api/users", reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return userRespFromREST(body)
}

// UpdateUser updates an existing user's username and/or secret key (admin).
// UserID (UUID string) is required and the user must exist. Users can only update
// their own record unless they are admin.
func (c *Client) UpdateUser(token string, user *userlib.UserReq) (*userlib.UserResp, error) {
	if user.UserID == "" {
		return nil, fmt.Errorf("userID is required for update operation")
	}
	if token == "" {
		return nil, fmt.Errorf("token is required for updating users")
	}
	reqBody, err := json.Marshal(restapi.UpdateUserRequest{Username: user.Username, NewSecretKey: user.NewSecretKey})
	if err != nil {
		return nil, err
	}
	body, err := c.restWrite(http.MethodPut, token, "/api/users/"+url.PathEscape(user.UserID), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}
	return userRespFromREST(body)
}

// ListUsers returns users. An empty GetReq lists all (admin); a UserID fetches a
// single user (GET /api/users/{id}); a Username filters the collection.
func (c *Client) ListUsers(token string, req userlib.GetReq) ([]userlib.UserResp, error) {
	if req.UserID != "" {
		body, err := c.restGet(token, "/api/users/"+url.PathEscape(req.UserID))
		if err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}
		ur, err := userRespFromREST(body)
		if err != nil {
			return nil, err
		}
		return []userlib.UserResp{*ur}, nil
	}

	path := "/api/users"
	if req.Username != "" {
		path += "?username=" + url.QueryEscape(req.Username)
	}
	body, err := c.restGet(token, path)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	lr, err := decodeEnvelope[restapi.ListUsersPayload](body)
	if err != nil {
		return nil, err
	}
	users := make([]userlib.UserResp, 0, len(lr.Users))
	for _, u := range lr.Users {
		users = append(users, userlib.UserResp{
			UserID:   u.ID,
			Username: u.Username,
			UserRole: u.Role,
			Status:   userlib.Status(u.Status),
			Success:  true,
		})
	}
	return users, nil
}

// GetUser retrieves a user by UserID (UUID string).
func (c *Client) GetUser(userID string, userToken string) (*userlib.UserResp, error) {
	users, err := c.ListUsers(userToken, userlib.GetReq{UserID: userID})
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	return &users[0], nil
}

// GetUserByUsername retrieves a user by username using the secondary index. The
// username filter returns a projection without the secret key, so the full
// record is then fetched by id.
func (c *Client) GetUserByUsername(username string, userToken string) (*userlib.UserResp, error) {
	users, err := c.ListUsers(userToken, userlib.GetReq{Username: username})
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	return c.GetUser(users[0].UserID, userToken)
}

// CreateAdminUser bootstraps the singleton admin user (unauthenticated).
func (c *Client) CreateAdminUser(req *userlib.UserReq) (*userlib.UserResp, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if req.Username == "" {
		return nil, fmt.Errorf("username is required")
	}
	reqBody, err := json.Marshal(restapi.CreateAdminUserRequest{Username: req.Username, SecretKey: req.NewSecretKey})
	if err != nil {
		return nil, err
	}
	body, err := c.restWrite(http.MethodPost, "", "/users/admin", reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin user: %w", err)
	}
	return userRespFromREST(body)
}

// UpdateAdminSecretKey updates the secret key for an admin user.
func (c *Client) UpdateAdminSecretKey(userID, newSecretKey, userToken string) (*userlib.UserResp, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if newSecretKey == "" {
		return nil, fmt.Errorf("newSecretKey is required")
	}
	reqBody, err := json.Marshal(restapi.UpdateUserRequest{NewSecretKey: newSecretKey})
	if err != nil {
		return nil, err
	}
	body, err := c.restWrite(http.MethodPut, userToken, "/api/users/"+url.PathEscape(userID), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update admin secret key: %w", err)
	}
	return userRespFromREST(body)
}

// Login authenticates with username and secret key, returning a JWT.
func (c *Client) Login(username, secretKey string) (*userlib.LoginResp, error) {
	reqBody, err := json.Marshal(restapi.LoginRequest{Username: username, Password: secretKey})
	if err != nil {
		return nil, err
	}
	c.sd.TillReady("PROXY", 5)
	// Login is unauthenticated and carries no X-RNCUI.
	body, status, err := c.sd.RESTRequest(http.MethodPost, "/users/login", reqBody,
		map[string]string{"Content-Type": "application/json"})
	body, err = restResult(body, status, err)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}
	lr, err := decodeEnvelope[restapi.LoginPayload](body)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}
	return &userlib.LoginResp{
		AccessToken: lr.AccessToken,
		TokenType:   lr.TokenType,
		ExpiresIn:   lr.ExpiresIn,
		UserID:      lr.UserID,
		Username:    lr.Username,
		UserRole:    lr.Role,
		IsAdmin:     lr.IsAdmin,
		Success:     true,
	}, nil
}

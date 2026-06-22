package client

import (
	"fmt"
	"sync"
	"sync/atomic"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	pmCmn "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	sd "github.com/00pauln00/niova-pumicedb/go/pkg/utils/servicediscovery"
)

const (
	defaultLogLevel    = "Info"
	defaultHTTPRetry   = 10
	defaultSerfRetry   = 5
	defaultEncodingFmt = pmCmn.JSON
)

// Config holds the configuration for new auth client
type Config struct {
	AppUUID          string
	RaftUUID         string
	GossipConfigPath string
	LogLevel         string
	LogFile          string
	EncodingFormat   pmCmn.Format
}

// Client represents an authentication service client
type Client struct {
	appUUID  string
	writeSeq uint64
	sd       *sd.ServiceDiscoveryHandler
	encType  pmCmn.Format
	stop     chan int
	once     sync.Once
}

// New creates and initializes a new authentication client
func New(cfg Config) (*Client, func()) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	if cfg.LogFile == "" {
		cfg.LogFile = cpLib.DefaultLogPath()
	}
	if cfg.EncodingFormat == "" {
		cfg.EncodingFormat = defaultEncodingFmt
	}

	c := &Client{
		appUUID: cfg.AppUUID,
		sd: &sd.ServiceDiscoveryHandler{
			HTTPRetry: defaultHTTPRetry,
			SerfRetry: defaultSerfRetry,
			RaftUUID:  cfg.RaftUUID,
		},
		encType: cfg.EncodingFormat,
		stop:    make(chan int),
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

// request sends a request to auth server
func (c *Client) request(data []byte, url string, isWrite bool) ([]byte, error) {
	c.sd.TillReady("PROXY", 5)
	resp, err := c.sd.Request(data, "/func?"+url, isWrite)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to auth server: %w", err)
	}
	return resp, nil
}

// executePut sends write request with proper sequencing
func (c *Client) executePut(url string, data []byte) ([]byte, error) {
	seq := atomic.LoadUint64(&c.writeSeq)
	atomic.AddUint64(&c.writeSeq, 1)

	rncui := fmt.Sprintf("%s:0:0:0:%d", c.appUUID, seq)
	url += "&rncui=" + rncui
	return c.request(data, url, true)
}

type requestFunc func(url string, data []byte) ([]byte, error)

// doRequest performs the common logic for get/put operations
func (c *Client) doRequest(token string, data, resp interface{}, operation string, reqFunc requestFunc) error {
	url := "name=" + operation

	// Wrap in CPReq
	cpReq := cpLib.CPReq{
		Token:   token,
		Payload: data,
	}

	encoded, err := pmCmn.Encoder(c.encType, cpReq)
	if err != nil {
		return fmt.Errorf("failed to encode request data: %w", err)
	}

	respData, err := reqFunc(url, encoded)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if respData == nil {
		return fmt.Errorf("received empty response from server")
	}

	// Prepare CPResp for single-pass decoding
	cpResp := cpLib.CPResp{
		Payload: resp,
	}

	if err := pmCmn.Decoder(c.encType, respData, &cpResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if err := cpResp.Err(); err != nil {
		code := ""
		if cpResp.Error != nil {
			code = string(cpResp.Error.Code)
		}
		return fmt.Errorf("server error: %s (code: %s)", err, code)
	}

	return nil
}

func (c *Client) put(token string, data, resp interface{}, operation string) error {
	return c.doRequest(token, data, resp, operation, c.executePut)
}

func (c *Client) get(token string, data, resp interface{}, operation string) error {
	return c.doRequest(token, data, resp, operation, func(url string, data []byte) ([]byte, error) {
		return c.request(data, url, false)
	})
}
func (c *Client) CreateUser(token string, user *userlib.UserReq) (*userlib.UserResp, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required for creating users")
	}
	resp := &userlib.UserResp{}
	if err := c.put(token, user, resp, userlib.PutUserAPI); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return resp, nil
}

// UpdateUser updates an existing user's username and/or capabilities.
// UserID (UUID string) is required and user must exist.
// Users can only update their own record unless they are admin.
func (c *Client) UpdateUser(token string, user *userlib.UserReq) (*userlib.UserResp, error) {
	if user.UserID == "" {
		return nil, fmt.Errorf("userID is required for update operation")
	}
	if token == "" {
		return nil, fmt.Errorf("token is required for updating users")
	}

	user.IsUpdate = true
	resp := &userlib.UserResp{}
	if err := c.put(token, user, resp, userlib.PutUserAPI); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}
	return resp, nil
}

func (c *Client) ListUsers(token string, req userlib.GetReq) ([]userlib.UserResp, error) {
	var users []userlib.UserResp
	if err := c.get(token, req, &users, userlib.GetUserAPI); err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	return users, nil
}

func (c *Client) GetUser(userID string, userToken string) (*userlib.UserResp, error) {
	req := userlib.GetReq{
		UserID: userID,
	}
	users, err := c.ListUsers(userToken, req)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	return &users[0], nil
}

// GetUserByUsername retrieves a user by username using the secondary index.
func (c *Client) GetUserByUsername(username string, userToken string) (*userlib.UserResp, error) {
	req := userlib.GetReq{
		Username: username,
	}
	users, err := c.ListUsers(userToken, req)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	return &users[0], nil
}

// CreateAdminUser creates an admin user.
func (c *Client) CreateAdminUser(req *userlib.UserReq) (*userlib.UserResp, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if req.Username == "" {
		return nil, fmt.Errorf("username is required")
	}

	resp := &userlib.UserResp{}
	if err := c.put("", req, resp, userlib.AdminUserAPI); err != nil {
		return nil, fmt.Errorf("failed to create admin user: %w", err)
	}
	return resp, nil
}

// UpdateAdminSecretKey updates the secret key for an admin user.
func (c *Client) UpdateAdminSecretKey(userID, newSecretKey, userToken string) (*userlib.UserResp, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if newSecretKey == "" {
		return nil, fmt.Errorf("newSecretKey is required")
	}

	req := &userlib.UserReq{
		UserID:       userID,
		IsUpdate:     true,
		NewSecretKey: newSecretKey,
	}

	resp := &userlib.UserResp{}
	if err := c.put(userToken, req, resp, userlib.PutUserAPI); err != nil {
		return nil, fmt.Errorf("failed to update admin secret key: %w", err)
	}
	return resp, nil
}

// Login authenticates the user with username and secret key, returns JWT token.
func (c *Client) Login(username, secretKey string) (*userlib.LoginResp, error) {
	req := &cpLib.GetReqEnvelope{
		Req: cpLib.GetReq{
			ID: fmt.Sprintf("%s:%s", username, secretKey),
		},
	}

	resp := &userlib.LoginResp{}
	if err := c.get("", req, resp, userlib.LoginAPI); err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("login failed: %s", resp.Error)
	}
	return resp, nil
}

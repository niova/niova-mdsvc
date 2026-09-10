package mdsvcpumiceclient

import (
	"fmt"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	usercl "github.com/00pauln00/niova-mdsvc/controlplane/user/client"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
)

var _ domain.UserServiceClient = (*userClient)(nil)

// userClient adapts *usercl.Client to domain.UserServiceClient.
type userClient struct {
	uc *usercl.Client
}

// NewUserClient wraps an existing raft/gossip user client so it satisfies
// domain.UserServiceClient.
func NewUserClient(uc *usercl.Client) domain.UserServiceClient {
	if uc == nil {
		return nil
	}
	return &userClient{uc: uc}
}

func (l *userClient) Login(username, secretKey string) (*userlib.LoginResp, error) {
	return l.uc.Login(username, secretKey)
}

// LoginWithTenant has no tenant-scoped login to delegate to — a non-empty
// tenantUUID is rejected rather than silently logging into the default
// tenant instead.
func (l *userClient) LoginWithTenant(username, secretKey, tenantUUID string) (*userlib.LoginResp, error) {
	if tenantUUID != "" {
		return nil, fmt.Errorf("tenant-scoped login isn't supported against niova-mdsvc")
	}
	return l.uc.Login(username, secretKey)
}

func (l *userClient) ListUsers(token string, req userlib.GetReq) ([]userlib.UserResp, error) {
	return l.uc.ListUsers(token, req)
}

func (l *userClient) CreateUser(token string, req *userlib.UserReq) (*userlib.UserResp, error) {
	return l.uc.CreateUser(token, req)
}

func (l *userClient) CreateAdminUser(req *userlib.UserReq) (*userlib.UserResp, error) {
	return l.uc.CreateAdminUser(req)
}

// InitUserClient initializes the user management client.
func InitUserClient(raftUUID, gossipPath, logFile string) (*usercl.Client, func()) {
	if raftUUID == "" || gossipPath == "" {
		log.Warn("Cannot initialize user client: missing raft UUID or gossip path")
		return nil, func() {}
	}

	appUUID := uuid.New().String()
	cfg := usercl.Config{
		AppUUID:          appUUID,
		RaftUUID:         raftUUID,
		GossipConfigPath: gossipPath,
		LogLevel:         "Info",
		LogFile:          logFile,
	}

	client, teardown := usercl.New(cfg)
	log.Info("User client initialized with UUID: ", appUUID)
	return client, teardown
}

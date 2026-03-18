package clictlplanefuncs

import (
	"testing"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
	log "github.com/sirupsen/logrus"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateHierarchyforUserAuthentication(t *testing.T) {
	c := newClient(t)
	adminToken := getAdminToken(t)

	pdus := []string{
		"9bc244bc-df29-11f0-a93b-277aec17e401",
	}

	// 2 RACKS
	racks := []string{
		"3f082930-df29-11f0-ab7b-4bd430991101",
		"3f082930-df29-11f0-ab7b-4bd430991102",
	}

	// 4 HVs
	hvs := []string{
		"bde1f08a-df63-11f0-88ef-430ddec19901",
		"bde1f08a-df63-11f0-88ef-430ddec19902",
		"bde1f08a-df63-11f0-88ef-430ddec19903",
		"bde1f08a-df63-11f0-88ef-430ddec19904",
	}

	// 4 Devices
	// devices := []string{
	// 	"nvme-fb6358163001",
	// 	"nvme-fb6358163002",
	// 	"nvme-fb6358163003",
	// 	"nvme-fb6358163004",
	// 	"nvme-fb6358163005",
	// }

	mockNisd := []cpLib.Nisd{
		cpLib.Nisd{
			PeerPort: 13000,
			ID:       "86adee3a-d5da-11f0-8250-5f1ad86a5661",
			FailureDomain: []string{
				pdus[0],
				racks[0],
				hvs[0],
				"/dev/loop25",
				"/dev/loop25",
			},
			TotalSize:     24 * 1024 * 1024 * 1024,
			AvailableSize: 24 * 1024 * 1024 * 1024,
			UserToken:     adminToken,
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "172.31.24.182",
					Port:   13001,
				},
			},
			NetInfoCnt: 1,
		},
	}

	for _, n := range mockNisd {
		resp, err := c.PutNisd(&n)
		if assert.NoError(t, err) {
			assert.True(t, resp.Success)
		}
		log.Info("response : ", resp, err)
	}

	req := cpLib.GetReq{
		GetAll:    true,
		UserToken: adminToken,
	}
	res, err := c.GetNisds(req)
	for _, n := range res {
		log.Infof("Nisd ID: %s, usage: %d", n.ID, usagePercent(n))
	}
	log.Info("total number of nisd's : ", len(res))
	assert.NoError(t, err)
}

func TestUserVdevCreation(t *testing.T) {
	// Initialize control plane client for vdev operations
	ctlClient := newClient(t)

	// Initialize user client for authentication operations
	authClient, tearDown := newUserClient(t)
	defer tearDown()

	// Step 0: Get admin token using shared helper (ensure admin exists/reset)
	adminToken := getAdminToken(t)
	t.Logf("Admin logged in/setup complete")

	// Step 1: Create normal user1
	user1Username := "vdev_owner_" + uuid.New().String()[:8]
	user1Req := &userlib.UserReq{
		Username:  user1Username,
		UserToken: adminToken,
	}

	user1Resp, err := authClient.CreateUser(user1Req)
	assert.NoError(t, err, "failed to create user1")
	assert.NotNil(t, user1Resp)
	assert.True(t, user1Resp.Success)
	assert.NotEmpty(t, user1Resp.SecretKey)
	assert.NotEmpty(t, user1Resp.UserID)
	assert.Equal(t, userlib.DefaultUserRole, user1Resp.UserRole)
	log.Infof("Created user1: %s with ID: %s", user1Username, user1Resp.UserID)
	log.Infof("Secret key of user1: %s", user1Resp.SecretKey)

	// Step 2: Login with user1 to get access token
	user1LoginResp, err := authClient.Login(user1Username, user1Resp.SecretKey)
	assert.NoError(t, err, "user1 login should succeed")
	assert.True(t, user1LoginResp.Success)
	assert.NotEmpty(t, user1LoginResp.AccessToken, "user1 access token should not be empty")
	user1AccessToken := user1LoginResp.AccessToken
	t.Logf("User1 logged in, access token obtained")

	// Step 3: User1 creates a vdev with their access token
	vdev1 := &cpLib.VdevReq{
		Vdev: &cpLib.VdevCfg{
			Size:       16 * 1024 * 1024 * 1024, // 16 GB
			NumReplica: 1,
		},
		UserToken: user1AccessToken,
	}

	vdevResp, err := ctlClient.CreateVdev(vdev1)
	assert.NoError(t, err, "user1 should be able to create vdev")
	require.NotNil(t, vdevResp, "user1 vdev response should not be nil")
	assert.True(t, vdevResp.Success, "vdev creation should succeed")
	assert.NotEmpty(t, vdevResp.ID, "vdev ID should not be empty")
	vdevID := vdevResp.ID
	t.Logf("User1 created vdev with ID: %s", vdevID)

	// Step 4: Verify user1 can access their own vdev
	getReqUser1 := &cpLib.GetReq{
		ID:        vdevID,
		UserToken: user1AccessToken,
	}

	vdevCfg, err := ctlClient.GetVdevCfg(getReqUser1)
	assert.NoError(t, err, "user1 should be able to read their own vdev")
	assert.Equal(t, vdevID, vdevCfg.ID, "fetched vdev ID should match")
	log.Infof("User1 successfully accessed their vdev: %s", vdevCfg.ID)
}
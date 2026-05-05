package clictlplanefuncs

import (
	"testing"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

func TestCreateHierarchyForUserAuthentication(t *testing.T) {
	c := newClient(t)
	adminToken := getAdminToken(t)

	pdus := []string{
		"9bc244bc-df29-11f0-a93b-277aec17e401",
	}

	racks := []string{
		"3f082930-df29-11f0-ab7b-4bd430991101",
	}

	hvs := []string{
		"bde1f08a-df63-11f0-88ef-430ddec19901",
	}

	c.SetToken(adminToken)

	mockNisd := cpLib.Nisd{
		PeerPort: 13000,
		ID:       "7467890a-2299-11f1-9fc3-ebb3b3f1fb90",
		FailureDomain: []string{
			pdus[0],
			racks[0],
			hvs[0],
			"/s3DV_nisd.device",
			"/s3DV_nisd.device",
		},
		TotalSize:     10 * 1024 * 1024 * 1024,
		AvailableSize: 10 * 1024 * 1024 * 1024,
		NetInfo: cpLib.NetInfoList{
			cpLib.NetworkInfo{
				IPAddr: "127.0.0.1",
				Port:   13001,
			},
		},
		NetInfoCnt: 1,
	}

	resp, err := c.PutNisd(&mockNisd)
	if assert.NoError(t, err) {
		assert.True(t, resp.Success)
	}
	log.Info("response : ", resp, err)

	req := cpLib.GetReq{
		GetAll: true,
	}
	res, err := c.GetNisds(req)

	log.Infof("Nisd ID: %s", res[0].ID)

	assert.NoError(t, err)
}

func TestUserAuthVdevCreation(t *testing.T) {
	// Initialize control plane client for vdev operations
	ctlClient := newClient(t)

	// Initialize user client for authentication operations
	authClient, tearDown := newUserClient(t)
	defer tearDown()

	// Step 0: Get admin token using shared helper (ensure admin exists/reset)
	adminToken := getAdminToken(t)
	log.Info("Admin logged in/setup complete")

	ctlClient.SetToken(adminToken)

	// Step 1: Create normal user
	userUsername := "vdev_owner_" + uuid.New().String()[:8]
	user := &userlib.UserReq{
		Username: userUsername,
	}

	userResp, err := authClient.CreateUser(adminToken, user)
	assert.NoError(t, err, "failed to create user")
	assert.NotNil(t, userResp)
	assert.True(t, userResp.Success)
	assert.NotEmpty(t, userResp.SecretKey)
	assert.NotEmpty(t, userResp.UserID)
	assert.Equal(t, userlib.DefaultUserRole, userResp.UserRole)
	log.Infof("Created user: %s with ID: %s", userUsername, userResp.UserID)
	log.Infof("Secret key of user: %s", userResp.SecretKey)

	// Step 2: Login with user to get access token
	userLoginResp, err := authClient.Login(userUsername, userResp.SecretKey)
	assert.NoError(t, err, "user login should succeed")
	assert.True(t, userLoginResp.Success)
	assert.NotEmpty(t, userLoginResp.AccessToken, "user access token should not be empty")
	user1AccessToken := userLoginResp.AccessToken
	log.Info("User logged in, access token obtained")
	ctlClient.SetToken(user1AccessToken)

	// Step 3: Admin creates a vdev with their access token
	vdev1 := &cpLib.VdevReq{
		Vdev: &cpLib.VdevCfg{
			Size:       8 * 1024 * 1024 * 1024, // 8 GB
			NumReplica: 1,
		},
	}

	vdevResp, err := ctlClient.CreateVdev(vdev1)
	assert.NoError(t, err, "user should be able to create vdev")
	require.NotNil(t, vdevResp, "user vdev response should not be nil")
	assert.True(t, vdevResp.Success, "vdev creation should succeed")
	assert.NotEmpty(t, vdevResp.ID, "vdev ID should not be empty")
	vdevID := vdevResp.ID
	log.Infof("user created vdev with ID: %s", vdevID)

	// Step 4: Verify user can access their own vdev
	getReqUser := &cpLib.GetReq{
		ID: vdevID,
	}

	vdevCfg, err := ctlClient.GetVdevCfg(getReqUser)
	assert.NoError(t, err, "user should be able to read their own vdev")
	assert.Equal(t, vdevID, vdevCfg.ID, "fetched vdev ID should match")
	log.Infof("User successfully accessed their vdev: %s", vdevCfg.ID)

	// Step 5: Create normal user2
	user2Username := "unauthorized_user_" + uuid.New().String()[:8]
	user2Req := &userlib.UserReq{
		Username: user2Username,
	}

	user2Resp, err := authClient.CreateUser(adminToken, user2Req)
	assert.NoError(t, err, "failed to create user2")
	assert.NotNil(t, user2Resp)
	assert.True(t, user2Resp.Success)
	assert.NotEmpty(t, user2Resp.SecretKey)
	assert.NotEmpty(t, user2Resp.UserID)
	log.Infof("Created user2: %s with ID: %s", user2Username, user2Resp.UserID)
	log.Infof("Secret key of user2: %s", userResp.SecretKey)
}

func TestCreateHierarchyForMultipleBlockTest(t *testing.T) {
	c := newClient(t)
	adminToken := getAdminToken(t)

	pdus := []string{
		"9bc244bc-df29-11f0-a93b-277aec17e402",
	}

	racks := []string{
		"3f082930-df29-11f0-ab7b-4bd430991102",
	}

	hvs := []string{
		"bde1f08a-df63-11f0-88ef-430ddec19902",
	}

	c.SetToken(adminToken)

	// NOTE: For testing in CI, only 1 NISD is created
	mockNisd := cpLib.Nisd{
		PeerPort: 13000,
		ID:       "7467890a-2299-11f1-9fc3-ebb3b3f1fb91",
		FailureDomain: []string{
			pdus[0],
			racks[0],
			hvs[0],
			"/s3DV_nisd.device",
			"/s3DV_nisd.device",
		},
		TotalSize:     24 * 1024 * 1024 * 1024,
		AvailableSize: 24 * 1024 * 1024 * 1024,
		NetInfo: cpLib.NetInfoList{
			cpLib.NetworkInfo{
				IPAddr: "127.0.0.1",
				Port:   13001,
			},
		},
		NetInfoCnt: 1,
	}

	resp, err := c.PutNisd(&mockNisd)
	if assert.NoError(t, err) {
		assert.True(t, resp.Success)
	}
	log.Info("response : ", resp, err)

	req := cpLib.GetReq{
		GetAll: true,
	}
	res, err := c.GetNisds(req)

	log.Infof("Nisd ID: %s", res[0].ID)

	assert.NoError(t, err)
}

func TestUserVdevCreationForMultipleBlockTest(t *testing.T) {
	// Initialize control plane client for vdev operations
	c := newClient(t)

	// Get admin token using shared helper (ensure admin exists/reset)
	adminToken := getAdminToken(t)
	log.Info("Admin logged in/setup complete")

	// NOTE: For testing in CI, only two vdevs are created
	numVdevs := 2
	vdevIDs := make([]string, 0, numVdevs)

	c.SetToken(adminToken)

	for i := 0; i < numVdevs; i++ {
		vdevReq := &cpLib.VdevReq{
			Vdev: &cpLib.VdevCfg{
				Size:       8 * 1024 * 1024 * 1024, // 8 GB
				NumReplica: 1,
			},
		}

		vdevResp, err := c.CreateVdev(vdevReq)

		assert.NoError(t, err, "vdev creation should not fail")
		require.NotNil(t, vdevResp, "vdev response should not be nil")
		assert.True(t, vdevResp.Success, "vdev creation should succeed")
		assert.NotEmpty(t, vdevResp.ID, "vdev ID should not be empty")

		vdevIDs = append(vdevIDs, vdevResp.ID)

		log.Infof("Created vdev %d with ID: %s", i, vdevResp.ID)
	}

	// validation
	assert.Equal(t, numVdevs, len(vdevIDs), "should create all vdevs")

	getReq := &cpLib.GetReq{}

	vdevList, err := c.GetVdevCfgs(getReq)

	assert.NoError(t, err, "should fetch all vdevs")
	require.NotNil(t, vdevList, "vdev list should not be nil")

	// vdevList now contains all vdevs
	log.Infof("Total vdevs fetched: %d", len(vdevList))

	for i, v := range vdevList {
		log.Infof("Index: %d, Vdev ID: %s", i, v.ID)
	}
}

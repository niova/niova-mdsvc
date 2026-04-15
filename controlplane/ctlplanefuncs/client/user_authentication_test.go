package clictlplanefuncs

import (
   "testing"
   "fmt"

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
   //  "nvme-fb6358163001",
   //  "nvme-fb6358163002",
   //  "nvme-fb6358163003",
   //  "nvme-fb6358163004",
   //  "nvme-fb6358163005",
   // }

   c.SetToken(adminToken)

   mockNisd := []cpLib.Nisd{
       cpLib.Nisd{
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
                //    IPAddr: "172.31.24.182",
                   IPAddr: "127.0.0.1",
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
   }
   res, err := c.GetNisds(req)
   for _, n := range res {
       log.Infof("Nisd ID: %s, usage: %d", n.ID, usagePercent(n))
   }
   log.Info("total number of nisd's : ", len(res))
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
       ID:        vdevID,
   }

   vdevCfg, err := ctlClient.GetVdevCfg(getReqUser)
   assert.NoError(t, err, "user should be able to read their own vdev")
   assert.Equal(t, vdevID, vdevCfg.ID, "fetched vdev ID should match")
   log.Infof("User successfully accessed their vdev: %s", vdevCfg.ID)

   // Step 5: Create normal user2
   user2Username := "unauthorized_user_" + uuid.New().String()[:8]
   user2Req := &userlib.UserReq{
       Username:  user2Username,
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

func TestCreateHierarchyforMultipleBlockTest(t *testing.T) {
   c := newClient(t)
   adminToken := getAdminToken(t)

   pdus := []string{
       "f0991962-2771-11f1-984f-ffb9728f3481",
       "f0991962-2771-11f1-984f-ffb9728f3482",
   }

   // 4 RACKS
   racks := []string{
       "0a2de204-2772-11f1-a514-1f1acb943981",
       "0a2de204-2772-11f1-a514-1f1acb943982",
       "0a2de204-2772-11f1-a514-1f1acb943983",
       "0a2de204-2772-11f1-a514-1f1acb943984",
   }

   // 8 HVs
   hvs := []string{
       "2fef1454-2772-11f1-997f-236331f79711",
       "2fef1454-2772-11f1-997f-236331f79712",
       "2fef1454-2772-11f1-997f-236331f79713",
       "2fef1454-2772-11f1-997f-236331f79714",
       "2fef1454-2772-11f1-997f-236331f79715",
       "2fef1454-2772-11f1-997f-236331f79716",
       "2fef1454-2772-11f1-997f-236331f79717",
       "2fef1454-2772-11f1-997f-236331f79718",
   }

   // 2 NISDs
   nisds := []string{
       "83b1a782-2772-11f1-91ea-5f00b1c98291",
       "83b1a782-2772-11f1-91ea-5f00b1c98292",
       "83b1a782-2772-11f1-91ea-5f00b1c98293",
       "83b1a782-2772-11f1-91ea-5f00b1c98294",
       "83b1a782-2772-11f1-91ea-5f00b1c98295",
       "83b1a782-2772-11f1-91ea-5f00b1c98296",
       "83b1a782-2772-11f1-91ea-5f00b1c98297",
       "83b1a782-2772-11f1-91ea-5f00b1c98298",
       "83b1a782-2772-11f1-91ea-5f00b1c98299",
       "83b1a782-2772-11f1-91ea-5f00b1c98300",
       "83b1a782-2772-11f1-91ea-5f00b1c98301",
       "83b1a782-2772-11f1-91ea-5f00b1c98302",
       "83b1a782-2772-11f1-91ea-5f00b1c98303",
       "83b1a782-2772-11f1-91ea-5f00b1c98304",
       "83b1a782-2772-11f1-91ea-5f00b1c98305",
       "83b1a782-2772-11f1-91ea-5f00b1c98306",
   }

   mockNisd := []cpLib.Nisd{}

   nisdIndex := 0
   basePort := uint16(16000)

   for p := 0; p < len(pdus); p++ {
       for r := 0; r < 2; r++ {
           rackIndex := p*2 + r

           for h := 0; h < 2; h++ {
               hvIndex := rackIndex*2 + h

               for n := 0; n < 2; n++ {

                   peerPort := basePort + uint16(nisdIndex*10)
                   clientPort := peerPort + 1

                   nisd := cpLib.Nisd{
                       PeerPort: peerPort,
                       ID:       nisds[nisdIndex],
                       FailureDomain: []string{
                           pdus[p],
                           racks[rackIndex],
                           hvs[hvIndex],
                           fmt.Sprintf("/auth_nisd_%d.device", nisdIndex),
                           fmt.Sprintf("/auth_nisd_%d.device", nisdIndex),
                       },
                       TotalSize:     16 * 1024 * 1024 * 1024,
                       AvailableSize: 16 * 1024 * 1024 * 1024,
                       NetInfo: cpLib.NetInfoList{
                           cpLib.NetworkInfo{
                               IPAddr: "172.31.24.182",
                               Port:   clientPort,
                           },
                       },
                       NetInfoCnt: 1,
                   }

                   mockNisd = append(mockNisd, nisd)
                   nisdIndex++
               }
           }
       }
   }

   c.SetToken(adminToken)

   Nisd := []cpLib.Nisd{
       cpLib.Nisd{
           PeerPort: 13000,
           ID:       nisds[0],
           FailureDomain: []string{
               pdus[0],
               racks[0],
               hvs[0],
               "/auth_nisd_0.device",
               "/auth_nisd_0.device",
           },
           TotalSize:     24 * 1024 * 1024 * 1024,
           AvailableSize: 24 * 1024 * 1024 * 1024,
           NetInfo: cpLib.NetInfoList{
               cpLib.NetworkInfo{
                   IPAddr: "172.31.24.182",
                   Port:   13001,
               },
           },
           NetInfoCnt: 1,
       },
       cpLib.Nisd{
           PeerPort: 13002,
           ID:       nisds[1],
           FailureDomain: []string{
               pdus[0],
               racks[0],
               hvs[0],
               "/auth_nisd_1.device",
               "/auth_nisd_1.device",
           },
           TotalSize:     24 * 1024 * 1024 * 1024,
           AvailableSize: 24 * 1024 * 1024 * 1024,
           NetInfo: cpLib.NetInfoList{
               cpLib.NetworkInfo{
                   IPAddr: "172.31.24.182",
                   Port:   13003,
               },
           },
           NetInfoCnt: 1,
       },
   }

   for _, n := range Nisd {
       resp, err := c.PutNisd(&n)
       if assert.NoError(t, err) {
           assert.True(t, resp.Success)
       }
       log.Info("response : ", resp, err)
   }

   req := cpLib.GetReq{
       GetAll:    true,
   }

   nisdList, err := c.GetNisds(req)

   assert.NoError(t, err)
   require.NotNil(t, nisdList)

   // nisdList is your stored list
   log.Infof("Total NISDs: %d", len(nisdList))

   nisdIDs := make([]string, 0, len(nisdList))

   for _, n := range nisdList {
       nisdIDs = append(nisdIDs, n.ID)
   }

   for i, n := range nisdList {
       log.Infof("Index: %d, Nisd ID: %s", i, n.ID)
   }
}

func TestUserVdevCreationForMultipleBlockTest(t *testing.T) {
   // Initialize control plane client for vdev operations
   c := newClient(t)

   // Get admin token using shared helper (ensure admin exists/reset)
   adminToken := getAdminToken(t)
   log.Info("Admin logged in/setup complete")

   numVdevs := 4
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
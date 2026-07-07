package srvctlplanefuncs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	"github.com/google/uuid"

	auth "github.com/00pauln00/niova-mdsvc/controlplane/auth/jwt"
	authz "github.com/00pauln00/niova-mdsvc/controlplane/authorizer"
	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

	pmCmn "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	funclib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/common"
	PumiceDBServer "github.com/00pauln00/niova-pumicedb/go/pkg/pumiceserver"
	storageiface "github.com/00pauln00/niova-pumicedb/go/pkg/utils/storage/interface"
	"github.com/00pauln00/niova-pumicedb/go/pkg/utils/storage/memstore"
)

const (
	testPDU  = "acdef556-1ea3-11f1-848b-9f6e716afc46"
	testRack = "b1b89a50-1ea3-11f1-b397-d76191bdb3d2"
	testHV   = "b726b99a-1ea3-11f1-95da-436ff27bf77e"
	testDev  = "nvme-001"
	testPT   = "nvme-001-01"
	LOG_FILE = "/tmp/test.log"
)

// TestMain initializes the test environment
func TestMain(m *testing.M) {
	// Initialize xlog to prevent nil pointer errors
	logLevel := "INFO"
	log.InitXlog(LOG_FILE, &logLevel)

	ctlplfl.RegisterGOBStructs()

	// Run tests
	code := m.Run()

	os.Exit(code)
}

// Test constants
const (
	testUserID1 = "user-123"
	testUserID2 = "user-456"
	testAdminID = "admin-001"
	testVdevID  = "vdev-test-001"
)

var (
	testSecret = []byte(ctlplfl.CP_SECRET)
)

// Helper function to create valid JWT token
func createTestToken(userID, role string, secret []byte) (string, error) {
	tc := &auth.Token{
		Secret: secret,
		TTL:    time.Hour, // Valid for 1 hour
	}
	claims := map[string]any{
		"userID": userID,
		"role":   role,
	}
	return tc.CreateToken(claims)
}

/*
// Helper function to create expired token
func createExpiredToken(userID, role string, secret []byte) (string, error) {
	tc := &auth.Token{
		Secret: secret,
		TTL:    -time.Hour, // Already expired
	}
	claims := map[string]any{
		"userID": userID,
		"role":   role,
	}
	return tc.CreateToken(claims)
}
*/

/*
// Helper function to verify ownership key exists in datastore
func verifyOwnershipKey(ds storageiface.DataStore, userID, vdevID string) bool {
	ownershipKey := fmt.Sprintf("/u/%s/v/%s", userID, vdevID)
	result, err := ds.Read(ownershipKey, "")
	if err != nil {
		return false
	}
	return string(result) == "1"
}
*/

// Helper function to setup vdev data in memstore
func setupVdevData(ds storageiface.DataStore, vdevID string) error {
	vdevKey := fmt.Sprintf("v/%s", vdevID)

	// Write vdev configuration data
	err := ds.Write(fmt.Sprintf("%s/cfg/size", vdevKey), "1073741824", "")
	if err != nil {
		return err
	}
	err = ds.Write(fmt.Sprintf("%s/cfg/num_chunks", vdevKey), "4", "")
	if err != nil {
		return err
	}
	err = ds.Write(fmt.Sprintf("%s/cfg/num_replicas", vdevKey), "3", "")
	if err != nil {
		return err
	}
	return nil
}

func TestWPCreateVdev(t *testing.T) {
	// Initialize test authorizer
	authorizer = authz.NewAuthorizerWithConfig(authz.Config{
		authz.WPCreateVdev: authz.FunctionPolicy{
			RBAC: []string{"admin", "user"},
		},
	})
	defer func() { authorizer = nil }()

	testCases := []struct {
		name            string
		setupToken      func() string
		vdevSize        int64
		numChunks       int
		numReplica      int
		expectError     bool
		errorContains   string
		expectedErrCode ctlplfl.CPErrCode
		checkOwnership  bool
		expectedUserID  string
	}{
		{
			name: "SuccessfulCreation_UserRole",
			setupToken: func() string {
				token, _ := createTestToken(testUserID1, "user", testSecret)
				return token
			},
			vdevSize:       1073741824,
			numChunks:      4,
			numReplica:     3,
			expectError:    false,
			checkOwnership: true,
			expectedUserID: testUserID1,
		},
		{
			name: "SuccessfulCreation_AdminRole",
			setupToken: func() string {
				token, _ := createTestToken(testAdminID, "admin", testSecret)
				return token
			},
			vdevSize:       1073741824,
			numChunks:      4,
			numReplica:     3,
			expectError:    false,
			checkOwnership: true,
			expectedUserID: testAdminID,
		},
		{
			name: "MissingToken",
			setupToken: func() string {
				return ""
			},
			vdevSize:        1073741824,
			numChunks:       4,
			numReplica:      3,
			expectError:     true,
			errorContains:   "user token is required",
			expectedErrCode: ctlplfl.ErrAuth,
		},
		{
			name: "MissingUserIDClaim",
			setupToken: func() string {
				tc := &auth.Token{Secret: testSecret, TTL: time.Hour}
				claims := map[string]any{"role": "user"} // Missing userID
				token, _ := tc.CreateToken(claims)
				return token
			},
			vdevSize:        1073741824,
			numChunks:       4,
			numReplica:      3,
			expectError:     true,
			errorContains:   "missing userID",
			expectedErrCode: ctlplfl.ErrAuth,
		},
		{
			name: "MissingRoleClaim",
			setupToken: func() string {
				tc := &auth.Token{Secret: testSecret, TTL: time.Hour}
				claims := map[string]any{"userID": testUserID1} // Missing role
				token, _ := tc.CreateToken(claims)
				return token
			},
			vdevSize:        1073741824,
			numChunks:       4,
			numReplica:      3,
			expectError:     true,
			errorContains:   "missing role",
			expectedErrCode: ctlplfl.ErrAuth,
		},
		{
			name: "UnauthorizedRole",
			setupToken: func() string {
				token, _ := createTestToken(testUserID1, "viewer", testSecret)
				return token
			},
			vdevSize:        1073741824,
			numChunks:       4,
			numReplica:      3,
			expectError:     true,
			errorContains:   "authorization failed",
			expectedErrCode: ctlplfl.ErrAuth,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test vdev
			cpReq := ctlplfl.CPReq{
				Token: tc.setupToken(),
				Payload: ctlplfl.VdevReq{
					Vdev: &ctlplfl.VdevConfig{
						Name:       "wpvdev" + uuid.NewString()[:8],
						Size:       tc.vdevSize,
						ChunkCnt:   uint32(tc.numChunks),
						DataBlkCnt: uint8(tc.numReplica),
					},
				},
			}

			// Call WPCreateVdev
			result, err := WPCreateVdev(cpReq)

			// Check error expectations
			if tc.expectError {
				if result == nil {
					t.Error("Expected non-nil result for encoded error response")
					return
				}
				// WP functions return FuncIntrm; decode it first, then extract CPResp.
				var intrm funclib.FuncIntrm
				if decErr := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &intrm); decErr != nil {
					t.Fatalf("Failed to decode FuncIntrm: %v", decErr)
				}
				cpResp, ok := intrm.Response.(ctlplfl.CPResp)
				if !ok || cpResp.Error == nil {
					t.Errorf("Expected CPResp with error in FuncIntrm.Response, got %T", intrm.Response)
					return
				}
				if tc.errorContains != "" && !strings.Contains(cpResp.Error.Message, tc.errorContains) {
					t.Errorf("Expected error message containing %q, got %q", tc.errorContains, cpResp.Error.Message)
				}
				if tc.expectedErrCode != "" && cpResp.Error.Code != tc.expectedErrCode {
					t.Errorf("Expected error code %q, got %q", tc.expectedErrCode, cpResp.Error.Code)
				}
				return
			}

			// Check success case
			if err != nil {
				t.Errorf("Expected no error, but got: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected non-nil result")
				return
			}

			// Decode the result to verify structure
			var funcIntrm funclib.FuncIntrm
			err = pmCmn.Decoder(pmCmn.GOB, result.([]byte), &funcIntrm)
			if err != nil {
				t.Errorf("Failed to decode result: %v", err)
				return
			}

			// Verify ownership key in commit changes if needed
			if tc.checkOwnership {
				var foundOwnership bool
				expectedOwnershipKey := fmt.Sprintf("/u/%s/v/", tc.expectedUserID)

				for _, chg := range funcIntrm.Changes {
					key := string(chg.Key)
					if strings.Contains(key, expectedOwnershipKey) && string(chg.Value) == "1" {
						foundOwnership = true
						break
					}
				}

				if !foundOwnership {
					t.Errorf("Expected ownership key for user %s, but not found in commit changes", tc.expectedUserID)
				}
			}
		})
	}
}

func TestWPCreateVdev_NilAuthorizer(t *testing.T) {
	// Ensure authorizer is nil
	authorizer = nil
	defer func() { authorizer = nil }()

	token, _ := createTestToken(testUserID1, "user", testSecret)
	cpReq := ctlplfl.CPReq{
		Token: token,
		Payload: ctlplfl.VdevReq{
			Vdev: &ctlplfl.VdevConfig{
				Size:       1073741824,
				ChunkCnt:   4,
				DataBlkCnt: 3,
			},
		},
	}

	// Should succeed even without authorizer (graceful degradation)
	result, err := WPCreateVdev(cpReq)
	if err != nil {
		t.Errorf("Expected success with nil authorizer, but got error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
}

func TestReadVdevInfo(t *testing.T) {
	// Initialize test authorizer
	authorizer = authz.NewAuthorizerWithConfig(authz.Config{
		authz.ReadVdevInfo: authz.FunctionPolicy{
			RBAC: []string{"admin", "user"},
			ABAC: []authz.ABACRule{
				{Argument: "vdev", Prefix: "v/"},
			},
		},
	})
	defer func() { authorizer = nil }()

	testCases := []struct {
		name            string
		setupData       func(storageiface.DataStore)
		setupToken      func() string
		vdevID          string
		expectError     bool
		errorContains   string
		expectedErrCode ctlplfl.CPErrCode
		replySize       int64
	}{
		{
			name: "SuccessfulRead_Owner",
			setupData: func(ds storageiface.DataStore) {
				// Setup vdev data
				setupVdevData(ds, testVdevID)
				// Setup ownership key
				ownershipKey := fmt.Sprintf("/u/%s/v/%s", testUserID1, testVdevID)
				ds.Write(ownershipKey, "1", "")
			},
			setupToken: func() string {
				token, _ := createTestToken(testUserID1, "user", testSecret)
				return token
			},
			vdevID:      testVdevID,
			expectError: false,
		},
		{
			name: "MissingToken",
			setupData: func(ds storageiface.DataStore) {
				setupVdevData(ds, testVdevID)
			},
			setupToken: func() string {
				return ""
			},
			vdevID:          testVdevID,
			expectError:     true,
			errorContains:   "Invalid Token",
			expectedErrCode: ctlplfl.ErrAuth,
		},
		{
			name: "UnauthorizedRole_RBAC",
			setupData: func(ds storageiface.DataStore) {
				setupVdevData(ds, testVdevID)
			},
			setupToken: func() string {
				token, _ := createTestToken(testUserID1, "viewer", testSecret)
				return token
			},
			vdevID:          testVdevID,
			expectError:     true,
			errorContains:   "User is not authorized",
			expectedErrCode: ctlplfl.ErrAuth,
		},
		{
			name: "UnauthorizedUser_NotOwner",
			setupData: func(ds storageiface.DataStore) {
				setupVdevData(ds, testVdevID)
				// Setup ownership for user2, but we'll try to access as user1
				ownershipKey := fmt.Sprintf("/u/%s/v/%s", testUserID2, testVdevID)
				ds.Write(ownershipKey, "1", "")
			},
			setupToken: func() string {
				token, _ := createTestToken(testUserID1, "user", testSecret)
				return token
			},
			vdevID:          testVdevID,
			expectError:     true,
			errorContains:   "User is not authorized",
			expectedErrCode: ctlplfl.ErrAuth,
		},
		{
			name: "InvalidRequest_EmptyID",
			setupData: func(ds storageiface.DataStore) {
				// No setup needed
			},
			setupToken: func() string {
				token, _ := createTestToken(testUserID1, "user", testSecret)
				return token
			},
			vdevID:          "",
			expectError:     true,
			errorContains:   "Invalid Request",
			expectedErrCode: ctlplfl.ErrFunc,
		},
		{
			name: "SuccessfulRead_RangeReadContinue",
			setupData: func(ds storageiface.DataStore) {
				setupVdevData(ds, testVdevID)
				// Add many chunks to ensure it exceeds the small reply buffer
				for i := range 100 {
					ds.Write(fmt.Sprintf("v/%s/c/%d/D.0", testVdevID, i), "nisd-001", "")
				}
				// Setup ownership key
				ownershipKey := fmt.Sprintf("/u/%s/v/%s", testUserID1, testVdevID)
				ds.Write(ownershipKey, "1", "")
			},
			setupToken: func() string {
				token, _ := createTestToken(testUserID1, "user", testSecret)
				return token
			},
			vdevID:      testVdevID,
			expectError: false,
			replySize:   512,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create memstore and setup test data
			ds := memstore.NewMemStore()
			colmfamily = "" // Use default column family

			if tc.setupData != nil {
				tc.setupData(ds)
			}

			rs := int64(4096)
			if tc.replySize > 0 {
				rs = tc.replySize
			}
			// Create mock PmdbCbArgs
			cbArgs := &PumiceDBServer.PmdbCbArgs{
				Store:     ds,
				ReplySize: rs,
			}

			// Build CPReq with GetReq as payload
			req := ctlplfl.GetReq{
				ID:     tc.vdevID,
				GetAll: false,
			}
			cpReq := ctlplfl.CPReq{
				Token:   tc.setupToken(),
				Payload: req,
			}

			// Call ReadVdevInfo
			result, err := ReadVdevInfo(cbArgs, cpReq)

			// Check error expectations
			if tc.expectError {
				if result == nil {
					t.Error("Expected non-nil result for encoded error response")
					return
				}
				var cpResp ctlplfl.CPResp
				if decErr := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp); decErr != nil {
					t.Fatalf("Failed to decode response: %v", decErr)
				}
				if cpResp.Error == nil {
					t.Errorf("Expected error response, got success")
					return
				}
				if tc.errorContains != "" && !strings.Contains(cpResp.Error.Message, tc.errorContains) {
					t.Errorf("Expected error message containing %q, got %q", tc.errorContains, cpResp.Error.Message)
				}
				if tc.expectedErrCode != "" && cpResp.Error.Code != tc.expectedErrCode {
					t.Errorf("Expected error code %q, got %q", tc.expectedErrCode, cpResp.Error.Code)
				}
				return
			}

			// Check success case
			if err != nil {
				t.Errorf("Expected no error, but got: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected non-nil result")
				return
			}

			var cpResp ctlplfl.CPResp
			if decErr := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp); decErr != nil {
				t.Fatalf("Failed to decode response: %v", decErr)
			}
			if cpResp.Error != nil {
				t.Errorf("Expected success response, got error: %s", cpResp.Err())
			}
		})
	}
}

func TestAPDeleteVdev(t *testing.T) {

	t.Log("Starting TestAPDeleteVdev")

	testVdevUUID := "28061cd0-1e01-11f1-a069-032bff036f03"
	testNisdUUID := "59ee0460-1e01-11f1-9566-83949aa998ea"
	testNisdUUID2 := "6aff1570-1e01-11f1-b677-94a5be37ab0f"

	// setupBasicVdevAndNisd seeds the store with a vdev that has one chunk
	// mapped to testNisdUUID and a complete n_cfg subtree for that NISD.
	setupBasicVdevAndNisd := func(ds storageiface.DataStore, vdevID, nisdID string) {
		ds.Write(fmt.Sprintf("v/%s/cfg/size", vdevID), "8589934592", "")
		ds.Write(fmt.Sprintf("v/%s/c/0/D.0", vdevID), nisdID, "")
		ds.Write(fmt.Sprintf("n/%s/%s", nisdID, vdevID), "D.0.0", "")
		ds.Write(fmt.Sprintf("%s/%s/d", NisdCfgKey, nisdID), testDev, "")
		ds.Write(fmt.Sprintf("%s/%s/pp", NisdCfgKey, nisdID), "8160", "")
		ds.Write(fmt.Sprintf("%s/%s/hv", NisdCfgKey, nisdID), testHV, "")
		ds.Write(fmt.Sprintf("%s/%s/ts", NisdCfgKey, nisdID), "1000000000000", "")
		ds.Write(fmt.Sprintf("%s/%s/as", NisdCfgKey, nisdID), "1000000000000", "")
		ds.Write(fmt.Sprintf("%s/%s/p", NisdCfgKey, nisdID), testPDU, "")
		ds.Write(fmt.Sprintf("%s/%s/r", NisdCfgKey, nisdID), testRack, "")
		ds.Write(fmt.Sprintf("%s/%s/pt", NisdCfgKey, nisdID), testPT, "")
		ds.Write(fmt.Sprintf("%s/%s/nic", NisdCfgKey, nisdID), "0", "")
	}

	setupHRWithNisd := func(nisdID string, availSize int64) {
		HR.Init()
		HR.AddNisd(&ctlplfl.Nisd{
			ID:            nisdID,
			AvailableSize: availSize,
			FailureDomain: []string{testPDU, testRack, testHV, testDev, testPT},
		})
	}

	testCases := []struct {
		name          string
		setupData     func(storageiface.DataStore)
		setupHR       func()
		vdevID        string
		token         string
		expectError   bool
		errorContains string
		verify        func(t *testing.T, ds storageiface.DataStore)
		replySize     int64
	}{
		{
			name: "SuccessfulDelete_WithChunks",
			setupData: func(ds storageiface.DataStore) {
				setupBasicVdevAndNisd(ds, testVdevUUID, testNisdUUID)
			},
			setupHR:     func() { setupHRWithNisd(testNisdUUID, 1073741824) },
			vdevID:      testVdevUUID,
			expectError: false,
			verify: func(t *testing.T, ds storageiface.DataStore) {
				// Vdev cfg key must be gone
				if _, err := ds.Read(fmt.Sprintf("v/%s/cfg/size", testVdevUUID), ""); err == nil {
					t.Error("Vdev metadata should be deleted")
				}
				// Chunk allocation key must be gone
				if _, err := ds.Read(fmt.Sprintf("v/%s/c/0/D.0", testVdevUUID), ""); err == nil {
					t.Error("Chunk allocation should be deleted")
				}
				// NISD reverse-mapping key must be gone
				if _, err := ds.Read(fmt.Sprintf("n/%s/%s", testNisdUUID, testVdevUUID), ""); err == nil {
					t.Error("NISD reverse mapping should be deleted")
				}
				// NISD available space must be refunded
				expectedAS := int64(1000000000000 + 8589934592)
				res, err := ds.Read(fmt.Sprintf("%s/%s/as", NisdCfgKey, testNisdUUID), "")
				if err != nil || string(res) != strconv.FormatInt(expectedAS, 10) {
					t.Errorf("Expected NISD AS %d, got %s", expectedAS, string(res))
				}
				// HR in-memory state must also be updated
				nisd, _ := HR.GetNisdByPDUID(testPDU, testNisdUUID)
				if nisd == nil || nisd.AvailableSize != expectedAS {
					t.Errorf("HR NISD AvailableSize not updated: got %v", nisd)
				}
			},
		},
		{
			name:        "SuccessfulDelete_VdevNotFound",
			setupData:   func(_ storageiface.DataStore) {},
			setupHR:     func() { HR.Init() },
			vdevID:      testVdevUUID,
			expectError: false,
			verify:      nil,
		},
		// multiple chunks spread across two NISDs
		// ensures the refund accumulates per NISD correctly
		{
			name: "SuccessfulDelete_MultiChunk_TwoNISDs",
			setupData: func(ds storageiface.DataStore) {
				// vdev has 2 chunks, each replica points to a different NISD
				ds.Write(fmt.Sprintf("v/%s/cfg/size", testVdevUUID), "17179869184", "") // 16 GiB
				ds.Write(fmt.Sprintf("v/%s/c/0/D.0", testVdevUUID), testNisdUUID, "")
				ds.Write(fmt.Sprintf("v/%s/c/1/D.0", testVdevUUID), testNisdUUID2, "")
				// reverse-mapping for both NISDs
				ds.Write(fmt.Sprintf("n/%s/%s", testNisdUUID, testVdevUUID), "D.0.0", "")
				ds.Write(fmt.Sprintf("n/%s/%s", testNisdUUID2, testVdevUUID), "D.0.1", "")
				// n_cfg for NISD 1
				for _, kv := range []struct{ k, v string }{
					{"d", testDev}, {"pp", "8160"}, {"hv", testHV},
					{"ts", "500000000000"}, {"as", "500000000000"},
					{"p", testPDU}, {"r", testRack}, {"pt", testPT}, {"nic", "0"},
				} {
					ds.Write(fmt.Sprintf("%s/%s/%s", NisdCfgKey, testNisdUUID, kv.k), kv.v, "")
				}
				// n_cfg for NISD 2
				for _, kv := range []struct{ k, v string }{
					{"d", testDev}, {"pp", "8160"}, {"hv", testHV},
					{"ts", "600000000000"}, {"as", "600000000000"},
					{"p", testPDU}, {"r", testRack}, {"pt", testPT}, {"nic", "0"},
				} {
					ds.Write(fmt.Sprintf("%s/%s/%s", NisdCfgKey, testNisdUUID2, kv.k), kv.v, "")
				}
			},
			setupHR: func() {
				HR.Init()
				nisd := &ctlplfl.Nisd{
					ID:            testNisdUUID,
					FailureDomain: []string{testPDU, testRack, testHV, testDev, testPT},
					AvailableSize: 500000000000,
				}
				nisd2 := &ctlplfl.Nisd{
					ID:            testNisdUUID2,
					FailureDomain: []string{testPDU, testRack, testHV, testDev, testPT},
					AvailableSize: 500000000000,
				}

				HR.AddNisd(nisd)

				t.Log("Added NISD to HR:", nisd.ID)
				HR.AddNisd(nisd2)

				t.Log("Added NISD to HR:", nisd2.ID)
			},
			vdevID:      testVdevUUID,
			expectError: false,

			verify: func(t *testing.T, ds storageiface.DataStore) {

				t.Log("Starting verification")

				_, err := ds.Read(fmt.Sprintf("v/%s/cfg/size", testVdevUUID), "")

				if err == nil {
					t.Error("Vdev metadata should be deleted")
				}

				_, err = ds.Read(fmt.Sprintf("v/%s/c/0/D.0", testVdevUUID), "")

				if err == nil {
					t.Error("Chunk allocation should be deleted")
				}

				_, err = ds.Read(fmt.Sprintf("n/%s/%s", testNisdUUID, testVdevUUID), "")

				if err == nil {
					t.Error("NISD reverse mapping should be deleted")
				}

				res, err := ds.Read(fmt.Sprintf("n_cfg/%s/as", testNisdUUID), "")

				expectedAS := 500000000000 + 8589934592

				if err != nil || string(res) != strconv.FormatInt(int64(expectedAS), 10) {
					t.Errorf("Expected NISD AS %d, got %s", expectedAS, string(res))
				}

				nisd, _ := HR.GetNisdByPDUID(testPDU, testNisdUUID)

				if nisd == nil || nisd.AvailableSize != int64(expectedAS) {
					t.Errorf("HR NISD AS not updated properly")
				}

				t.Log("Verification completed")
			},
		},
		{
			name: "DeleteVdev_WithMountInfo",
			setupData: func(ds storageiface.DataStore) {
				ds.Write(fmt.Sprintf("v/%s/cfg/size", testVdevUUID), "8589934592", "")
				// Setup mount counter and last updated LTS
				mcKey := fmt.Sprintf("v/%s/%s/%s", testVdevUUID, ctlplfl.MntKey, ctlplfl.MOUNT_COUNTER)
				luKey := fmt.Sprintf("v/%s/%s/%s", testVdevUUID, ctlplfl.MntKey, ctlplfl.LAST_UPDATED_LTS)
				ds.Write(mcKey, "5", "")
				ds.Write(luKey, time.Now().Format(time.RFC3339), "")

				// ownership key
				ownershipKey := fmt.Sprintf("/u/%s/v/%s", testAdminID, testVdevUUID)
				ds.Write(ownershipKey, "1", "")
			},
			setupHR: func() {
				HR.Init()
			},
			vdevID:      testVdevUUID,
			expectError: false,
			verify: func(t *testing.T, ds storageiface.DataStore) {
				// Verify Vdev metadata is deleted
				_, err := ds.Read(fmt.Sprintf("v/%s/cfg/size", testVdevUUID), "")
				if err == nil {
					t.Error("Vdev metadata should be deleted")
				}

				// Verify mount counter is deleted
				mcKey := fmt.Sprintf("v/%s/%s/%s", testVdevUUID, ctlplfl.MntKey, ctlplfl.MOUNT_COUNTER)
				_, err = ds.Read(mcKey, "")
				if err == nil {
					t.Error("Mount counter should be deleted")
				}

				// Verify last updated LTS is deleted
				luKey := fmt.Sprintf("v/%s/%s/%s", testVdevUUID, ctlplfl.MntKey, ctlplfl.LAST_UPDATED_LTS)
				_, err = ds.Read(luKey, "")
				if err == nil {
					t.Error("Last updated LTS should be deleted")
				}

				// Verify ownership key is deleted
				ownershipKey := fmt.Sprintf("/u/%s/v/%s", testAdminID, testVdevUUID)
				_, err = ds.Read(ownershipKey, "")
				if err == nil {
					t.Error("Ownership key should be deleted")
				}
			},
		},
		{
			name: "SuccessfulDelete_WithPFS",
			setupData: func(ds storageiface.DataStore) {
				testPFSUUID := "a1b2c3d4-0000-0000-0000-000000000001"
				setupBasicVdevAndNisd(ds, testVdevUUID, testNisdUUID)
				// pfs cfg key inside vdev namespace
				ds.Write(fmt.Sprintf("v/%s/cfg/pfs", testVdevUUID), testPFSUUID, "")
				// pfs-vdev reverse mapping
				ds.Write(fmt.Sprintf("pfs/%s/v/%s", testPFSUUID, testVdevUUID), "1", "")
			},
			setupHR:     func() { setupHRWithNisd(testNisdUUID, 1073741824) },
			vdevID:      testVdevUUID,
			expectError: false,
			verify: func(t *testing.T, ds storageiface.DataStore) {
				testPFSUUID := "a1b2c3d4-0000-0000-0000-000000000001"
				if _, err := ds.Read(fmt.Sprintf("v/%s/cfg/pfs", testVdevUUID), ""); err == nil {
					t.Error("vdev cfg/pfs key should be deleted")
				}
				if _, err := ds.Read(fmt.Sprintf("pfs/%s/v/%s", testPFSUUID, testVdevUUID), ""); err == nil {
					t.Error("pfs-vdev reverse mapping key should be deleted")
				}
			},
		},
	}

	for _, tc := range testCases {

		t.Run(tc.name, func(t *testing.T) {

			t.Log("Running test case:", tc.name)

			ds := memstore.NewMemStore()
			if tc.setupData != nil {
				tc.setupData(ds)
			}
			if tc.setupHR != nil {
				tc.setupHR()
			}

			// Set up authorizer to allow admin role for APDeleteVdev
			authorizer = authz.NewAuthorizerWithConfig(authz.Config{
				authz.APDeleteVdev: authz.FunctionPolicy{
					RBAC: []string{"admin"},
				},
			})
			defer func() { authorizer = nil }()

			rs := int64(4096)
			if tc.replySize > 0 {
				rs = tc.replySize
			}
			cbArgs := &PumiceDBServer.PmdbCbArgs{
				Store:     ds,
				ReplySize: rs,
			}

			// Use provided token or default admin token
			token := tc.token
			if token == "" && !tc.expectError {
				token, _ = createTestToken(testAdminID, "admin", testSecret)
			}
			if token == "" && tc.name != "Error_MissingToken" {
				token, _ = createTestToken(testAdminID, "admin", testSecret)
			}

			cpReq := ctlplfl.CPReq{
				Token:   token,
				Payload: ctlplfl.DeleteVdevReq{ID: tc.vdevID},
			}

			result, err := APDeleteVdev(cpReq, cbArgs)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			var cpResp ctlplfl.CPResp
			if decErr := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp); decErr != nil {
				t.Fatalf("Failed to decode response: %v", decErr)
			}

			t.Log("Decoded CPResp errorMsg:", cpResp.Err())

			if tc.expectError {
				if cpResp.Error == nil {
					t.Errorf("Expected error response but got success")
					return
				}
				if tc.errorContains != "" && !strings.Contains(cpResp.Error.Message, tc.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tc.errorContains, cpResp.Error.Message)
				}
				return
			}

			if cpResp.Error != nil {
				t.Errorf("Expected success response, got error: %s", cpResp.Err())
			}

			if tc.verify != nil {
				t.Log("Running verify")
				tc.verify(t, ds)
			}

			t.Log("Test case finished:", tc.name)
		})
	}
}

func TestReadNisdListWithAvailSize(t *testing.T) {
	testNisdUUID := "59ee0460-1e01-11f1-9566-83949aa998ea"
	testNisdUUID2 := "6aff1570-1e01-11f1-b677-94a5be37ab0f"

	writeNisdToStore := func(ds storageiface.DataStore, nisdID string, availSize int64) {
		ds.Write(fmt.Sprintf("%s/%s/d", NisdCfgKey, nisdID), testDev, "")
		ds.Write(fmt.Sprintf("%s/%s/pp", NisdCfgKey, nisdID), "8160", "")
		ds.Write(fmt.Sprintf("%s/%s/hv", NisdCfgKey, nisdID), testHV, "")
		ds.Write(fmt.Sprintf("%s/%s/ts", NisdCfgKey, nisdID), "1000000000000", "")
		ds.Write(fmt.Sprintf("%s/%s/as", NisdCfgKey, nisdID), strconv.FormatInt(availSize, 10), "")
		ds.Write(fmt.Sprintf("%s/%s/p", NisdCfgKey, nisdID), testPDU, "")
		ds.Write(fmt.Sprintf("%s/%s/r", NisdCfgKey, nisdID), testRack, "")
		ds.Write(fmt.Sprintf("%s/%s/pt", NisdCfgKey, nisdID), testPT, "")
		ds.Write(fmt.Sprintf("%s/%s/nic", NisdCfgKey, nisdID), "0", "")
	}

	testCases := []struct {
		name            string
		setupData       func(storageiface.DataStore)
		token           string
		expectError     bool
		errorContains   string
		expectedErrCode ctlplfl.CPErrCode
		verifyPayload   func(t *testing.T, payload any)
	}{
		{
			name: "Success_SingleNisd",
			setupData: func(ds storageiface.DataStore) {
				writeNisdToStore(ds, testNisdUUID, 1073741824)
			},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				list, ok := payload.([]ctlplfl.NisdListAvailSize)
				if !ok {
					t.Fatalf("Expected []NisdListAvailSize payload, got %T", payload)
				}
				if len(list) != 1 {
					t.Fatalf("Expected 1 entry, got %d", len(list))
				}
				if list[0].ID != testNisdUUID {
					t.Errorf("Expected ID %q, got %q", testNisdUUID, list[0].ID)
				}
				if list[0].AvailableSize != 1073741824 {
					t.Errorf("Expected AvailableSize 1073741824, got %d", list[0].AvailableSize)
				}
			},
		},
		{
			name: "Success_MultipleNisds",
			setupData: func(ds storageiface.DataStore) {
				writeNisdToStore(ds, testNisdUUID, 2000000000)
				writeNisdToStore(ds, testNisdUUID2, 5000000000)
			},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				list, ok := payload.([]ctlplfl.NisdListAvailSize)
				if !ok {
					t.Fatalf("Expected []NisdListAvailSize payload, got %T", payload)
				}
				if len(list) != 2 {
					t.Fatalf("Expected 2 entries, got %d", len(list))
				}
			},
		},
		{
			name:        "Success_EmptyStore",
			setupData:   func(_ storageiface.DataStore) {},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				list, ok := payload.([]ctlplfl.NisdListAvailSize)
				if !ok {
					t.Fatalf("Expected []NisdListAvailSize payload, got %T", payload)
				}
				if len(list) != 0 {
					t.Errorf("Expected empty list, got %d entries", len(list))
				}
			},
		},
		{
			name: "Error_MissingToken",
			setupData: func(ds storageiface.DataStore) {
				writeNisdToStore(ds, testNisdUUID, 1073741824)
			},
			token:           "",
			expectError:     true,
			errorContains:   "user token is required",
			expectedErrCode: ctlplfl.ErrAuth,
		},
		{
			name: "Error_UnauthorizedRole",
			setupData: func(ds storageiface.DataStore) {
				writeNisdToStore(ds, testNisdUUID, 1073741824)
			},
			token: func() string {
				tk, _ := createTestToken(testUserID1, "user", testSecret)
				return tk
			}(),
			expectError:     true,
			errorContains:   "authorization failed",
			expectedErrCode: ctlplfl.ErrAuth,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			authorizer = authz.NewAuthorizerWithConfig(authz.Config{
				authz.ReadAllNisdConfigs: authz.FunctionPolicy{
					RBAC: []string{"admin"},
				},
			})
			defer func() { authorizer = nil }()

			ds := memstore.NewMemStore()
			colmfamily = ""
			if tc.setupData != nil {
				tc.setupData(ds)
			}

			cbArgs := &PumiceDBServer.PmdbCbArgs{
				Store:     ds,
				ReplySize: 4096,
			}

			token := tc.token
			if token == "" && !tc.expectError {
				token, _ = createTestToken(testAdminID, "admin", testSecret)
			}

			cpReq := ctlplfl.CPReq{
				Token:   token,
				Payload: ctlplfl.GetReq{Fields: ctlplfl.NisdAvailSizeFields},
			}
			result, err := ReadAllNisdConfigs(cbArgs, cpReq)
			if err != nil {
				t.Fatalf("Unexpected Go-level error: %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			var cpResp ctlplfl.CPResp
			if decErr := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp); decErr != nil {
				t.Fatalf("Failed to decode response: %v", decErr)
			}

			if tc.expectError {
				if cpResp.Error == nil {
					t.Errorf("Expected error response but got success")
					return
				}
				if tc.errorContains != "" && !strings.Contains(cpResp.Error.Message, tc.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tc.errorContains, cpResp.Error.Message)
				}
				if tc.expectedErrCode != "" && cpResp.Error.Code != tc.expectedErrCode {
					t.Errorf("Expected error code %q, got %q", tc.expectedErrCode, cpResp.Error.Code)
				}
				return
			}

			if cpResp.Error != nil {
				t.Errorf("Expected success, got error: %s", cpResp.Err())
				return
			}

			if tc.verifyPayload != nil {
				tc.verifyPayload(t, cpResp.Payload)
			}
		})
	}
}

func TestReadAllNisdConfigs(t *testing.T) {
	nisdUUID1 := "59ee0460-1e01-11f1-9566-83949aa998ea"
	nisdUUID2 := "6aff1570-1e01-11f1-b677-94a5be37ab0f"

	writeNisd := func(ds storageiface.DataStore, nisdID string, totalSize, availSize int64) {
		ds.Write(fmt.Sprintf("%s/%s/d", NisdCfgKey, nisdID), testDev, "")
		ds.Write(fmt.Sprintf("%s/%s/pp", NisdCfgKey, nisdID), "8160", "")
		ds.Write(fmt.Sprintf("%s/%s/hv", NisdCfgKey, nisdID), testHV, "")
		ds.Write(fmt.Sprintf("%s/%s/ts", NisdCfgKey, nisdID), strconv.FormatInt(totalSize, 10), "")
		ds.Write(fmt.Sprintf("%s/%s/as", NisdCfgKey, nisdID), strconv.FormatInt(availSize, 10), "")
		ds.Write(fmt.Sprintf("%s/%s/p", NisdCfgKey, nisdID), testPDU, "")
		ds.Write(fmt.Sprintf("%s/%s/r", NisdCfgKey, nisdID), testRack, "")
		ds.Write(fmt.Sprintf("%s/%s/pt", NisdCfgKey, nisdID), testPT, "")
		ds.Write(fmt.Sprintf("%s/%s/nic", NisdCfgKey, nisdID), "0", "")
	}

	adminToken := func() string {
		tk, _ := createTestToken(testAdminID, "admin", testSecret)
		return tk
	}()

	authCfg := authz.Config{
		authz.ReadAllNisdConfigs: authz.FunctionPolicy{RBAC: []string{"admin"}},
	}

	t.Run("NoFilter_ReturnsFullNisdList", func(t *testing.T) {
		authorizer = authz.NewAuthorizerWithConfig(authCfg)
		defer func() { authorizer = nil }()

		ds := memstore.NewMemStore()
		colmfamily = ""
		writeNisd(ds, nisdUUID1, 1000000000000, 800000000000)
		writeNisd(ds, nisdUUID2, 2000000000000, 1500000000000)

		cbArgs := &PumiceDBServer.PmdbCbArgs{Store: ds, ReplySize: 4096}
		// no Fields set — expect full []Nisd
		cpReq := ctlplfl.CPReq{Token: adminToken, Payload: ctlplfl.GetReq{GetAll: true}}
		result, err := ReadAllNisdConfigs(cbArgs, cpReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var cpResp ctlplfl.CPResp
		if err := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if cpResp.Error != nil {
			t.Fatalf("expected success, got: %s", cpResp.Error.Message)
		}

		nisds, ok := cpResp.Payload.([]ctlplfl.Nisd)
		if !ok {
			t.Fatalf("expected []Nisd payload, got %T", cpResp.Payload)
		}
		if len(nisds) != 2 {
			t.Errorf("expected 2 NISDs, got %d", len(nisds))
		}
		for _, n := range nisds {
			if n.TotalSize == 0 {
				t.Errorf("NISD %s: expected non-zero TotalSize in full response", n.ID)
			}
		}
	})

	t.Run("WithFilter_ReturnsAvailSizeProjection", func(t *testing.T) {
		authorizer = authz.NewAuthorizerWithConfig(authCfg)
		defer func() { authorizer = nil }()

		ds := memstore.NewMemStore()
		colmfamily = ""
		writeNisd(ds, nisdUUID1, 1000000000000, 800000000000)
		writeNisd(ds, nisdUUID2, 2000000000000, 1500000000000)

		cbArgs := &PumiceDBServer.PmdbCbArgs{Store: ds, ReplySize: 4096}
		cpReq := ctlplfl.CPReq{
			Token:   adminToken,
			Payload: ctlplfl.GetReq{Fields: ctlplfl.NisdAvailSizeFields},
		}
		result, err := ReadAllNisdConfigs(cbArgs, cpReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var cpResp ctlplfl.CPResp
		if err := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if cpResp.Error != nil {
			t.Fatalf("expected success, got: %s", cpResp.Error.Message)
		}

		list, ok := cpResp.Payload.([]ctlplfl.NisdListAvailSize)
		if !ok {
			t.Fatalf("expected []NisdListAvailSize payload, got %T", cpResp.Payload)
		}
		if len(list) != 2 {
			t.Errorf("expected 2 entries, got %d", len(list))
		}
		ids := map[string]bool{}
		for _, n := range list {
			ids[n.ID] = true
			if n.TotalSize == 0 || n.AvailableSize == 0 {
				t.Errorf("NISD %s: TotalSize or AvailableSize is zero", n.ID)
			}
		}
		if !ids[nisdUUID1] || !ids[nisdUUID2] {
			t.Errorf("expected both NISD UUIDs in result, got %v", ids)
		}
	})

	t.Run("WithFilter_EmptyStore", func(t *testing.T) {
		authorizer = authz.NewAuthorizerWithConfig(authCfg)
		defer func() { authorizer = nil }()

		ds := memstore.NewMemStore()
		colmfamily = ""
		cbArgs := &PumiceDBServer.PmdbCbArgs{Store: ds, ReplySize: 4096}
		cpReq := ctlplfl.CPReq{
			Token:   adminToken,
			Payload: ctlplfl.GetReq{Fields: ctlplfl.NisdAvailSizeFields},
		}
		result, _ := ReadAllNisdConfigs(cbArgs, cpReq)
		var cpResp ctlplfl.CPResp
		pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp)
		list, ok := cpResp.Payload.([]ctlplfl.NisdListAvailSize)
		if !ok {
			t.Fatalf("expected []NisdListAvailSize, got %T", cpResp.Payload)
		}
		if len(list) != 0 {
			t.Errorf("expected empty list, got %d entries", len(list))
		}
	})

	t.Run("AuthFailure_InvalidToken", func(t *testing.T) {
		authorizer = authz.NewAuthorizerWithConfig(authCfg)
		defer func() { authorizer = nil }()

		ds := memstore.NewMemStore()
		colmfamily = ""
		cbArgs := &PumiceDBServer.PmdbCbArgs{Store: ds, ReplySize: 4096}

		for _, desc := range []struct {
			name  string
			token string
		}{
			{"MissingToken", ""},
			{"UserRole", func() string { tk, _ := createTestToken(testUserID1, "user", testSecret); return tk }()},
		} {
			t.Run(desc.name, func(t *testing.T) {
				cpReq := ctlplfl.CPReq{Token: desc.token, Payload: ctlplfl.GetReq{}}
				result, _ := ReadAllNisdConfigs(cbArgs, cpReq)
				var cpResp ctlplfl.CPResp
				pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp)
				if cpResp.Error == nil {
					t.Error("expected auth error, got success")
				}
				if cpResp.Error.Code != ctlplfl.ErrAuth {
					t.Errorf("expected ErrAuth, got %q", cpResp.Error.Code)
				}
			})
		}
	})
}

func TestReadAllResources(t *testing.T) {
	testNisdID := "59ee0460-1e01-11f1-9566-83949aa998ea"
	testRackID := "rack-001"
	testPDUID := "pdu-001"
	testHVID := "hv-001"
	testDevID := "dev-001"
	testPTID := "pt-001"

	writeNisd := func(ds storageiface.DataStore, id string) {
		ds.Write(fmt.Sprintf("%s/%s/d", NisdCfgKey, id), testDev, "")
		ds.Write(fmt.Sprintf("%s/%s/pp", NisdCfgKey, id), "8160", "")
		ds.Write(fmt.Sprintf("%s/%s/hv", NisdCfgKey, id), testHV, "")
		ds.Write(fmt.Sprintf("%s/%s/ts", NisdCfgKey, id), "2000000000", "")
		ds.Write(fmt.Sprintf("%s/%s/as", NisdCfgKey, id), "1000000000", "")
		ds.Write(fmt.Sprintf("%s/%s/p", NisdCfgKey, id), testPDU, "")
		ds.Write(fmt.Sprintf("%s/%s/r", NisdCfgKey, id), testRack, "")
		ds.Write(fmt.Sprintf("%s/%s/pt", NisdCfgKey, id), testPT, "")
		ds.Write(fmt.Sprintf("%s/%s/nic", NisdCfgKey, id), "0", "")
	}

	writeRack := func(ds storageiface.DataStore, id string) {
		ds.Write(fmt.Sprintf("r/%s/nm", id), "rack-name", "")
		ds.Write(fmt.Sprintf("r/%s/l", id), "dc1", "")
		ds.Write(fmt.Sprintf("r/%s/p", id), testPDUID, "")
	}

	writePDU := func(ds storageiface.DataStore, id string) {
		ds.Write(fmt.Sprintf("p/%s/nm", id), "pdu-name", "")
		ds.Write(fmt.Sprintf("p/%s/l", id), "dc1", "")
		ds.Write(fmt.Sprintf("p/%s/pw", id), "10000", "")
	}

	writeHV := func(ds storageiface.DataStore, id string) {
		ds.Write(fmt.Sprintf("hv/%s/nm", id), "hv-name", "")
		ds.Write(fmt.Sprintf("hv/%s/r", id), testRackID, "")
		ds.Write(fmt.Sprintf("hv/%s/rdma", id), "false", "")
	}

	writeDev := func(ds storageiface.DataStore, id string) {
		ds.Write(fmt.Sprintf("d_cfg/%s/nm", id), "nvme-001", "")
		ds.Write(fmt.Sprintf("d_cfg/%s/dp", id), "/dev/nvme0n1", "")
		ds.Write(fmt.Sprintf("d_cfg/%s/sz", id), "4000000000000", "")
	}

	writePT := func(ds storageiface.DataStore, id string) {
		ds.Write(fmt.Sprintf("pt/%s/ptp", id), "/dev/nvme0n1p1", "")
		ds.Write(fmt.Sprintf("pt/%s/d", id), testDevID, "")
		ds.Write(fmt.Sprintf("pt/%s/sz", id), "2000000000000", "")
	}

	adminToken := func() string {
		tk, _ := createTestToken(testAdminID, "admin", testSecret)
		return tk
	}

	testCases := []struct {
		name            string
		setupData       func(storageiface.DataStore)
		token           string
		req             ctlplfl.GetResourceReq
		expectError     bool
		errorContains   string
		expectedErrCode ctlplfl.CPErrCode
		verifyPayload   func(t *testing.T, payload any)
	}{
		{
			name: "Success_Nisd_GetAll",
			setupData: func(ds storageiface.DataStore) {
				writeNisd(ds, testNisdID)
			},
			req:         ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourceNisd, GetAll: true},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				resp, ok := payload.(ctlplfl.ResourceListResp)
				if !ok {
					t.Fatalf("Expected ResourceListResp, got %T", payload)
				}
				if resp.ResourceType != ctlplfl.ResourceNisd {
					t.Errorf("Expected resource type %q, got %q", ctlplfl.ResourceNisd, resp.ResourceType)
				}
				if len(resp.Nisds) != 1 {
					t.Fatalf("Expected 1 NISD, got %d", len(resp.Nisds))
				}
				if resp.Nisds[0].ID != testNisdID {
					t.Errorf("Expected NISD ID %q, got %q", testNisdID, resp.Nisds[0].ID)
				}
			},
		},
		{
			name: "Success_Rack_GetAll",
			setupData: func(ds storageiface.DataStore) {
				writeRack(ds, testRackID)
			},
			req:         ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourceRack, GetAll: true},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				resp, ok := payload.(ctlplfl.ResourceListResp)
				if !ok {
					t.Fatalf("Expected ResourceListResp, got %T", payload)
				}
				if len(resp.Racks) != 1 {
					t.Fatalf("Expected 1 Rack, got %d", len(resp.Racks))
				}
				if resp.Racks[0].ID != testRackID {
					t.Errorf("Expected Rack ID %q, got %q", testRackID, resp.Racks[0].ID)
				}
			},
		},
		{
			name: "Success_PDU_GetAll",
			setupData: func(ds storageiface.DataStore) {
				writePDU(ds, testPDUID)
			},
			req:         ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourcePDU, GetAll: true},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				resp, ok := payload.(ctlplfl.ResourceListResp)
				if !ok {
					t.Fatalf("Expected ResourceListResp, got %T", payload)
				}
				if len(resp.PDUs) != 1 {
					t.Fatalf("Expected 1 PDU, got %d", len(resp.PDUs))
				}
				if resp.PDUs[0].ID != testPDUID {
					t.Errorf("Expected PDU ID %q, got %q", testPDUID, resp.PDUs[0].ID)
				}
			},
		},
		{
			name: "Success_Hypervisor_GetAll",
			setupData: func(ds storageiface.DataStore) {
				writeHV(ds, testHVID)
			},
			req:         ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourceHypervisor, GetAll: true},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				resp, ok := payload.(ctlplfl.ResourceListResp)
				if !ok {
					t.Fatalf("Expected ResourceListResp, got %T", payload)
				}
				if len(resp.Hypervisors) != 1 {
					t.Fatalf("Expected 1 Hypervisor, got %d", len(resp.Hypervisors))
				}
				if resp.Hypervisors[0].ID != testHVID {
					t.Errorf("Expected HV ID %q, got %q", testHVID, resp.Hypervisors[0].ID)
				}
			},
		},
		{
			name: "Success_Device_GetAll",
			setupData: func(ds storageiface.DataStore) {
				writeDev(ds, testDevID)
			},
			req:         ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourceDevice, GetAll: true},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				resp, ok := payload.(ctlplfl.ResourceListResp)
				if !ok {
					t.Fatalf("Expected ResourceListResp, got %T", payload)
				}
				if len(resp.Devices) != 1 {
					t.Fatalf("Expected 1 Device, got %d", len(resp.Devices))
				}
				if resp.Devices[0].ID != testDevID {
					t.Errorf("Expected Device ID %q, got %q", testDevID, resp.Devices[0].ID)
				}
			},
		},
		{
			name: "Success_Partition_GetAll",
			setupData: func(ds storageiface.DataStore) {
				writePT(ds, testPTID)
			},
			req:         ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourcePartition, GetAll: true},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				resp, ok := payload.(ctlplfl.ResourceListResp)
				if !ok {
					t.Fatalf("Expected ResourceListResp, got %T", payload)
				}
				if len(resp.Partitions) != 1 {
					t.Fatalf("Expected 1 Partition, got %d", len(resp.Partitions))
				}
				if resp.Partitions[0].PartitionID != testPTID {
					t.Errorf("Expected Partition ID %q, got %q", testPTID, resp.Partitions[0].PartitionID)
				}
			},
		},
		{
			name: "Success_Nisd_GetByID",
			setupData: func(ds storageiface.DataStore) {
				writeNisd(ds, testNisdID)
			},
			req:         ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourceNisd, ID: testNisdID, GetAll: false},
			expectError: false,
			verifyPayload: func(t *testing.T, payload any) {
				resp, ok := payload.(ctlplfl.ResourceListResp)
				if !ok {
					t.Fatalf("Expected ResourceListResp, got %T", payload)
				}
				if len(resp.Nisds) != 1 {
					t.Fatalf("Expected 1 NISD, got %d", len(resp.Nisds))
				}
			},
		},
		{
			name:            "Error_UnknownResourceType",
			setupData:       func(_ storageiface.DataStore) {},
			token:           adminToken(),
			req:             ctlplfl.GetResourceReq{ResourceType: "unknown", GetAll: true},
			expectError:     true,
			errorContains:   "unknown resource type",
			expectedErrCode: ctlplfl.ErrFunc,
		},
		{
			name:            "Error_MissingToken",
			setupData:       func(_ storageiface.DataStore) {},
			token:           "",
			req:             ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourceNisd, GetAll: true},
			expectError:     true,
			errorContains:   "user token is required",
			expectedErrCode: ctlplfl.ErrAuth,
		},
		{
			name:      "Error_UnauthorizedRole",
			setupData: func(_ storageiface.DataStore) {},
			token: func() string {
				tk, _ := createTestToken(testUserID1, "user", testSecret)
				return tk
			}(),
			req:             ctlplfl.GetResourceReq{ResourceType: ctlplfl.ResourceNisd, GetAll: true},
			expectError:     true,
			errorContains:   "authorization failed",
			expectedErrCode: ctlplfl.ErrAuth,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			authorizer = authz.NewAuthorizerWithConfig(authz.Config{
				authz.ReadAllResources: authz.FunctionPolicy{
					RBAC: []string{"admin"},
				},
			})
			defer func() { authorizer = nil }()

			ds := memstore.NewMemStore()
			colmfamily = ""
			if tc.setupData != nil {
				tc.setupData(ds)
			}

			cbArgs := &PumiceDBServer.PmdbCbArgs{
				Store:     ds,
				ReplySize: 4096,
			}

			token := tc.token
			if token == "" && !tc.expectError {
				token = adminToken()
			}

			cpReq := ctlplfl.CPReq{
				Token:   token,
				Payload: tc.req,
			}

			result, err := ReadAllResources(cbArgs, cpReq)
			if err != nil {
				t.Fatalf("Unexpected Go-level error: %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			var cpResp ctlplfl.CPResp
			if decErr := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp); decErr != nil {
				t.Fatalf("Failed to decode response: %v", decErr)
			}

			if tc.expectError {
				if cpResp.Error == nil {
					t.Errorf("Expected error response but got success")
					return
				}
				if tc.errorContains != "" && !strings.Contains(cpResp.Error.Message, tc.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tc.errorContains, cpResp.Error.Message)
				}
				if tc.expectedErrCode != "" && cpResp.Error.Code != tc.expectedErrCode {
					t.Errorf("Expected error code %q, got %q", tc.expectedErrCode, cpResp.Error.Code)
				}
				return
			}

			if cpResp.Error != nil {
				t.Errorf("Expected success, got error: %s", cpResp.Err())
				return
			}

			if tc.verifyPayload != nil {
				tc.verifyPayload(t, cpResp.Payload)
			}
		})
	}
}

func TestWPMountVdev(t *testing.T) {
	authorizer = authz.NewAuthorizerWithConfig(authz.Config{
		authz.WPMountVdev: authz.FunctionPolicy{
			RBAC: []string{"admin", "user"},
		},
	})
	defer func() { authorizer = nil }()

	testCases := []struct {
		name        string
		setupToken  func() string
		vdevID      string
		expectError bool
	}{
		{
			name: "Success_User",
			setupToken: func() string {
				token, _ := createTestToken(testUserID1, "user", testSecret)
				return token
			},
			vdevID:      testVdevID,
			expectError: false,
		},
		{
			name: "MissingToken",
			setupToken: func() string {
				return ""
			},
			vdevID:      testVdevID,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cpReq := ctlplfl.CPReq{
				Token: tc.setupToken(),
				Payload: ctlplfl.MountVdevRequest{
					VdevID: tc.vdevID,
				},
			}
			result, err := WPMountVdev(cpReq)
			if tc.expectError {
				if err == nil && result != nil {
					// Check if result is encoded error
					var intrm funclib.FuncIntrm
					if decErr := pmCmn.Decoder(pmCmn.GOB, result.([]byte), &intrm); decErr == nil {
						if cpResp, ok := intrm.Response.(ctlplfl.CPResp); ok && cpResp.Error != nil {
							return
						}
					}
				}
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// func TestAPMountVdev(t *testing.T) {
// 	authorizer = authz.NewAuthorizerWithConfig(authz.Config{
// 		authz.APMountVdev: authz.FunctionPolicy{
// 			RBAC: []string{"admin", "user"},
// 			ABAC: []authz.ABACRule{
// 				{Argument: "vdev", Prefix: "v/"},
// 			},
// 		},
// 	})
// 	defer func() { authorizer = nil }()

// 	ds := memstore.NewMemStore()
// 	setupVdevData(ds, testVdevID)
// 	// Setup ownership
// 	ownershipKey := fmt.Sprintf("/u/%s/v/%s", testUserID1, testVdevID)
// 	ds.Write(ownershipKey, "1", "")

// 	cbArgs := &PumiceDBServer.PmdbCbArgs{
// 		Store:     ds,
// 		ReplySize: 4096,
// 	}

// 	token, _ := createTestToken(testUserID1, "user", testSecret)
// 	req := ctlplfl.MountVdevRequest{VdevID: testVdevID}

// 	// Create context for AP call
// 	now := time.Now().Truncate(time.Second) // Use truncated time for consistency
// 	intrm := funclib.FuncIntrm{
// 		Response: ctlplfl.VdevMountInfo{
// 			LastUpdatedLTS: time.Now(),
// 		},
// 	}
// 	intrmBuf, _ := pmCmn.Encoder(pmCmn.GOB, intrm)
// 	cbArgs.AppData = (*C.uchar)(C.CBytes(intrmBuf))
// 	cbArgs.AppDataSize = C.size_t(len(intrmBuf))
// 	defer C.free(unsafe.Pointer(cbArgs.AppData))

// 	cpReq := ctlplfl.CPReq{
// 		Token:   token,
// 		Payload: req,
// 	}

// 	// 1st Mount
// 	result, err := APMountVdev(cpReq, cbArgs)
// 	if err != nil {
// 		t.Fatalf("1st mount failed: %v", err)
// 	}
// 	var cpResp ctlplfl.CPResp
// 	pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp)
// 	if cpResp.Error != nil {
// 		t.Fatalf("1st mount error: %s", cpResp.Err())
// 	}
// 	mountInfo := cpResp.Payload.(ctlplfl.VdevMountInfo)
// 	if mountInfo.MountCounter != 1 {
// 		t.Errorf("Expected counter 1, got %d", mountInfo.MountCounter)
// 	}

// 	// 2nd Mount (Immediate - should fail due to time window)
// 	result, err = APMountVdev(cpReq, cbArgs)
// 	pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp)
// 	if cpResp.Error == nil {
// 		t.Errorf("Expected failure for immediate remount, but got success")
// 	}

// 	// 3rd Mount (After window)
// 	future := now.Add(ctlplfl.VDEV_MOUNT_LIMIT + time.Second)
// 	resp := intrm.Response.(ctlplfl.VdevMountInfo)
// 	resp.LastUpdatedLTS = future
// 	intrm.Response = resp
// 	intrmBuf, _ = pmCmn.Encoder(pmCmn.GOB, intrm)
// 	cbArgs.AppData = (*C.uchar)(C.CBytes(intrmBuf))
// 	cbArgs.AppDataSize = C.size_t(len(intrmBuf))

// 	result, err = APMountVdev(cpReq, cbArgs)
// 	if err != nil {
// 		t.Fatalf("3rd mount failed: %v", err)
// 	}
// 	pmCmn.Decoder(pmCmn.GOB, result.([]byte), &cpResp)
// 	if cpResp.Error != nil {
// 		t.Fatalf("3rd mount error: %s", cpResp.Err())
// 	}
// 	mountInfo = cpResp.Payload.(ctlplfl.VdevMountInfo)
// 	if mountInfo.MountCounter != 2 {
// 		t.Errorf("Expected counter 2, got %d", mountInfo.MountCounter)
// 	}
// }

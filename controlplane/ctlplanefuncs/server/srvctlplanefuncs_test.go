package srvctlplanefuncs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

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
)

// TestMain initializes the test environment
func TestMain(m *testing.M) {
	// Initialize xlog to prevent nil pointer errors
	logLevel := "INFO"
	log.InitXlog("/tmp/test.log", &logLevel)

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
					Vdev: &ctlplfl.VdevCfg{
						Size:       tc.vdevSize,
						NumChunks:  uint32(tc.numChunks),
						NumReplica: uint8(tc.numReplica),
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
			Vdev: &ctlplfl.VdevCfg{
				Size:       1073741824,
				NumChunks:  4,
				NumReplica: 3,
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
					ds.Write(fmt.Sprintf("v/%s/c/%d/R.0", testVdevID, i), "nisd-001", "")
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
		ds.Write(fmt.Sprintf("v/%s/c/0/R.0", vdevID), nisdID, "")
		ds.Write(fmt.Sprintf("n/%s/%s", nisdID, vdevID), "R.0.0", "")
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
				if _, err := ds.Read(fmt.Sprintf("v/%s/c/0/R.0", testVdevUUID), ""); err == nil {
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
				ds.Write(fmt.Sprintf("v/%s/c/0/R.0", testVdevUUID), testNisdUUID, "")
				ds.Write(fmt.Sprintf("v/%s/c/1/R.0", testVdevUUID), testNisdUUID2, "")
				// reverse-mapping for both NISDs
				ds.Write(fmt.Sprintf("n/%s/%s", testNisdUUID, testVdevUUID), "R.0.0", "")
				ds.Write(fmt.Sprintf("n/%s/%s", testNisdUUID2, testVdevUUID), "R.0.1", "")
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
				HR.AddNisd(&ctlplfl.Nisd{
					ID:            testNisdUUID,
					AvailableSize: 500000000000,
					FailureDomain: []string{testPDU, testRack, testHV, testDev, testPT},
				})
				HR.AddNisd(&ctlplfl.Nisd{
					ID:            testNisdUUID2,
					AvailableSize: 600000000000,
					FailureDomain: []string{testPDU, testRack, testHV, testDev, testPT},
				})
			},
			vdevID:      testVdevUUID,
			expectError: false,
			verify: func(t *testing.T, ds storageiface.DataStore) {
				// Both chunk entries must be deleted
				for _, chunkKey := range []string{
					fmt.Sprintf("v/%s/c/0/R.0", testVdevUUID),
					fmt.Sprintf("v/%s/c/1/R.0", testVdevUUID),
				} {
					if _, err := ds.Read(chunkKey, ""); err == nil {
						t.Errorf("chunk key %q should be deleted", chunkKey)
					}
				}
				// Each NISD must be refunded exactly one CHUNK_SIZE
				for nisdID, baseAS := range map[string]int64{
					testNisdUUID:  500000000000,
					testNisdUUID2: 600000000000,
				} {
					expected := baseAS + ctlplfl.CHUNK_SIZE
					res, err := ds.Read(fmt.Sprintf("%s/%s/as", NisdCfgKey, nisdID), "")
					if err != nil || string(res) != strconv.FormatInt(expected, 10) {
						t.Errorf("NISD %s: expected AS %d, got %s (err=%v)", nisdID, expected, string(res), err)
					}
					hrNisd, err := HR.GetNisdByPDUID(testPDU, nisdID)
					if err != nil || hrNisd == nil || hrNisd.AvailableSize != expected {
						t.Errorf("HR NISD %s AvailableSize not updated correctly", nisdID)
					}
				}
			},
		},
		{
			name: "SuccessfulDelete_RangeReadContinue",
			setupData: func(ds storageiface.DataStore) {
				ds.Write(fmt.Sprintf("v/%s/cfg/size", testVdevUUID), "858993459200", "") // 100 * 8GB
				for i := range 100 {
					ds.Write(fmt.Sprintf("v/%s/c/%d/R.0", testVdevUUID, i), testNisdUUID, "")
					ds.Write(fmt.Sprintf("n/%s/%s", testNisdUUID, testVdevUUID), fmt.Sprintf("R.0.%d", i), "")
				}
				// n_cfg for NISD 1
				for _, kv := range []struct{ k, v string }{
					{"d", testDev}, {"pp", "8160"}, {"hv", testHV},
					{"ts", "5000000000000"}, {"as", "5000000000000"},
					{"p", testPDU}, {"r", testRack}, {"pt", testPT}, {"nic", "0"},
				} {
					ds.Write(fmt.Sprintf("%s/%s/%s", NisdCfgKey, testNisdUUID, kv.k), kv.v, "")
				}
			},
			setupHR: func() {
				HR.Init()
				HR.AddNisd(&ctlplfl.Nisd{
					ID:            testNisdUUID,
					AvailableSize: 5000000000000,
					FailureDomain: []string{testPDU, testRack, testHV, testDev, testPT},
				})
			},
			vdevID:      testVdevUUID,
			expectError: false,
			replySize:   1024,
			verify: func(t *testing.T, ds storageiface.DataStore) {
				expectedAS := int64(5000000000000) + (100 * ctlplfl.CHUNK_SIZE)
				res, err := ds.Read(fmt.Sprintf("%s/%s/as", NisdCfgKey, testNisdUUID), "")
				if err != nil || string(res) != strconv.FormatInt(expectedAS, 10) {
					t.Errorf("Expected NISD AS %d, got %s", expectedAS, string(res))
				}
				// Verify chunks are deleted
				if _, err := ds.Read(fmt.Sprintf("v/%s/c/50/R.0", testVdevUUID), ""); err == nil {
					t.Error("Expected chunks to be deleted, but found them")
				}
			},
		},
		{
			name:          "Error_MissingToken",
			setupData:     func(ds storageiface.DataStore) { setupBasicVdevAndNisd(ds, testVdevUUID, testNisdUUID) },
			setupHR:       func() { setupHRWithNisd(testNisdUUID, 1073741824) },
			vdevID:        testVdevUUID,
			token:         "",
			expectError:   true,
			errorContains: "user token is required",
		},
		{
			name:      "Error_UnauthorizedRole",
			setupData: func(ds storageiface.DataStore) { setupBasicVdevAndNisd(ds, testVdevUUID, testNisdUUID) },
			setupHR:   func() { setupHRWithNisd(testNisdUUID, 1073741824) },
			vdevID:    testVdevUUID,
			token: func() string {
				tk, _ := createTestToken(testUserID1, "viewer", testSecret)
				return tk
			}(),
			expectError:   true,
			errorContains: "authorization failed",
		},
		{
			name:          "Error_EmptyVdevID",
			setupData:     func(_ storageiface.DataStore) {},
			setupHR:       func() { HR.Init() },
			vdevID:        "",
			expectError:   true,
			errorContains: "invalid ID",
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

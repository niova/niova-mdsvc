package userserver

import (
	"crypto/subtle"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	auth "github.com/00pauln00/niova-mdsvc/controlplane/auth/jwt"
	authz "github.com/00pauln00/niova-mdsvc/controlplane/authorizer"
	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	srvctlplanefuncs "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/server"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	pmCommon "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	pumiceFunc "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/common"
	"github.com/00pauln00/niova-pumicedb/go/pkg/pumiceserver"
	storageiface "github.com/00pauln00/niova-pumicedb/go/pkg/utils/storage/interface"
)

// Default admin secret key - used for initial admin user creation
// This should be changed after first login via update request
const defaultAdminSecretKey = "niova-admin-secret-key"

// Default token TTL (15 minutes)
const defaultTokenTTL = 15 * time.Minute

// Error message returned by PMDB when key is not found
const errKeyNotFoundMsg = "Failed to lookup for key"

// isKeyNotFoundError checks if the error indicates that the key was not found in PMDB
func isKeyNotFoundError(err error) bool {
	return err != nil && err.Error() == errKeyNotFoundMsg
}

//
// var columnFamily string = "AUTH_CF"

// checkUserExistsByID checks if a user with the given UserID (UUID string) already exists
func checkUserExistsByID(userID string, callbackArgs *pumiceserver.PmdbCbArgs) (*userlib.User, error) {
	if userID == "" {
		return nil, nil
	}

	prefixKey := fmt.Sprintf("%s/%s", userKeyPrefix, userID)
	readResult, err := callbackArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector:   cpLib.PmdbColumnFamily,
		Key:        prefixKey,
		Prefix:     prefixKey,
		BufSize:    callbackArgs.ReplySize,
		SeqNum:     0,
		Consistent: false,
	})

	if isKeyNotFoundError(err) {
		// User doesn't exist
		log.Infof("Key [%v] not found\n", prefixKey)
		return nil, nil
	} else if err != nil {
		// Actual error
		return nil, fmt.Errorf("range read failure: %w", err)
	}

	existing := DeserializeSingleUser(readResult.ResultMap, userID)
	return existing, nil
}

// getUserIDByUsername looks up a UserID by username using the secondary index.
// Returns empty string if username doesn't exist.
func getUserIDByUsername(username string, callbackArgs *pumiceserver.PmdbCbArgs) (string, error) {
	if username == "" {
		return "", nil
	}

	indexKey := fmt.Sprintf("%s/%s/%s", userIndexPrefix, usernameIdxPrefix, username)
	readResult, err := callbackArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector:   cpLib.PmdbColumnFamily,
		Key:        indexKey,
		Prefix:     indexKey,
		BufSize:    callbackArgs.ReplySize,
		SeqNum:     0,
		Consistent: false,
	})

	if isKeyNotFoundError(err) {
		// Username doesn't exist in index
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("index lookup failure: %w", err)
	}

	// Extract the UserID from the result
	for _, v := range readResult.ResultMap {
		if len(v) > 0 {
			return string(v), nil
		}
	}

	return "", nil
}

// checkUsernameExists checks if a username already exists using the secondary index.
// Returns the UserID if exists, empty string otherwise.
func checkUsernameExists(username string, callbackArgs *pumiceserver.PmdbCbArgs) (string, error) {
	return getUserIDByUsername(username, callbackArgs)
}

// handleCreateUser handles user creation logic
// Generates UUID for UserID (if not provided) and returns the plain text secret key for response
func handleCreateUser(req *userlib.UserReq, user *userlib.User) (string, error) {
	// Generate UUID for UserID only if not already provided (e.g., admin initialization)
	if req.UserID == "" {
		newUUID, err := uuid.NewV7()
		if err != nil {
			return "", fmt.Errorf("failed to generate UUID: %w", err)
		}
		req.UserID = newUUID.String()
	}

	// Initialize user from request
	if err := user.Init(req); err != nil {
		return "", fmt.Errorf("user initialization failed: %w", err)
	}

	// Determine the secret key to use
	var plainSecretKey string
	if req.IsAdmin {
		// Use fixed default secret key for admin user
		// This should be changed after first login via update request
		plainSecretKey = defaultAdminSecretKey
	} else {
		// Generate random secret key for regular users
		var err error
		plainSecretKey, err = userlib.GenerateSecretKey()
		if err != nil {
			return "", fmt.Errorf("failed to generate secret key: %w", err)
		}
	}

	// Encrypt the secret key before storing
	encryptedSecretKey, err := userlib.EncryptSecretKey(plainSecretKey)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt secret key: %w", err)
	}

	// Store encrypted version in user (for RocksDB storage)
	user.SecretKey = encryptedSecretKey

	// Return plain text for response
	return plainSecretKey, nil
}

// handleUpdateUser handles user update logic (username/secretKey changes)
func handleUpdateUser(req *userlib.UserReq, existing *userlib.User) (string, error) {
	var updatedSecretKey string

	// Apply username change if provided
	if req.Username != "" && req.Username != existing.Username {
		if existing.IsAdmin {
			return "", fmt.Errorf("admin user cannot change username")
		}
		// Prevent regular users from using the reserved admin username
		if req.Username == userlib.AdminUsername {
			return "", fmt.Errorf("username %q is reserved for admin user", userlib.AdminUsername)
		}
		existing.Username = req.Username
	}

	// Update secret key if provided (admin only)
	if req.NewSecretKey != "" {
		if !existing.IsAdmin {
			return "", fmt.Errorf("only admin users can update their secret key")
		}

		// Encrypt the new secret key before storing
		encryptedSecretKey, err := userlib.EncryptSecretKey(req.NewSecretKey)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt new secret key: %w", err)
		}
		existing.SecretKey = encryptedSecretKey
		updatedSecretKey = req.NewSecretKey // Return plain text for response
	}

	return updatedSecretKey, nil
}

// PutUser creates or updates a user's authentication credentials.
// For create operations, it generates a new UUID and secret key.
// For update operations, it updates username/capabilities but keeps the existing secret key.
// UserID (UUID) is always generated server-side for create operations.
// Only admin users can create new users.
func PutUser(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(cpLib.CPReq)
	req := cpReq.Payload.(userlib.UserReq)
	callbackArgs := args[1].(*pumiceserver.PmdbCbArgs)

	var err error
	var isCreate bool
	var oldUsername string // Track old username for index deletion on update
	user := &userlib.User{}

	tokenClaims, err := srvctlplanefuncs.ValidateToken(cpReq.Token)
	if err != nil {
		return returnError(err.Error())
	}
	callerUserID := tokenClaims.UserID
	callerRole := tokenClaims.Role
	callerIsAdmin := tokenClaims.IsAdmin
	log.Infof("PutUser called by user %s with role %s", callerUserID, callerRole)
	// Check authorization
	auth := srvctlplanefuncs.GetAuthorizer()
	if auth != nil && srvctlplanefuncs.IsAuthEnabled() {
		if !auth.CheckRBAC(authz.PutUser, []string{callerRole}) {
			log.Errorf("User %s with role %s not authorized for PutUser", callerUserID, callerRole)
			return returnError("authorization failed: insufficient permissions")
		}
	}
	if !req.IsUpdate {
		// Only admins can create new users
		if srvctlplanefuncs.IsAuthEnabled() && (!callerIsAdmin || callerRole != userlib.AdminUserRole) {
			log.Errorf("User %s with role %s attempted to create a user - permission denied", callerUserID, callerRole)
			return returnError("permission denied: only admin users can create new users")
		}
		log.Infof("Admin user %s authorized for user creation", callerUserID)
	}

	if req.IsUpdate {
		// UPDATE operation
		if req.UserID == "" {
			log.Error("UserID is required for update operation")
			return returnError("userID is required for update operation")
		}

		// Look up existing user by ID
		existing, err := checkUserExistsByID(req.UserID, callbackArgs)
		if err != nil {
			log.Errorf("Failed to check user existence: %v", err)
			return returnError(fmt.Sprintf("failed to check user existence: %v", err))
		}

		if existing == nil {
			log.Error("Cannot update: user not found: ", req.UserID)
			return returnError(fmt.Sprintf("user not found: %s", req.UserID))
		}

		// Users can only update their own record, unless they are admin
		if srvctlplanefuncs.IsAuthEnabled() && (!callerIsAdmin && callerUserID != req.UserID) {
			log.Errorf("User %s attempted to update user %s - permission denied", callerUserID, req.UserID)
			return returnError("permission denied: you can only update your own record")
		}

		// These fields should remain as they are in the existing record
		req.IsAdmin = existing.IsAdmin

		// Check if username is being changed
		if req.Username != "" && req.Username != existing.Username {
			// Use secondary index to check if new username already exists
			existingUserID, err := checkUsernameExists(req.Username, callbackArgs)
			if err != nil {
				log.Errorf("Failed to check username uniqueness: %v", err)
				return returnError(fmt.Sprintf("failed to check username uniqueness: %v", err))
			}
			if existingUserID != "" {
				log.Errorf("Username '%s' already exists.", req.Username)
				return returnError(fmt.Sprintf("username '%s' already exists", req.Username))
			}
			// Store old username for index deletion
			oldUsername = existing.Username
		}

		isCreate = false
		user = existing
	} else {
		// CREATE operation
		if strings.TrimSpace(req.Username) == "" {
			log.Error("Username cannot be empty for create operation")
			return returnError("username cannot be empty")
		}

		// Prevent creating regular users with the reserved admin username
		if req.Username == userlib.AdminUsername {
			log.Errorf("Username %q is reserved for admin user", userlib.AdminUsername)
			return returnError(fmt.Sprintf("username %q is reserved for admin user", userlib.AdminUsername))
		}

		// Use secondary index to check if username already exists
		existingUserID, err := checkUsernameExists(req.Username, callbackArgs)
		if err != nil {
			log.Errorf("Failed to check username uniqueness: %v", err)
			return returnError(fmt.Sprintf("failed to check username uniqueness: %v", err))
		}
		if existingUserID != "" {
			log.Errorf("Username '%s' already exists.", req.Username)
			return returnError(fmt.Sprintf("username '%s' already exists", req.Username))
		}
		isCreate = true
	}
	// Handle create or update
	var plainSecretKey string
	if isCreate {
		plainSecretKey, err = handleCreateUser(&req, user)
		if err != nil {
			log.Error("Create operation failed: ", err)
			return returnError(err.Error())
		}
	} else {
		plainSecretKey, err = handleUpdateUser(&req, user)
		if err != nil {
			log.Error("Update operation failed: ", err)
			return returnError(err.Error())
		}
	}

	// Serialize to commit changes (user.SecretKey is encrypted)
	commitChanges := SerializeUser(user)

	// If username was changed, delete the old username index
	if oldUsername != "" {
		commitChanges = append(commitChanges, SerializeUsernameIndexDelete(oldUsername))
	}

	// Prepare response (return plain text secret key if created or updated)
	authResponse := &userlib.UserResp{
		UserID:   user.UserID,
		Username: user.Username,
		UserRole: user.UserRole,
		Status:   user.Status,
		Success:  true,
	}
	if plainSecretKey != "" {
		// Return plain text secret key in response (for create or secret key update)
		authResponse.SecretKey = plainSecretKey
	}

	funcIntermediate := pumiceFunc.FuncIntrm{
		Changes:  commitChanges,
		Response: authResponse,
	}

	encodedIntermediate, err := pmCommon.Encoder(pmCommon.GOB, funcIntermediate)
	if err != nil {
		log.Error("Failed to encode intermediate: ", err)
		return returnError(fmt.Sprintf("failed to encode intermediate: %v", err))
	}

	return encodedIntermediate, nil
}

// GetUser retrieves user authentication information from RocksDB.
// It supports querying by UserID (UUID string), Username, or getting all users.
// Users can only view their own information unless they are admin.
func GetUser(args ...interface{}) (interface{}, error) {
	callbackArgs := args[0].(*pumiceserver.PmdbCbArgs)
	cpReq := args[1].(cpLib.CPReq)
	req := cpReq.Payload.(userlib.GetReq)

	tokenClaims, err := srvctlplanefuncs.ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("GetUser: token validation failed: %v", err)
		return cpLib.AuthError(err)
	}
	callerUserID := tokenClaims.UserID
	callerRole := tokenClaims.Role
	callerIsAdmin := tokenClaims.IsAdmin
	log.Infof("GetUser called by user %s with role %s", callerUserID, callerRole)

	// Check authorization
	auth := srvctlplanefuncs.GetAuthorizer()
	if auth != nil && srvctlplanefuncs.IsAuthEnabled() {
		if !auth.CheckRBAC(authz.GetUser, []string{callerRole}) {
			log.Errorf("User %s with role %s not authorized for GetUser", callerUserID, callerRole)
			return cpLib.AuthError(fmt.Errorf("authorization failed: insufficient permissions"))
		}
	}

	// Build the key for range read
	prefixKey := userKeyPrefix
	var targetUserID string

	// UserID takes precedence over Username
	if req.UserID != "" {
		// Query specific user by ID
		targetUserID = req.UserID
		prefixKey = fmt.Sprintf("%s/%s", userKeyPrefix, req.UserID)
	} else if req.Username != "" {
		userID, err := getUserIDByUsername(req.Username, callbackArgs)
		if err != nil {
			log.Errorf("Failed to lookup user by username: %v", err)
			return cpLib.FuncError(fmt.Errorf("failed to lookup user by username: %v", err))
		}
		if userID == "" {
			// Username not found - return empty result
			log.Infof("Username [%v] not found in index\n", req.Username)
			return cpLib.EncodeResponse([]userlib.UserResp{})
		}
		// Use the resolved UserID for lookup
		targetUserID = userID
		prefixKey = fmt.Sprintf("%s/%s", userKeyPrefix, userID)
	}

	// Regular users can only view their own information
	// Admins can view any user information (including listing all users)
	if srvctlplanefuncs.IsAuthEnabled() && (!callerIsAdmin && targetUserID != "" && targetUserID != callerUserID) {
		log.Errorf("User %s attempted to view user %s - permission denied", callerUserID, targetUserID)
		return cpLib.AuthError(fmt.Errorf("authorization failed: you can only view your own information"))
	}

	// If querying all users (no specific ID/username), only admin is allowed
	if srvctlplanefuncs.IsAuthEnabled() && (!callerIsAdmin && req.UserID == "" && req.Username == "") {
		log.Errorf("Non-admin user %s attempted to list all users - permission denied", callerUserID)
		return cpLib.AuthError(fmt.Errorf("authorization failed: only admins can list all users"))
	}

	readResult, err := callbackArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector:   cpLib.PmdbColumnFamily,
		Key:        prefixKey,
		Prefix:     prefixKey,
		BufSize:    callbackArgs.ReplySize,
		SeqNum:     0,
		Consistent: false,
	})

	var users []userlib.User
	if err != nil {
		if isKeyNotFoundError(err) {
			// Key/prefix not found treat as empty result (no users)
			log.Infof("Key [%v] not found\n", prefixKey)
			users = []userlib.User{}
		} else {
			log.Error("Range read failure: ", err.Error())
			return cpLib.FuncError(fmt.Errorf("range read failure: %v", err))
		}
	} else {
		// Deserialize entities
		users = DeserializeUsers(readResult.ResultMap)
	}

	// Convert to UserResp slice with decrypted secret keys
	userResps := make([]userlib.UserResp, 0, len(users))
	for _, u := range users {
		// Decrypt the secret key before returning
		decryptedKey := ""
		if u.SecretKey != "" {
			decrypted, decryptErr := userlib.DecryptSecretKey(u.SecretKey)
			if decryptErr != nil {
				log.Error("Failed to decrypt secret key for user ", u.UserID, ": ", decryptErr)
				// Continue with empty key rather than failing the entire request
			} else {
				decryptedKey = decrypted
			}
		}

		userResps = append(userResps, userlib.UserResp{
			Username:  u.Username,
			UserID:    u.UserID,
			SecretKey: decryptedKey,
			UserRole:  u.UserRole,
			Status:    u.Status,
			Success:   true,
		})
	}

	log.Debugf("GetUser: returning %d user(s)", len(userResps))
	return cpLib.EncodeResponse(userResps)
}

// applyCommitChanges applies commit changes to RocksDB.
// If Value is empty, the key is deleted using PmdbDeleteKV.
// Otherwise, the key-value is written using PmdbWriteKV.
func applyCommitChanges(changes []pumiceFunc.CommitChg, cbArgs *pumiceserver.PmdbCbArgs) error {
	for _, chg := range changes {
		var err error
		switch chg.Op {
		case pumiceFunc.OpApply:
			log.Debugf("Applying change: %s", string(chg.Key))
			err = cbArgs.Store.Write(string(chg.Key), string(chg.Value), cpLib.PmdbColumnFamily)
		case pumiceFunc.OpDelete:
			log.Debugf("Deleting key: %s", string(chg.Key))
			err = cbArgs.Store.Delete(string(chg.Key), cpLib.PmdbColumnFamily)
		default:
			log.Error("Unknown operation type: ", chg.Op, " for key: ", string(chg.Key))
			return fmt.Errorf("unknown operation type %d for key %s", chg.Op, string(chg.Key))
		}
		if err != nil {
			log.Errorf("Failed to apply change for key: %s", string(chg.Key))
			return err
		}
	}
	return nil
}

// CreateAdminUser creates/bootstraps the singleton admin user.
func CreateAdminUser(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("missing arguments: expected UserReq and PmdbCbArgs")
	}

	cpreq, ok1 := args[0].(cpLib.CPReq)
	cbArgs, ok2 := args[1].(*pumiceserver.PmdbCbArgs)
	req, ok3 := cpreq.Payload.(userlib.UserReq)
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("type assertion failed: invalid argument types")
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		log.Error("CreateAdminUser: username cannot be empty")
		return cpLib.FuncError(fmt.Errorf("username cannot be empty"))
	}
	if username != userlib.AdminUsername {
		log.Errorf("CreateAdminUser: unauthorized username %q", username)
		return cpLib.FuncError(fmt.Errorf("unauthorized: username must be %q", userlib.AdminUsername))
	}

	adminID := uuid.Nil.String()
	secretKey := req.NewSecretKey
	if secretKey == "" {
		secretKey = defaultAdminSecretKey
	}

	// Check for Existence
	existing, err := checkUserExistsByID(adminID, cbArgs)
	if err != nil {
		return nil, fmt.Errorf("datastore lookup failed: %w", err)
	}

	if existing != nil {
		log.Infof("Admin user already exists (ID: %s). Returning existing info.", adminID)

		decryptedKey, err := userlib.DecryptSecretKey(existing.SecretKey)
		if err != nil {
			log.Warnf("Could not decrypt existing admin key: %v", err)
		}

		return cpLib.EncodeResponse(userlib.UserResp{
			UserID:    existing.UserID,
			Username:  existing.Username,
			SecretKey: decryptedKey,
			UserRole:  existing.UserRole,
			Status:    existing.Status,
			Success:   true,
			Error:     "admin user already exists",
		})
	}

	// Creation & Persistence
	encryptedSecretKey, err := userlib.EncryptSecretKey(secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt secret key: %w", err)
	}

	newUser := &userlib.User{
		UserID:    adminID,
		Username:  username,
		SecretKey: encryptedSecretKey,
		UserRole:  userlib.AdminUserRole,
		IsAdmin:   true,
		Status:    userlib.StatusActive,
	}

	if err := applyCommitChanges(SerializeUser(newUser), cbArgs); err != nil {
		return nil, fmt.Errorf("failed to persist admin user: %w", err)
	}

	log.Infof("Admin user created successfully (ID: %s)", adminID)
	return cpLib.EncodeResponse(userlib.UserResp{
		UserID:    newUser.UserID,
		Username:  newUser.Username,
		SecretKey: secretKey,
		UserRole:  newUser.UserRole,
		Status:    newUser.Status,
		Success:   true,
	})
}

// Login authenticates a user and returns a JWT token
func Login(args ...interface{}) (interface{}, error) {
	callbackArgs := args[0].(*pumiceserver.PmdbCbArgs)
	cpreq := args[1].(cpLib.CPReq)
	req := cpreq.Payload.(cpLib.GetReq)

	var username, secretKey string
	// Expected ID as username:secretKey
	sreq := strings.Split(req.ID, ":")
	if len(sreq) != 2 {
		log.Error("Login: invalid request format: expected username:secretKey")
		return cpLib.FuncError(fmt.Errorf("username:secretKey missing"))
	} else {
		username, secretKey = sreq[0], sreq[1]
		if strings.TrimSpace(username) == "" {
			log.Error("Login: username cannot be empty")
			return cpLib.FuncError(fmt.Errorf("username cannot be empty"))
		}
		if strings.TrimSpace(secretKey) == "" {
			log.Error("Login: secretKey cannot be empty")
			return cpLib.FuncError(fmt.Errorf("secretKey cannot be empty"))
		}
	}

	// Look up user by username
	userID, err := getUserIDByUsername(username, callbackArgs)
	if err != nil {
		log.Errorf("failed to lookup user by username '%s': %v", username, err)
		return cpLib.FuncError(fmt.Errorf("invalid credentials"))
	}
	if userID == "" {
		log.Errorf("username not found: %s", username)
		return cpLib.FuncError(fmt.Errorf("invalid credentials"))
	}

	// Get full user data
	user, err := checkUserExistsByID(userID, callbackArgs)
	if err != nil {
		log.Errorf("failed to get user data for userID '%s': %v", userID, err)
		return cpLib.FuncError(fmt.Errorf("invalid credentials"))
	}
	if user == nil {
		log.Errorf("user not found by ID '%s'", userID)
		return cpLib.FuncError(fmt.Errorf("invalid credentials"))
	}

	// Check user status
	if user.Status != userlib.StatusActive {
		log.Errorf("login attempt for inactive account - username: %s, userID: %s, status: %s",
			user.Username, user.UserID, user.Status.String())
		return cpLib.FuncError(fmt.Errorf("account not available"))
	}

	// Decrypt stored secret key
	storedKey, err := userlib.DecryptSecretKey(user.SecretKey)
	if err != nil {
		log.Errorf("failed to decrypt stored secret key for user '%s' (ID: %s): %v",
			user.Username, user.UserID, err)
		return cpLib.InternalError(fmt.Errorf("internal error"))
	}

	// Compare provided secret key with stored key using constant time comparison
	if subtle.ConstantTimeCompare([]byte(storedKey), []byte(secretKey)) != 1 {
		log.Errorf("invalid secret key provided for user '%s' (ID: %s)",
			user.Username, user.UserID)
		return cpLib.FuncError(fmt.Errorf("invalid credentials"))
	}

	tokenTTL := defaultTokenTTL
	tc := &auth.Token{
		Secret: []byte(cpLib.CP_SECRET),
		TTL:    tokenTTL,
	}

	customClaims := map[string]any{
		"userID":   user.UserID,
		"username": user.Username,
		"role":     user.UserRole,
		"isAdmin":  user.IsAdmin,
	}

	accessToken, err := tc.CreateToken(customClaims)
	if err != nil {
		log.Errorf("failed to create JWT token for user '%s' (ID: %s): %v",
			user.Username, user.UserID, err)
		return cpLib.InternalError(fmt.Errorf("failed to generate token"))
	}

	// Build response
	resp := userlib.LoginResp{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(tokenTTL.Seconds()),
		UserID:      user.UserID,
		Username:    user.Username,
		UserRole:    user.UserRole,
		IsAdmin:     user.IsAdmin,
		Success:     true,
	}

	log.Infof("user '%s' (ID: %s) authenticated successfully", user.Username, user.UserID)
	return cpLib.EncodeResponse(resp)
}

func returnError(errMsg string) (interface{}, error) {
	errorResponse := &userlib.UserResp{
		Error: errMsg,
	}

	funcIntermediate := pumiceFunc.FuncIntrm{
		Changes:  nil,
		Response: errorResponse,
	}

	encodedIntermediate, err := pmCommon.Encoder(pmCommon.GOB, funcIntermediate)
	if err != nil {
		log.Error("Failed to encode error intermediate: ", err)
		return pmCommon.Encoder(pmCommon.GOB, cpLib.CPResp{
			Error: &cpLib.CPError{Code: cpLib.ErrFunc, Message: err.Error()},
		})
	}

	return encodedIntermediate, nil
}

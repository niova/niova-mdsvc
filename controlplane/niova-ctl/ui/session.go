package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	log "github.com/sirupsen/logrus"

	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain/mdsvcpumiceclient"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/pages"
)

func (a *App) IsLoggedIn() bool {
	return a.Auth.User != nil
}

// EnsureTokenValid checks if the login token TTL is close to expiration and
// automatically re-authenticates — a real network call when it fires. Only
// call this from inside an already-async tea.Cmd (as UserToken() below
// does); calling it synchronously from Update() blocks the whole UI on
// whatever that request takes.
func (a *App) EnsureTokenValid() {
	if a.Auth.User == nil || a.Auth.SecretKey == "" || a.UserClient == nil {
		return
	}

	expiresIn := a.Auth.User.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600 // Default 1 hour fallback
	}

	// Auto-refresh when 75% of token TTL has elapsed
	elapsed := time.Since(a.Auth.LoginTime).Seconds()
	threshold := float64(expiresIn) * 0.75

	if elapsed >= threshold {
		log.Infof("Auto-refreshing login token for user %s (elapsed %.0fs / %ds TTL)", a.Auth.Username, elapsed, expiresIn)
		resp, err := a.UserClient.LoginWithTenant(a.Auth.Username, a.Auth.SecretKey, a.Auth.TenantUUID)
		if err == nil && resp != nil && resp.Success {
			a.Auth.User = resp
			a.Auth.LoginTime = time.Now()
			if a.CPClient != nil {
				a.CPClient.SetToken(resp.AccessToken)
			}
			log.Infof("Token auto-refreshed successfully for user %s", a.Auth.Username)
		} else {
			log.Errorf("Token auto-refresh failed for user %s: %v", a.Auth.Username, err)
		}
	}
}

func (a *App) UserToken() string {
	a.EnsureTokenValid()
	if a.Auth.User != nil {
		return a.Auth.User.AccessToken
	}
	return ""
}

// IsFormState reports whether the active page is a form/input screen rather
// than a resource table view — used to suppress tab-switching hotkeys and
// route Esc "cancel" appropriately (update.go). Derived from ActivePage's
// own type (pages.FormPage) so it can't drift out of sync with the screen.
func (a *App) IsFormState() bool {
	_, ok := a.ActivePage.(pages.FormPage)
	return ok
}

func (a *App) EnsureUserClient() error {
	if a.UserClient != nil {
		return nil
	}
	if a.Backend == domain.BackendMdsvcTidb {
		// Constructed eagerly in NewApp for this backend (shares the same
		// underlying HTTP client/token as CPClient) — nil here means the
		// -mdsvc-url flag was never set up correctly.
		return fmt.Errorf("mdsvc-tidb client not initialized")
	}
	client, teardown := mdsvcpumiceclient.InitUserClient(a.RaftUUID, a.GossipPath, a.LogFile)
	if client == nil {
		return fmt.Errorf("failed to initialize user client (missing raft UUID or gossip path)")
	}
	a.UserClient = mdsvcpumiceclient.NewUserClient(client)
	a.TeardownFn = teardown
	return nil
}

// usersLoadedMsg reports the result of loadUsersView's async fetch.
type usersLoadedMsg struct {
	Users []userlib.UserResp
	Err   error
}

// loadUsersView switches to the Users tab and returns the tea.Cmd that
// fetches them — like refreshCPDataCmd, the actual ListUsers call happens
// off the Update() thread, never here, so opening this tab never blocks
// the UI on a slow or unreachable server.
func (a *App) loadUsersView() tea.Cmd {
	a.ActiveTab = 0
	if !a.isAdmin() {
		a.StatusMsg = "Error: admin role required to view user accounts"
		a.StatusType = components.StatusError
		return nil
	}
	if err := a.EnsureUserClient(); err != nil {
		a.StatusMsg = err.Error()
		a.StatusType = components.StatusError
		return nil
	}
	return func() tea.Msg {
		users, err := a.UserClient.ListUsers(a.UserToken(), userlib.GetReq{})
		return usersLoadedMsg{Users: users, Err: err}
	}
}

// tenantsLoadedMsg reports the result of loadTenantsView's async fetch.
type tenantsLoadedMsg struct {
	Tenants []domain.TenantInfo
	Err     error
}

// loadTenantsView switches to the Tenants tab and returns the tea.Cmd that
// fetches the tenant registry using the tenant-admin session (mdsvc-tidb
// only — a separate identity from the regular user login). See
// loadUsersView's doc for why the fetch itself lives in the returned Cmd.
func (a *App) loadTenantsView() tea.Cmd {
	a.ActiveTab = 10
	tm, ok := a.CPClient.(domain.TenantManager)
	if !ok {
		a.StatusMsg = "Tenant management is only available against the mdsvc-tidb backend"
		a.StatusType = components.StatusError
		return nil
	}
	if a.TenantAdmin.Token == "" {
		a.StatusMsg = "Not logged in as tenant-admin — run :tenant-login first"
		a.StatusType = components.StatusError
		return nil
	}
	token := a.TenantAdmin.Token
	return func() tea.Msg {
		tenants, err := tm.ListTenants(token)
		return tenantsLoadedMsg{Tenants: tenants, Err: err}
	}
}

// authzLoadedMsg reports the result of loadAuthzView's async fetch.
type authzLoadedMsg struct {
	RBAC []domain.RBACPolicy
	ABAC []domain.ABACPolicy
	Err  error
}

// loadAuthzView returns the tea.Cmd that fetches RBAC and ABAC policies
// (mdsvc-tidb only, admin token) — see loadUsersView's doc for why. Unlike
// loadUsersView/loadTenantsView, ActiveTab isn't set here: it's only set to
// 11 on success (authzLoadedMsg handler), so a blocked/failed attempt never
// leaves the sidebar highlighting Authz over the previous tab's content.
func (a *App) loadAuthzView() tea.Cmd {
	if !a.isAdmin() {
		a.StatusMsg = "Error: admin role required to manage authorization policies"
		a.StatusType = components.StatusError
		return nil
	}
	am, ok := a.CPClient.(domain.AuthzManager)
	if !ok {
		a.StatusMsg = "Authorization policy management is only available against the mdsvc-tidb backend"
		a.StatusType = components.StatusError
		return nil
	}
	a.CPClient.SetToken(a.UserToken())
	return func() tea.Msg {
		rbac, err := am.ListRBACPolicies()
		if err != nil {
			return authzLoadedMsg{Err: fmt.Errorf("failed to list RBAC policies: %w", err)}
		}
		abac, err := am.ListABACPolicies()
		if err != nil {
			return authzLoadedMsg{Err: fmt.Errorf("failed to list ABAC policies: %w", err)}
		}
		return authzLoadedMsg{RBAC: rbac, ABAC: abac}
	}
}

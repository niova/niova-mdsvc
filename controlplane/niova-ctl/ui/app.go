// Package ui implements niova-ctl's Bubble Tea TUI. App is the root Model;
// this file holds its struct/construction, with Update/View/policy/session/
// navigation/form-submit logic split into their own like-named files.
package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/sirupsen/logrus"

	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain/mdsvcpumiceclient"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain/mdsvctidbclient"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/pages"
)

// App is the root Bubble Tea model orchestrator
type App struct {
	ActivePage pages.Page
	ActiveTab  int // 1=PDUs, 2=Racks, 3=Hypervisors, 4=Disks, 5=Partitions, 6=NISDs, 7=Vdevs, 8=PFS, 9=Topology

	// Confirmation Dialog Modal Overlay
	ConfirmDialog components.ConfirmDialogModel

	// Command Bar & Cheat Sheet Overlays
	CommandBar components.CommandBarModel
	HelpModal  components.HelpModalModel

	// Infrastructure Configuration
	CPEnabled   bool
	RaftUUID    string
	GossipPath  string
	LogFile     string
	MdsvcURL    string
	Backend     domain.BackendKind
	CPConnected bool
	AuthEnabled bool
	CPClient    domain.ControlPlaneClient
	UserClient  domain.UserServiceClient
	TeardownFn  func()

	// Auth is the regular Admin/User login identity — zero until a
	// UserLoggedInMsg, reset by clearAuthSession() (see TenantAdmin below).
	Auth authSession
	// TenantAdmin is a separate tenant-admin session (mdsvc-tidb only, via
	// :tenant-login) — a distinct identity from Auth, never mixed with it:
	// each login resets the other via clearAuthSession/clearTenantAdminSession.
	TenantAdmin tenantAdminSession

	// Control Plane Topology (cached — refreshed asynchronously via
	// refreshCPDataCmd, never fetched from View() or synchronously in Update)
	PDUs          []domain.PDU
	Racks         []domain.Rack
	StandaloneHVs []domain.Hypervisor
	NISDs         []domain.Nisd
	Vdevs         []domain.VdevConfig
	PFSs          []domain.PFS

	// Metrics is recomputed from the cached fields above whenever they
	// change (see cpDataRefreshedMsg handling) — View() must never compute
	// it directly, since that would mean network I/O on every keypress.
	Metrics components.ClusterMetrics

	// Terminal Dimensions
	TermWidth  int
	TermHeight int

	// Status Message
	StatusMsg  string
	StatusType components.StatusType
	Quitting   bool

	// Status bar auto-dismiss bookkeeping (see status_ticker.go) — internal,
	// never set directly outside the statusTickMsg handler in Update().
	lastStatusMsg string
	statusSetAt   time.Time

	// Background Spinner
	Spinner spinner.Model
	Loading bool
}

// authSession is the regular Admin/User login identity. Grouped into one
// struct so a fresh login (UserLoggedInMsg) or logout can set/clear every
// field in one assignment (see clearAuthSession) instead of a hand-maintained
// list that's easy to under-reset as fields are added.
type authSession struct {
	User       *userlib.LoginResp
	Username   string
	SecretKey  string
	TenantUUID string
	LoginTime  time.Time
}

// clearAuthSession resets the regular login identity to zero — used when a
// tenant-admin login replaces it (the two are mutually exclusive sessions).
func (a *App) clearAuthSession() { a.Auth = authSession{} }

// tenantAdminSession is the separate tenant-admin identity (mdsvc-tidb only,
// via :tenant-login) — Username is shown in the header as Role
// "tenant-admin" only when there's no regular Auth.User session, which
// already has its own identity/role taking precedence there.
type tenantAdminSession struct {
	Token    string
	Username string
}

// clearTenantAdminSession resets the tenant-admin identity to zero — used
// when a regular login replaces it.
func (a *App) clearTenantAdminSession() { a.TenantAdmin = tenantAdminSession{} }

func NewApp(cpEnabled bool, raftUUID, gossipPath, logFile, mdsvcURL string) *App {
	if logFile == "" {
		logFile = "/tmp/niova-ctl.log"
	}

	var (
		cpClient     domain.ControlPlaneClient
		userClient   domain.UserServiceClient
		isConnected  bool
		authDetected bool
		backend      = domain.BackendMdsvcPumice
	)

	if mdsvcURL != "" {
		backend = domain.BackendMdsvcTidb
		mc := mdsvctidbclient.New(mdsvcURL)
		cpClient = mc
		userClient = mc
		isConnected = true
		if enabled, err := mdsvctidbclient.ProbeAuth(mdsvcURL); err == nil {
			authDetected = enabled
		} else {
			log.Warn("mdsvc-tidb auth probe failed: ", err)
		}
	} else if cpEnabled {
		pumiceClient := mdsvcpumiceclient.InitControlPlane(raftUUID, gossipPath, logFile)
		if pumiceClient != nil {
			isConnected = true
			cpClient = mdsvcpumiceclient.NewControlPlaneClient(pumiceClient)
			authDetected = mdsvcpumiceclient.ProbeServerAuth(pumiceClient)
		}
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4")).Bold(true)

	app := &App{
		CPEnabled:   cpEnabled,
		RaftUUID:    raftUUID,
		GossipPath:  gossipPath,
		LogFile:     logFile,
		MdsvcURL:    mdsvcURL,
		Backend:     backend,
		CPConnected: isConnected,
		AuthEnabled: authDetected,
		CPClient:    cpClient,
		UserClient:  userClient,
		ActiveTab:   1,
		CommandBar:  components.NewCommandBar(),
		HelpModal:   components.NewHelpModal(),
		TermWidth:   100,
		TermHeight:  30,
		StatusMsg:   "",
		StatusType:  components.StatusInfo,
		Spinner:     sp,
		Loading:     false,
	}

	if authDetected {
		// Both backends land here — niova-mdsvc has no default admin at
		// all, so "Create Admin User" needs to be as visible as "Login"
		// from the very first screen, not a keybinding buried in the login
		// form (see LoginChoicePage's doc comment).
		app.ActivePage = pages.NewLoginChoicePage(backend)
	} else {
		app.ActivePage = pages.NewPDUViewPage(app.PDUs, app.TermWidth, app.TermHeight, app.isAdmin())
	}
	return app
}

func (a *App) Init() tea.Cmd {
	// When the server has auth enabled, there's no token yet at startup —
	// fetching topology data now would just hit the same auth-gated
	// endpoints and surface a spurious "missing authorization header"
	// error. UserLoggedInMsg triggers the real refresh once a token exists.
	if a.AuthEnabled && !a.IsLoggedIn() {
		return statusTickCmd()
	}
	a.Loading = true
	return tea.Batch(a.Spinner.Tick, a.refreshCPDataCmd(), statusTickCmd())
}

// AllHypervisors returns every known hypervisor — a.StandaloneHVs plus every
// hypervisor nested under a.PDUs[*].Racks[*].Hypervisors. Any picker/list
// meaning "every hypervisor in the system" must use this, or rack-attached
// hypervisors silently go missing from it.
func (a *App) AllHypervisors() []domain.Hypervisor {
	hvs := append([]domain.Hypervisor{}, a.StandaloneHVs...)
	for _, p := range a.PDUs {
		for _, r := range p.Racks {
			hvs = append(hvs, r.Hypervisors...)
		}
	}
	return hvs
}

// CalculateMetrics derives cluster counts purely from cached state — it must
// never touch CPClient. It used to hit the network on every call (including
// from View(), i.e. on every keypress); callers now recompute a.Metrics only
// when the underlying cached data actually changes (see cpDataRefreshedMsg).
func (a *App) CalculateMetrics() components.ClusterMetrics {
	allHVs := a.AllHypervisors()
	m := components.ClusterMetrics{
		PDUCount:        len(a.PDUs),
		RackCount:       len(a.Racks),
		HypervisorCount: len(allHVs),
		NISDCount:       len(a.NISDs),
		VdevCount:       len(a.Vdevs),
		PFSCount:        len(a.PFSs),
	}

	devCount := 0
	partCount := 0
	for _, hv := range allHVs {
		devCount += len(hv.Dev)
		for _, dev := range hv.Dev {
			partCount += len(dev.Partitions)
		}
	}

	m.DeviceCount = devCount
	m.PartitionCount = partCount

	return m
}

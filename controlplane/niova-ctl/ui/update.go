package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	log "github.com/sirupsen/logrus"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/pages"
)

// Internal async result messages
type vdevDeletedMsg struct{ Err error }

// cpDataRefreshedMsg reports the result of refreshCPDataCmd. It is the only
// place a.PDUs/Racks/StandaloneHVs/NISDs/Vdevs/PFSs and a.Metrics are ever
// mutated, keeping cache updates and metric recomputation in lockstep.
type cpDataRefreshedMsg struct {
	PDUs  []domain.PDU
	Racks []domain.Rack
	HVs   []domain.Hypervisor
	Nisds []domain.Nisd
	Vdevs []domain.VdevConfig
	PFSs  []domain.PFS
	Err   error
}

// refreshCPDataCmd fetches the full topology plus NISDs/Vdevs/PFS in the
// background and reports back via cpDataRefreshedMsg. This is the only place
// that talks to CPClient for this cached data — it must always be returned
// as a tea.Cmd, never called inline from Update, or it blocks the UI thread.
func (a *App) refreshCPDataCmd() tea.Cmd {
	return func() tea.Msg {
		if !a.canRefreshCPData() {
			return cpDataRefreshedMsg{}
		}
		token := a.UserToken()
		pdus, racks, hvs, err := a.CPClient.RefreshTopology(token)
		if err != nil {
			log.Error("refreshCPDataCmd: RefreshTopology failed: ", err)
			return cpDataRefreshedMsg{Err: err}
		}
		a.CPClient.SetToken(token)
		nisds, err := a.CPClient.GetNisds()
		if err != nil {
			log.Warn("refreshCPDataCmd: GetNisds failed: ", err)
		}
		vdevs, err := a.CPClient.GetVdevConfigs()
		if err != nil {
			log.Warn("refreshCPDataCmd: GetVdevConfigs failed: ", err)
		}
		pfss, err := a.CPClient.GetPFS()
		if err != nil {
			log.Warn("refreshCPDataCmd: GetPFS failed: ", err)
		}
		return cpDataRefreshedMsg{PDUs: pdus, Racks: racks, HVs: hvs, Nisds: nisds, Vdevs: vdevs, PFSs: pfss}
	}
}

// inFlightLabel is the status message shown (with the spinner) while a
// confirmed ConfirmDialog action's async command runs — keyed by its
// ActionID, covering every case ConfirmResultMsg below dispatches.
var inFlightLabel = map[string]string{
	"save_pdu":                    "Saving PDU...",
	"save_rack":                   "Saving Rack...",
	"save_hypervisor":             "Saving Hypervisor...",
	"save_device":                 "Initializing Device...",
	"save_discovered_devices":     "Initializing discovered device(s)...",
	"partition_disk":              "Partitioning disk over SSH — this can take a few seconds...",
	"save_partition":              "Saving Partition...",
	"save_discovered_partitions":  "Registering discovered partition(s)...",
	"save_whole_device_partition": "Registering whole-device partition...",
	"save_nisd":                   "Initializing NISD...",
	"save_nisd_batch":             "Initializing NISD(s)...",
	"save_vdev":                   "Creating Vdev pool...",
	"save_pfs":                    "Saving PFS...",
	"confirm_import_infra":        "Importing infrastructure...",
	"delete_tenant":               "Deleting tenant...",
	"delete_authz_policy":         "Deleting policy...",
	"delete_vdev":                 "Deleting Vdev pool...",
}

// startInFlight puts up the spinner and an in-progress status for an async
// action that just began, batched with a.Spinner.Tick so it's animating by
// the time this Update() call returns — the same treatment Ctrl+R already
// gives refreshCPDataCmd, extended to every confirmed save/delete so none of
// them goes silent while running (see partition_disk's real SSH latency).
func (a *App) startInFlight(statusMsg string, cmd tea.Cmd) tea.Cmd {
	a.StatusMsg = statusMsg
	a.StatusType = components.StatusInfo
	a.Loading = true
	return tea.Batch(a.Spinner.Tick, cmd)
}

func (a *App) handleCommandSubmit(cmdStr string) tea.Cmd {
	cmdLower := strings.ToLower(strings.TrimSpace(cmdStr))

	if a.AuthEnabled && !a.IsLoggedIn() && cmdLower != "login" && cmdLower != "help" &&
		cmdLower != "quit" && cmdLower != "q" && cmdLower != "exit" && cmdLower != "tenant-login" {
		a.StatusMsg = "Error: Authentication required to execute command"
		a.StatusType = components.StatusError
		return nil
	}

	switch cmdLower {
	case "quit", "q", "exit":
		a.Quitting = true
		if a.TeardownFn != nil {
			a.TeardownFn()
		}
		return tea.Quit
	case "pdu", "pdus", "1":
		return a.SwitchToTab(1)
	case "rack", "racks", "2":
		return a.SwitchToTab(2)
	case "hv", "hvs", "hypervisor", "hypervisors", "3":
		return a.SwitchToTab(3)
	case "dev", "devs", "disk", "disks", "device", "devices", "4":
		return a.SwitchToTab(4)
	case "part", "parts", "partition", "partitions", "5":
		return a.SwitchToTab(5)
	case "nisd", "nisds", "6":
		return a.SwitchToTab(6)
	case "vdev", "vdevs", "7":
		return a.SwitchToTab(7)
	case "pfs", "8":
		return a.SwitchToTab(8)
	case "tree", "topo", "topology", "9":
		return a.SwitchToTab(9)
	case "login":
		a.ActivePage = pages.NewLoginChoicePage(a.Backend)
		a.StatusMsg = ""
	case "users", "0":
		return a.loadUsersView()
	case "tenant-login":
		if !a.hasTenantAdminAccess() {
			a.StatusMsg = "Tenant management is only available against the mdsvc-tidb backend"
			a.StatusType = components.StatusError
			return nil
		}
		a.ActivePage = pages.NewTenantAdminLoginPage()
		a.StatusMsg = ""
	case "tenants":
		return a.loadTenantsView()
	case "authz", "rbac", "abac":
		return a.loadAuthzView()
	case "help", "?":
		a.HelpModal.Active = true
	default:
		a.StatusMsg = fmt.Sprintf("Unknown command \":%s\". Press '?' for available commands.", cmdStr)
		a.StatusType = components.StatusError
	}
	return nil
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Resize and spinner ticks must be handled before modal routing below —
	// otherwise resizing the terminal while a dialog/command bar/help modal
	// is open leaves the layout stuck until the next tab switch.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.TermWidth = msg.Width
		a.TermHeight = msg.Height
		a.ResizeActivePageTable()

	case spinner.TickMsg:
		if a.Loading {
			var cmd tea.Cmd
			a.Spinner, cmd = a.Spinner.Update(msg)
			return a, cmd
		}
		return a, nil

	case statusTickMsg:
		if a.StatusMsg != a.lastStatusMsg {
			a.lastStatusMsg = a.StatusMsg
			a.statusSetAt = time.Now()
		} else if a.StatusMsg != "" && time.Since(a.statusSetAt) >= statusDisplayDuration {
			a.StatusMsg = ""
			a.lastStatusMsg = ""
		}
		return a, statusTickCmd()
	}

	// 1. Handle ConfirmDialog active
	if a.ConfirmDialog.Active {
		var cmd tea.Cmd
		a.ConfirmDialog, cmd = a.ConfirmDialog.Update(msg)
		return a, cmd
	}

	// 2. Handle CommandBar active
	if a.CommandBar.Active {
		var cmd tea.Cmd
		a.CommandBar, cmd = a.CommandBar.Update(msg)
		return a, cmd
	}

	// 3. Handle HelpModal active
	if a.HelpModal.Active {
		var cmd tea.Cmd
		a.HelpModal, cmd = a.HelpModal.Update(msg)
		return a, cmd
	}

	switch msg := msg.(type) {
	case cpDataRefreshedMsg:
		a.Loading = false
		if msg.Err != nil {
			a.StatusMsg = "Failed to refresh: " + msg.Err.Error()
			a.StatusType = components.StatusError
			break
		}
		a.PDUs = msg.PDUs
		a.Racks = msg.Racks
		a.StandaloneHVs = msg.HVs
		a.NISDs = msg.Nisds
		a.Vdevs = msg.Vdevs
		a.PFSs = msg.PFSs
		a.Metrics = a.CalculateMetrics()
		a.StatusMsg = "Data refreshed successfully!"
		a.StatusType = components.StatusSuccess
		// Rebuild the current page from fresh data. Tabs 1-9 are driven by
		// this topology cache; tabs 0, 10, and 11 (users/tenants/authz) hit
		// their respective load functions.
		if a.ActiveTab == 0 {
			cmds = append(cmds, a.loadUsersView())
		} else if a.ActiveTab == 10 {
			cmds = append(cmds, a.loadTenantsView())
		} else if a.ActiveTab == 11 {
			cmds = append(cmds, a.loadAuthzView())
		} else if a.ActiveTab >= 1 && a.ActiveTab <= 9 && !a.IsFormState() && !a.IsInspecting() {
			// cancelForm(), not SwitchToTab: this restores the view after a
			// background refresh, not an explicit navigation request.
			cmds = append(cmds, a.cancelForm())
		}

	case usersLoadedMsg:
		if msg.Err != nil {
			a.StatusMsg = "Failed to list users: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.ActivePage = pages.NewUserListPage(msg.Users, a.TermWidth)
			a.ResizeActivePageTable()
			a.StatusMsg = ""
		}

	case tenantsLoadedMsg:
		if msg.Err != nil {
			a.StatusMsg = "Failed to list tenants: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.ActivePage = pages.NewTenantListPage(msg.Tenants, a.TermWidth)
			a.ResizeActivePageTable()
			a.StatusMsg = ""
		}

	case authzLoadedMsg:
		if msg.Err != nil {
			a.StatusMsg = msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.ActiveTab = 11
			// Carry the previously focused table across a reload (e.g. after
			// adding or deleting an ABAC policy, stay on the ABAC table)
			// instead of always resetting to RBAC.
			focusRBAC := true
			if prev, ok := a.ActivePage.(*pages.AuthzPolicyPage); ok {
				focusRBAC = prev.FocusRBAC
			}
			newPage := pages.NewAuthzPolicyPage(msg.RBAC, msg.ABAC, a.TermWidth)
			newPage.SetFocusRBAC(focusRBAC)
			a.ActivePage = newPage
			a.ResizeActivePageTable()
			a.StatusMsg = ""
		}

	case components.SubmitCommandMsg:
		return a, a.handleCommandSubmit(msg.Command)

	case components.ConfirmResultMsg:
		a.ConfirmDialog.Active = false
		if !msg.Confirmed {
			a.StatusMsg = "Operation cancelled."
			a.StatusType = components.StatusInfo
			return a, nil
		}
		// Each branch type-asserts msg.Data and delegates to the executeSaveX/
		// executeDeleteX method owning that resource's persistence logic. cmd
		// stays nil if the assertion ever fails, so startInFlight is skipped
		// too — else the spinner would spin forever with nothing to end it.
		var cmd tea.Cmd
		switch msg.ActionID {
		case "save_pdu":
			if pdu, ok := msg.Data.(ctlplfl.PDU); ok {
				cmd = a.executeSavePDU(pdu)
			}
		case "save_rack":
			if rack, ok := msg.Data.(ctlplfl.Rack); ok {
				cmd = a.executeSaveRack(rack)
			}
		case "save_hypervisor":
			if hv, ok := msg.Data.(ctlplfl.Hypervisor); ok {
				cmd = a.executeSaveHypervisor(hv)
			}
		case "save_device":
			if dev, ok := msg.Data.(ctlplfl.Device); ok {
				cmd = a.executeSaveDevice(dev)
			}
		case "save_discovered_devices":
			if req, ok := msg.Data.(discoveredDevicesSaveRequest); ok {
				cmd = a.executeSaveDiscoveredDevices(req)
			}
		case "partition_disk":
			if req, ok := msg.Data.(partitionDiskRequest); ok {
				cmd = a.executePartitionDisk(req)
			}
		case "save_partition":
			if part, ok := msg.Data.(ctlplfl.DevicePartition); ok {
				cmd = a.executeSavePartition(part)
			}
		case "save_discovered_partitions":
			if req, ok := msg.Data.(discoveredPartitionsSaveRequest); ok {
				cmd = a.executeSaveDiscoveredPartitions(req)
			}
		case "save_whole_device_partition":
			if req, ok := msg.Data.(wholeDevicePartitionRequest); ok {
				cmd = a.executeSaveWholeDevicePartition(req)
			}
		case "save_nisd":
			if nisd, ok := msg.Data.(ctlplfl.Nisd); ok {
				cmd = a.executeSaveNISD(nisd)
			}
		case "save_nisd_batch":
			if targets, ok := msg.Data.([]domain.NISDTarget); ok {
				cmd = a.executeSaveBatchNISDs(targets)
			}
		case "save_vdev":
			if vdev, ok := msg.Data.(ctlplfl.VdevConfig); ok {
				cmd = a.executeSaveVdev(vdev)
			}
		case "save_pfs":
			if req, ok := msg.Data.(pfsSaveRequest); ok {
				cmd = a.executeSavePFS(req)
			}
		case "confirm_import_infra":
			if req, ok := msg.Data.(importInfraRequest); ok {
				cmd = a.executeImportInfra(req)
			}
		case "delete_tenant":
			if tenantUUID, ok := msg.Data.(string); ok {
				cmd = a.executeDeleteTenant(tenantUUID)
			}
		case "delete_authz_policy":
			if target, ok := msg.Data.(pages.AuthzDeleteTarget); ok {
				cmd = a.executeDeleteAuthzPolicy(target)
			}
		case "delete_vdev":
			if vdevID, ok := msg.Data.(string); ok {
				cmd = a.executeDeleteVdev(vdevID)
			}
		}
		if cmd != nil {
			return a, a.startInFlight(inFlightLabel[msg.ActionID], cmd)
		}

	case pages.SubmitPDUFormMsg:
		return a.handleSubmitPDUForm(msg)

	case resourceSavedMsg:
		a.Loading = false
		if msg.Err != nil {
			a.StatusMsg = msg.FailPrefix + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.StatusMsg = msg.SuccessMsg
			a.StatusType = components.StatusSuccess
			if a.CPClient != nil {
				cmds = append(cmds, a.refreshCPDataCmd())
			}
			cmds = append(cmds, a.SwitchToTab(msg.Tab))
		}

	case pages.SubmitRackFormMsg:
		return a.handleSubmitRackForm(msg)

	case pages.SubmitHypervisorFormMsg:
		return a.handleSubmitHypervisorForm(msg)

	case pages.SubmitDeviceFormMsg:
		return a.handleSubmitDeviceForm(msg)

	case pages.SubmitDiscoverDevicesMsg:
		hv := msg.Hypervisor
		return a, func() tea.Msg {
			devices, err := domain.DiscoverDevices(hv)
			return pages.DevicesDiscoveredMsg{Hypervisor: hv, Devices: devices, Err: err}
		}

	case pages.DeviceDiscoverySelectedMsg:
		// "Enter Manually" — blank form, nothing was discovered/chosen.
		initial := domain.Device{HypervisorID: msg.HypervisorID}
		a.ActivePage = pages.NewDeviceFormPage(false, initial, a.AllHypervisors())
		return a, nil

	case pages.SubmitDiscoveredDevicesMsg:
		return a.handleSubmitDiscoveredDevices(msg)

	case pages.SubmitPartitionDiskFormMsg:
		return a.handleSubmitPartitionDiskForm(msg)

	case partitionDiskDoneMsg:
		a.Loading = false
		if msg.Err != nil {
			a.StatusMsg = "Failed to partition disk: " + msg.Err.Error()
			a.StatusType = components.StatusError
			return a, a.SwitchToTab(4)
		}
		a.StatusMsg = fmt.Sprintf("Created %d partition(s) on %s", len(msg.Partitions), msg.DevicePath)
		a.StatusType = components.StatusSuccess
		a.ActivePage = pages.NewPartitionDiskResultPage(msg.DevicePath, msg.Partitions)
		return a, nil

	case pages.SubmitDiscoverPartitionsMsg:
		dev := msg.Device
		hv := msg.Hypervisor
		return a, func() tea.Msg {
			deviceName := strings.TrimPrefix(dev.DevicePath, "/dev/")
			parts, err := domain.GetDevicePartitionInfo(hv, deviceName)
			return pages.PartitionsDiscoveredMsg{Device: dev, Partitions: parts, Err: err}
		}

	case pages.SubmitDiscoveredPartitionsMsg:
		return a.handleSubmitDiscoveredPartitions(msg)

	case pages.SubmitWholeDevicePartitionMsg:
		return a.handleSubmitWholeDevicePartition(msg)

	case pages.SubmitPartitionFormMsg:
		return a.handleSubmitPartitionForm(msg)

	case pages.SubmitNISDFormMsg:
		return a.handleSubmitNISDForm(msg)

	case pages.SubmitNISDBatchMsg:
		return a.handleSubmitNISDBatch(msg)

	case pages.SubmitVdevFormMsg:
		return a.handleSubmitVdevForm(msg)

	case pages.SubmitPFSFormMsg:
		return a.handleSubmitPFSForm(msg)

	case pages.SubmitImportInfraFormMsg:
		return a.handleSubmitImportInfraForm(msg)

	case importInfraDoneMsg:
		a.Loading = false
		created, failed := msg.Result.Counts()
		a.StatusType = components.StatusSuccess
		if failed > 0 {
			a.StatusType = components.StatusError
		}
		a.StatusMsg = fmt.Sprintf("Import finished: %d created, %d failed", created, failed)
		a.ActivePage = pages.NewImportInfraResultPage(msg.FilePath, msg.Result)
		if a.CPClient != nil {
			cmds = append(cmds, a.refreshCPDataCmd())
		}

	case pages.SelectLoginModeMsg:
		switch msg.Mode {
		case pages.LoginModeTenantAdmin:
			a.ActivePage = pages.NewTenantAdminLoginPage()
		case pages.LoginModeCreateAdmin:
			a.ActivePage = pages.NewUserCreatePage(true)
		default:
			a.ActivePage = pages.NewUserLoginPage(a.hasTenantAdminAccess())
		}
		return a, nil

	case pages.SubmitUserLoginMsg:
		if err := a.EnsureUserClient(); err != nil {
			a.StatusMsg = err.Error()
			a.StatusType = components.StatusError
			return a, nil
		}
		username := msg.Username
		secretKey := msg.SecretKey
		tenantUUID := msg.TenantUUID
		return a, func() tea.Msg {
			resp, err := a.UserClient.LoginWithTenant(username, secretKey, tenantUUID)
			return pages.UserLoggedInMsg{
				Resp:       resp,
				Username:   username,
				SecretKey:  secretKey,
				TenantUUID: tenantUUID,
				Err:        err,
			}
		}

	case pages.SwitchToCreateAdminMsg:
		a.ActivePage = pages.NewUserCreatePage(true)
		return a, nil

	case pages.SubmitUserCreateMsg:
		if err := a.EnsureUserClient(); err != nil {
			a.StatusMsg = err.Error()
			a.StatusType = components.StatusError
			return a, nil
		}
		return a, func() tea.Msg {
			req := &userlib.UserReq{
				Username:     msg.Username,
				NewSecretKey: msg.SecretKey,
				IsAdmin:      msg.IsAdmin,
			}
			var resp *userlib.UserResp
			var err error
			if !a.IsLoggedIn() {
				// No session to authorize a CreateUser call with — covers a
				// no-auth deployment and a fresh niova-mdsvc deployment with
				// auth on but no admin yet (CreateAdminUser bootstraps that).
				resp, err = a.UserClient.CreateAdminUser(req)
			} else {
				resp, err = a.UserClient.CreateUser(a.UserToken(), req)
			}
			return pages.UserCreatedMsg{Resp: resp, Err: err}
		}

	case pages.UserLoggedInMsg:
		if msg.Err != nil {
			a.StatusMsg = "Login failed: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.Auth = authSession{User: msg.Resp, Username: msg.Username, SecretKey: msg.SecretKey, TenantUUID: msg.TenantUUID, LoginTime: time.Now()}
			// A fresh regular login starts a clean session — any earlier
			// tenant-admin session must not carry over. Re-run :tenant-login
			// to re-establish it if still needed.
			a.clearTenantAdminSession()
			statusMsg := "Successfully logged in as " + msg.Resp.Username
			if msg.TenantUUID != "" {
				statusMsg += " (Tenant: " + msg.TenantUUID + ")"
			}
			a.StatusMsg = statusMsg
			a.StatusType = components.StatusSuccess
			if a.CPClient != nil {
				a.CPClient.SetToken(msg.Resp.AccessToken)
				cmds = append(cmds, a.refreshCPDataCmd())
			}
			cmds = append(cmds, a.SwitchToTab(1))
		}

	case pages.UserCreatedMsg:
		if msg.Err != nil {
			a.StatusMsg = "User creation failed: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.StatusMsg = "User created successfully!"
			a.StatusType = components.StatusSuccess
			details := []string{
				fmt.Sprintf("User ID:    %s", msg.Resp.UserID),
				fmt.Sprintf("Username:   %s", msg.Resp.Username),
				fmt.Sprintf("Secret Key: %s", msg.Resp.SecretKey),
			}
			a.ActivePage = pages.NewUserResultPage(pages.ResultKindUser, "User Created Successfully", details)
		}

	case pages.SubmitTenantAdminLoginMsg:
		ta, ok := a.CPClient.(domain.TenantAdminAuthenticator)
		if !ok {
			a.StatusMsg = "Tenant management is only available against the mdsvc-tidb backend"
			a.StatusType = components.StatusError
			return a, nil
		}
		username := msg.Username
		password := msg.Password
		return a, func() tea.Msg {
			token, expiresIn, err := ta.TenantAdminLogin(username, password)
			return pages.TenantAdminLoggedInMsg{AccessToken: token, ExpiresIn: expiresIn, Username: username, Err: err}
		}

	case pages.TenantAdminLoggedInMsg:
		if msg.Err != nil {
			a.StatusMsg = "Tenant-admin login failed: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			// A tenant-admin login replaces any regular session rather than
			// layering on top: RoleTenantAdmin has zero /api/ access, only
			// /cp/tenants*. Run :login again to return to resource management.
			a.clearAuthSession()
			if a.CPClient != nil {
				a.CPClient.SetToken("")
			}
			a.TenantAdmin = tenantAdminSession{Token: msg.AccessToken, Username: msg.Username}
			a.StatusMsg = "Logged in as tenant-admin " + msg.Username
			a.StatusType = components.StatusSuccess
			cmds = append(cmds, a.loadTenantsView())
		}

	case pages.SubmitTenantCreateMsg:
		tm, ok := a.CPClient.(domain.TenantManager)
		if !ok {
			a.StatusMsg = "Tenant management is only available against the mdsvc-tidb backend"
			a.StatusType = components.StatusError
			return a, nil
		}
		if a.TenantAdmin.Token == "" {
			a.StatusMsg = "Not logged in as tenant-admin — run :tenant-login first"
			a.StatusType = components.StatusError
			return a, nil
		}
		displayName := msg.DisplayName
		token := a.TenantAdmin.Token
		return a, func() tea.Msg {
			result, err := tm.CreateTenant(token, displayName)
			return pages.TenantCreatedMsg{Result: result, Err: err}
		}

	case pages.TenantCreatedMsg:
		if msg.Err != nil {
			a.StatusMsg = "Tenant creation failed: " + msg.Err.Error()
			a.StatusType = components.StatusError
			if fp, ok := a.ActivePage.(*pages.TenantFormPage); ok {
				fp.Submitting = false
			}
		} else {
			a.StatusMsg = "Tenant created successfully!"
			a.StatusType = components.StatusSuccess
			details := []string{
				fmt.Sprintf("Tenant UUID:    %s", msg.Result.Tenant.TenantUUID),
				fmt.Sprintf("Display Name:   %s", msg.Result.Tenant.DisplayName),
				fmt.Sprintf("Admin Username: %s", msg.Result.AdminUsername),
				fmt.Sprintf("Admin Password: %s", msg.Result.AdminPassword),
			}
			a.ActivePage = pages.NewUserResultPage(pages.ResultKindTenant, "Tenant Created Successfully", details)
		}

	case pages.TenantDeletedMsg:
		a.Loading = false
		if msg.Err != nil {
			a.StatusMsg = "Failed to delete tenant: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.StatusMsg = "Tenant deleted."
			a.StatusType = components.StatusSuccess
			cmds = append(cmds, a.loadTenantsView())
		}

	case vdevDeletedMsg:
		a.Loading = false
		if msg.Err != nil {
			a.StatusMsg = "Failed to delete Vdev pool: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.StatusMsg = "Vdev pool deleted successfully."
			a.StatusType = components.StatusSuccess
			if a.CPClient != nil {
				cmds = append(cmds, a.refreshCPDataCmd())
			}
			cmds = append(cmds, a.SwitchToTab(7))
		}

	case pages.CopyOutputMsg:
		if msg.Err != nil {
			a.StatusMsg = "Failed to copy to clipboard: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.StatusMsg = "Copied output to clipboard!"
			a.StatusType = components.StatusSuccess
		}

	case pages.SubmitAuthzPolicyMsg:
		am, ok := a.CPClient.(domain.AuthzManager)
		if !ok {
			a.StatusMsg = "Authorization policy management is only available against the mdsvc-tidb backend"
			a.StatusType = components.StatusError
			return a, nil
		}
		a.CPClient.SetToken(a.UserToken())
		isRBAC := msg.IsRBAC
		resource, action, role := msg.Resource, msg.Action, msg.Role
		return a, func() tea.Msg {
			var err error
			if isRBAC {
				err = am.AddRBACPolicy(domain.RBACPolicy{Resource: resource, Action: action, Role: role})
			} else {
				err = am.AddABACPolicy(domain.ABACPolicy{Resource: resource, Action: action})
			}
			return pages.AuthzPolicySavedMsg{Err: err}
		}

	case pages.AuthzPolicySavedMsg:
		if msg.Err != nil {
			a.StatusMsg = "Failed to save policy: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.StatusMsg = "Policy saved."
			a.StatusType = components.StatusSuccess
			cmds = append(cmds, a.loadAuthzView())
		}

	case pages.AuthzPolicyDeletedMsg:
		a.Loading = false
		if msg.Err != nil {
			a.StatusMsg = "Failed to delete policy: " + msg.Err.Error()
			a.StatusType = components.StatusError
		} else {
			a.StatusMsg = "Policy deleted."
			a.StatusType = components.StatusSuccess
			cmds = append(cmds, a.loadAuthzView())
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			a.Quitting = true
			if a.TeardownFn != nil {
				a.TeardownFn()
			}
			return a, tea.Quit

		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if !a.globalShortcutsBlocked(false) {
				tabNum, _ := strconv.Atoi(msg.String())
				return a, a.SwitchToTab(tabNum)
			}

		case "u", "U":
			if !a.globalShortcutsBlocked(false) {
				return a, a.loadUsersView()
			}

		case "t", "T":
			if !a.globalShortcutsBlocked(false) {
				return a, a.loadTenantsView()
			}

		case "z", "Z":
			if !a.globalShortcutsBlocked(false) {
				return a, a.loadAuthzView()
			}

		case "right", "l":
			if !a.globalShortcutsBlocked(true) {
				return a, a.NextTab()
			}

		case "left", "h":
			if !a.globalShortcutsBlocked(true) {
				return a, a.PrevTab()
			}

		case "p", "P":
			if !a.globalShortcutsBlocked(true) {
				return a, a.SwitchToTab(1)
			}

		case "r", "R":
			if !a.globalShortcutsBlocked(true) {
				return a, a.SwitchToTab(2)
			}

		case "d", "D", "ctrl+d":
			if !a.globalShortcutsBlocked(false) {
				if tenantPage, ok := a.ActivePage.(*pages.TablePage[domain.TenantInfo]); ok {
					if t, ok := tenantPage.SelectedItem(); ok {
						details := []string{
							fmt.Sprintf("Tenant UUID: %s", t.TenantUUID),
							fmt.Sprintf("Schema: %s", t.SchemaName),
						}
						a.ConfirmDialog = components.NewConfirmDialog("Confirm Tenant Deletion",
							fmt.Sprintf("Delete tenant %q? This cannot be undone.", t.DisplayName),
							details, components.ConfirmTypeDanger, "delete_tenant", t.TenantUUID)
					}
					return a, nil
				}
				if authzPage, ok := a.ActivePage.(*pages.AuthzPolicyPage); ok {
					if rbac, ok := authzPage.SelectedRBAC(); ok {
						a.ConfirmDialog = components.NewConfirmDialog("Confirm Policy Deletion",
							fmt.Sprintf("Remove RBAC grant: role %q may %q %q?", rbac.Role, rbac.Action, rbac.Resource),
							nil, components.ConfirmTypeDanger, "delete_authz_policy",
							pages.AuthzDeleteTarget{IsRBAC: true, RBAC: rbac})
					} else if abac, ok := authzPage.SelectedABAC(); ok {
						a.ConfirmDialog = components.NewConfirmDialog("Confirm Policy Deletion",
							fmt.Sprintf("Remove ABAC ownership check: %q on %q?", abac.Action, abac.Resource),
							nil, components.ConfirmTypeDanger, "delete_authz_policy",
							pages.AuthzDeleteTarget{IsRBAC: false, ABAC: abac})
					}
					return a, nil
				}
				if vdevPage, ok := a.ActivePage.(*pages.TablePage[domain.VdevConfig]); ok {
					if vd, ok := vdevPage.SelectedItem(); ok {
						a.ConfirmDialog = components.NewConfirmDialog("Confirm Vdev Pool Deletion",
							fmt.Sprintf("Delete Vdev Pool %q (UUID: %s)? This cannot be undone.", vd.Name, vd.ID),
							nil, components.ConfirmTypeDanger, "delete_vdev", vd.ID)
					}
					return a, nil
				}
				if a.IsInspecting() {
					return a, a.SwitchToTab(4)
				}
			}

		case "n", "N":
			if !a.globalShortcutsBlocked(true) {
				return a, a.SwitchToTab(6)
			}

		case "v", "V":
			if !a.globalShortcutsBlocked(true) {
				return a, a.SwitchToTab(7)
			}

		case "f", "F":
			if !a.globalShortcutsBlocked(true) {
				return a, a.SwitchToTab(8)
			}

		case "ctrl+r", "f5":
			if !a.globalShortcutsBlocked(false) {
				if a.canRefreshCPData() {
					a.StatusMsg = "Refreshing topology and metrics..."
					a.StatusType = components.StatusInfo
					a.Loading = true
					return a, tea.Batch(a.Spinner.Tick, a.refreshCPDataCmd())
				}
				return a, nil
			}

		case "a", "A":
			if !a.globalShortcutsBlocked(false) {
				// Tenants has no ActiveTab slot (reached only via :tenants),
				// so it can't route through the ActiveTab-based TriggerAddForm.
				if _, ok := a.ActivePage.(*pages.TablePage[domain.TenantInfo]); ok {
					a.ActivePage = pages.NewTenantFormPage()
					return a, nil
				}
				// Same reasoning for Authorization Policies (:authz).
				if authzPage, ok := a.ActivePage.(*pages.AuthzPolicyPage); ok {
					a.ActivePage = pages.NewAuthzFormPage(authzPage.FocusRBAC)
					return a, nil
				}
				return a, a.TriggerAddForm()
			}

		case "e", "E":
			if !a.globalShortcutsBlocked(false) {
				a.TriggerEditForm()
				return a, nil
			}

		case "s", "S":
			if !a.globalShortcutsBlocked(false) {
				a.TriggerPartitionDiskForm()
				return a, nil
			}

		case ":":
			// Reachable even with a form field focused (else there's no
			// keyboard path out besides Ctrl+C). Excluded while IsFiltering()
			// though: a page's own search box needs this character too, and
			// the two share one rendered line (view.go), never open at once.
			if !a.ConfirmDialog.Active && !a.HelpModal.Active && !a.IsFiltering() {
				a.CommandBar.Active = true
				a.CommandBar.Input.Focus()
				return a, nil
			}

		case "?":
			if !a.ConfirmDialog.Active && !a.CommandBar.Active && !a.IsFormState() && !a.IsFiltering() {
				a.HelpModal.Active = !a.HelpModal.Active
				return a, nil
			}

		case "esc":
			// Inspecting view pages need no case here — they fall through to
			// the generic a.ActivePage.Update(msg) dispatch below, which
			// closes their inline detail overlay itself.

			// Pre-login screens have no tab to cancel back to, so they skip
			// the generic IsFormState() branch below and go to LoginChoicePage
			// instead — except a re-authenticating TenantAdminLoginForm
			// (ActiveTab 10, a valid return point) and the chooser itself.
			_, isLoginChoicePage := a.ActivePage.(*pages.LoginChoicePage)
			_, isUserLoginPage := a.ActivePage.(*pages.UserLoginPage)
			_, isTenantAdminLoginPage := a.ActivePage.(*pages.TenantAdminLoginPage)
			_, isUserCreatePage := a.ActivePage.(*pages.UserCreatePage)
			isBootstrapResultPage := false
			if rp, ok := a.ActivePage.(*pages.UserResultPage); ok && rp.Kind == pages.ResultKindUser {
				isBootstrapResultPage = true
			}
			if !a.IsLoggedIn() &&
				(isLoginChoicePage || isUserLoginPage || isUserCreatePage || isBootstrapResultPage ||
					(isTenantAdminLoginPage && a.TenantAdmin.Token == "")) {
				if !isLoginChoicePage {
					a.ActivePage = pages.NewLoginChoicePage(a.Backend)
				}
				return a, nil
			}

			// Pages reached via a command with no ActiveTab slot of their own
			// (TenantFormPage, UserCreatePage, AuthzFormPage, UserResultPage)
			// declare which list Esc should reload via pages.EscTarget,
			// rather than each needing its own case here.
			if et, ok := a.ActivePage.(pages.EscTarget); ok {
				switch et.EscAction() {
				case pages.EscTargetTenants:
					return a, a.loadTenantsView()
				case pages.EscTargetUsers:
					return a, a.loadUsersView()
				case pages.EscTargetAuthz:
					return a, a.loadAuthzView()
				}
			}

			if a.IsFormState() {
				return a, a.cancelForm()
			}
		}
	}

	if a.ActivePage != nil {
		var cmd tea.Cmd
		a.ActivePage, cmd = a.ActivePage.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

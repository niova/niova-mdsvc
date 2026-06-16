package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	ctlplcl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	usercl "github.com/00pauln00/niova-mdsvc/controlplane/user/client"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

type state int

const (
	stateMenu state = iota
	// PDU Management States
	statePDUManagement
	statePDUForm
	stateEditPDU
	stateDeletePDU
	stateViewPDU
	stateShowAddedPDU
	// Rack Management States
	stateRackManagement
	stateRackForm
	stateRackPDUSelection
	stateEditRack
	stateDeleteRack
	stateViewRack
	stateShowAddedRack
	// Hypervisor Management States
	stateHypervisorManagement
	stateHypervisorForm
	stateHypervisorRackSelection
	stateDeviceDiscovery
	stateDeviceSelection
	stateViewHypervisor
	stateEditHypervisor
	stateDeleteHypervisor
	stateShowAddedHypervisor
	// Device Management States
	stateDeviceManagement
	stateDeviceView
	stateDeviceDelete
	stateDevicePartitioning
	// Partition Management States
	statePartitionManagement
	statePartitionCreate
	statePartitionView
	statePartitionDelete
	stateShowAddedPartition
	statePartitionKeyCreation
	stateWholeDevicePrompt
	// NISD Management States
	stateNISDManagement
	stateNISDInitialize
	stateNISDPartitionSelection
	stateShowInitializedNISD
	stateNISDStart
	stateNISDSelection
	stateShowStartedNISD
	stateViewAllNISDs
	// Vdev Management States
	stateVdevManagement
	stateVdevForm
	stateVdevDeviceSelection
	stateEditVdev
	stateViewVdev
	stateDeleteVdev
	stateShowAddedVdev
	stateVdevCreationProgress
	stateVdevCreationSummary
	stateSearchVdev
	stateSearchVdevResult
	// PFS Management States
	statePFSManagement
	statePFSForm
	stateShowAddedPFS
	stateViewPFS
	// User Management States
	stateUserCreateForm
	stateUserCreateAdminKey
	stateUserCreateToken
	stateUserCreateResult
	// User Login States
	stateUserLoginForm
	stateUserLoginSecret
	stateUserLoginResult
	// Configuration View
	stateViewConfig
)

type inputField int

const (
	// Common fields
	inputName inputField = iota
	inputDescription
	inputLocation
	// PDU specific
	inputPowerCapacity
	// Hypervisor specific
	// inputIP
	// inputAdditionalIPs
	// inputSSHPort
	// inputPortRange
	// Device partition specific
	// inputNISDInstance
	// inputPartitionSize
	// Vdev specific
	inputVdevSearch
	inputVdevName
	inputVdevReplica
	inputVdevCount
	inputVdevSize
	inputVdevEntityUUID
	inputVdevFilterType
	inputVdevPFSName
)

type model struct {
	state   state
	logFile string

	// Terminal dimensions for dynamic pagination
	termWidth  int
	termHeight int

	// Menu
	menuCursor int
	menuItems  []menuItem

	// PDU Management
	pduManagementCursor int
	pduListCursor       int

	// Rack Management
	rackManagementCursor int
	rackListCursor       int
	selectedPDUIdx       int
	pduSelectionCursor   int // For PDU selection menu

	// Hypervisor Management
	hvManagementCursor  int
	hvListCursor        int
	selectedRackIdx     int
	rackSelectionCursor int // For Rack selection menu
	editingUUID         string

	// Forms (shared for PDU, Rack, Hypervisor)
	inputs       []textinput.Model
	focusedInput inputField
	currentPDU   ctlplfl.PDU
	currentRack  ctlplfl.Rack
	currentHv    ctlplfl.Hypervisor

	// Dynamic IP Address Management
	ipInputs         []textinput.Model // Dynamic list of IP address inputs
	ipFocusedIndex   int               // Which IP input is currently focused
	ipManagementMode bool              // Whether we're in IP management mode

	// Device Discovery
	loading bool

	// Device Selection
	deviceList      list.Model
	deviceCursor    int
	devicePage      int
	devicesPerPage  int
	discoveredDevs  []ctlplfl.Device
	selectedDevices map[int]bool

	// Config View
	configViewport viewport.Model
	configCursor   int
	expandedHvs    map[int]bool // Track which hypervisors are expanded

	// Hierarchical expansion tracking
	expandedPDUs    map[int]bool    // Track which PDUs are expanded
	expandedRacks   map[string]bool // Track which racks are expanded (by UUID)
	expandedHypers  map[string]bool // Track which hypervisors are expanded (by UUID)
	expandedDevices map[string]bool // Track which devices are expanded (by UUID)

	// Device Management
	deviceMgmtCursor      int
	selectedHypervisorIdx int
	selectedDeviceIdx     int
	deviceFailureDomain   textinput.Model

	// Partition Management
	partitionMgmtCursor        int
	selectedPartitionIdx       int
	currentPartition           DevicePartition
	partitionCountInput        textinput.Model
	selectedDeviceForPartition Device
	selectedHvForPartition     ctlplfl.Hypervisor
	// Partition Key Creation
	existingPartitions           []DevicePartitionInfo
	selectedExistingPartitionIdx int
	selectedPartitions           map[int]bool // Track which partitions are selected

	// NISD Management
	nisdMgmtCursor int
	// nisdCursor                int // For NISD pagination
	selectedNISDIdx           int // For NISD item navigation like partitions
	selectedNISDPartitionIdx  int
	selectedNISDHypervisorIdx int
	selectedNISDDeviceIdx     int
	selectedNISDForStart      int
	selectedNISDPartitions    map[int]bool // Track which partitions are selected for NISD initialization
	currentNISD               ctlplfl.Nisd
	// selectedPartitionForNISD  DevicePartition
	// selectedHvForNISD         ctlplfl.Hypervisor
	// selectedDeviceForNISD     Device
	selectedNISDToStart ctlplfl.Nisd

	// Vdev Management
	vdevMgmtCursor         int
	vdevDeleteCursor       int
	vdevViewCursor         int
	currentVdev            ctlplfl.Vdev
	selectedDevicesForVdev map[int]bool // Track which devices are selected for Vdev creation
	vdevSearchInput        textinput.Model
	vdevSearchResult       *ctlplfl.Vdev
	vdevSearchErr          string
	vdevSearchChunkPage    int
	vdevNameInput          textinput.Model
	vdevReplicaInput       textinput.Model
	vdevSizeInput          textinput.Model
	vdevCountInput         textinput.Model
	vdevEntityUUIDInput    textinput.Model
	vdevFilterTypeInput    textinput.Model
	vdevPFSNameInput       textinput.Model
	vdevFormActiveField    inputField              // Track which field is currently active
	createdVdevs           []ctlplfl.VdevCfg       // Store created Vdevs for summary
	nisdCache              map[string]ctlplfl.Nisd // NISD UUID → NISD info, populated on device view entry
	vdevCreationProgress   int                     // Track creation progress
	vdevCreationTotal      int                     // Total Vdevs to create
	vdevCreationErrors     []string                // Store any creation errors
	vdevCreationPFSID      string                  // PFS UUID for current creation batch
	vdevCreationPFSName    string                  // PFS name for current creation batch

	// PFS Management
	pfsMgmtCursor int
	pfsViewCursor int
	currentPFS    ctlplfl.PFS
	pfsNameInput  textinput.Model

	// Control Plane
	cpClient            *ctlplcl.CliCFuncs
	cpEnabled           bool
	cpRaftUUID          string
	cpGossipPath        string
	cpConnected         bool
	cpPDUs              []ctlplfl.PDU        // PDU data from Control Plane
	cpPDURefresh        bool                 // Flag to trigger PDU refresh from CP
	cpRacks             []ctlplfl.Rack       // Rack data from Control Plane
	cpRackRefresh       bool                 // Flag to trigger Rack refresh from CP
	cpHypervisors       []ctlplfl.Hypervisor // Hypervisor data from Control Plane
	cpHypervisorRefresh bool                 // Flag to trigger Hypervisor refresh from CP

	// User Management
	authEnabled        bool // true when the server has authentication enabled
	userClient         *usercl.Client
	userUsernameInput  textinput.Model
	userSecretKeyInput textinput.Model
	userTokenInput     textinput.Model
	userCreateResp     *userlib.UserResp
	userCreateErr      string
	loggedInUser       *userlib.LoginResp
	userLoginErr       string

	// General
	message  string
	quitting bool
}

type menuItem struct {
	title string
	desc  string
}

type deviceItem struct {
	device   ctlplfl.Device
	index    int
	selected bool
}

func (d deviceItem) Title() string { return d.device.Name }
func (d deviceItem) Description() string {
	return fmt.Sprintf("ID: %s", d.device.ID)
}

func (d deviceItem) FilterValue() string { return d.device.Name }

type deviceDiscoveredMsg struct {
	devices []ctlplfl.Device
	err     error
}

type deviceStateRefreshMsg struct {
	devices []ctlplfl.Device
	nisds   []ctlplfl.Nisd
	err     error
}

type userCreatedMsg struct {
	resp *userlib.UserResp
	err  error
}

type userLoggedInMsg struct {
	resp *userlib.LoginResp
	err  error
}

// userClientTeardownFn holds the teardown function for the lazily-initialized
// user client. It's package-level because Bubbletea copies the model by value,
// so a teardown set inside Update() wouldn't be visible in main().
var userClientTeardownFn func()

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	focusedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#01BE85")).
			Background(lipgloss.Color("#0D1117")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#01BE85"))

	blurredStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#333333"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F87"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#50FA7B"))

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#01BE85")).
				Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#01BE85")).
			Bold(true)
)

// isValidIP checks if the given string is a valid IP address
func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// formatIPAddresses formats multiple IP addresses for display
func formatIPAddresses(hv ctlplfl.Hypervisor) string {
	if len(hv.IPAddrs) <= 0 {
		return "no network info"
	}
	if len(hv.IPAddrs) == 1 {
		return hv.IPAddrs[0]
	}
	return fmt.Sprintf("%s +%d more", hv.IPAddrs[0], len(hv.IPAddrs)-1)
}

// formatDetailedIPAddresses formats all IP addresses for detailed views
func formatDetailedIPAddresses(hv ctlplfl.Hypervisor) string {
	if len(hv.IPAddrs) == 0 {
		return "" // Fallback to old field
	}
	if len(hv.IPAddrs) == 1 {
		return hv.IPAddrs[0]
	}
	result := fmt.Sprintf("IP Addresses: %s", hv.IPAddrs[0])
	for i := 1; i < len(hv.IPAddrs); i++ {
		result += fmt.Sprintf(", %s", hv.IPAddrs[i])
	}
	return result
}

// createIPInput creates a new IP address input field
func createIPInput() textinput.Model {
	t := textinput.New()
	t.Placeholder = "192.168.1.100"
	t.CharLimit = 1000
	t.Width = 20
	return t
}

func initialModel(cpEnabled bool, cpRaftUUID, cpGossipPath, logFile string) model {
	if logFile == "" {
		logFile = ctlplfl.DefaultLogPath()
	}
	// Initialize text inputs (expanded for all form types)
	var inputs []textinput.Model
	for i := 0; i < 9; i++ {
		t := textinput.New()
		t.CharLimit = 1000
		switch i {
		case 0: // inputName
			t.Placeholder = "Enter name"
			t.Focus()
		case 1: // inputDescription
			t.Placeholder = "Enter description (optional)"
		case 2: // inputLocation
			t.Placeholder = "Enter location (optional)"
		case 3: // inputPowerCapacity
			t.Placeholder = "Enter power capacity (e.g., 30kW)"
		case 4: // inputIP
			t.Placeholder = "192.168.1.100"
		case 5: // inputAdditionalIPs
			t.Placeholder = "Additional IPs (comma-separated): 192.168.1.101,192.168.2.100"
		case 6: // inputSSHPort
			t.Placeholder = "22"
		case 7: // inputPortRange
			t.Placeholder = "8000-8100"
		case 8: // inputNISDInstance/inputPartitionSize
			t.Placeholder = "Enter value"
		}
		inputs = append(inputs, t)
	}

	// Initialize IP address inputs (start with one)
	ipInputs := []textinput.Model{createIPInput()}

	// Initialize device list
	deviceList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	deviceList.Title = "Storage Devices"
	deviceList.SetShowStatusBar(false)
	deviceList.SetShowFilter(false)
	deviceList.SetShowHelp(false)

	// Initialize viewport
	vp := viewport.New(80, 20)
	vp.SetContent("Loading...")

	// Initialize device failure domain input
	deviceFailureDomainInput := textinput.New()
	deviceFailureDomainInput.Placeholder = "Enter failure domain for device"
	deviceFailureDomainInput.CharLimit = 500

	// Initialize vdev search input
	vdevSearchInput := textinput.New()
	vdevSearchInput.Placeholder = "vdev name or UUID"
	vdevSearchInput.CharLimit = 64

	// Initialize vdev name input
	vdevNameInput := textinput.New()
	vdevNameInput.Placeholder = "unique name (letters and digits only)"
	vdevNameInput.CharLimit = 64

	// Initialize vdev replica input
	vdevReplicaInput := textinput.New()
	vdevReplicaInput.Placeholder = "1"
	vdevReplicaInput.CharLimit = 4
	vdevReplicaInput.SetValue("1")

	// Initialize vdev size input
	vdevSizeInput := textinput.New()
	vdevSizeInput.Placeholder = "e.g., 10GB, 1TB, 1PB"
	vdevSizeInput.CharLimit = 32

	// Initialize vdev count input
	vdevCountInput := textinput.New()
	vdevCountInput.Placeholder = "1"
	vdevCountInput.CharLimit = 10
	vdevCountInput.SetValue("1")

	// Initialize vdev entity UUID input
	vdevEntityUUIDInput := textinput.New()
	vdevEntityUUIDInput.Placeholder = "optional: entity UUID (PDU/Rack/HV UUID)"
	vdevEntityUUIDInput.CharLimit = 64

	// Initialize vdev filter type input
	vdevFilterTypeInput := textinput.New()
	vdevFilterTypeInput.Placeholder = "any|pdu|rack|hv|device (default: any)"
	vdevFilterTypeInput.CharLimit = 16
	vdevFilterTypeInput.SetValue("any")

	vdevPFSNameInput := textinput.New()
	vdevPFSNameInput.Placeholder = "PFS name or UUID (optional)"
	vdevPFSNameInput.CharLimit = 64

	// Initialize PFS name input
	pfsNameInput := textinput.New()
	pfsNameInput.Placeholder = "Enter PFS name"
	pfsNameInput.CharLimit = 64

	// Initialize user management text inputs
	userUsernameInput := textinput.New()
	userUsernameInput.Placeholder = "Enter username (use 'admin' for admin user)"
	userUsernameInput.CharLimit = 256

	userSecretKeyInput := textinput.New()
	userSecretKeyInput.Placeholder = "Enter secret key (leave empty to auto-generate)"
	userSecretKeyInput.CharLimit = 512

	userTokenInput := textinput.New()
	userTokenInput.Placeholder = "Enter admin JWT token"
	userTokenInput.CharLimit = 2048

	isConnected := false
	cpClient := initControlPlane(cpRaftUUID, cpGossipPath, logFile)
	if cpClient != nil {
		isConnected = true
	}

	// Probe the server to determine whether authentication is enabled.
	// When auth is disabled the server grants admin access for any token
	var loggedInUser *userlib.LoginResp
	// Initialize menu items
	authDetected := cpClient != nil && probeServerAuth(cpClient)
	if authDetected {
		log.Info("niova-ctl: server authentication is enabled")
	} else {
		log.Warn("niova-ctl: server authentication is disabled")
	}
	menuItems := []menuItem{}
	if authDetected {
		menuItems = append(menuItems, menuItem{"Login", "Login with username and secret key"})
		menuItems = append(menuItems, menuItem{"Create User", "Create admin or regular users"})
	}

	menuItems = append(menuItems, []menuItem{
		{"Manage PDUs", "Add, edit, or delete Power Distribution Units"},
		{"Manage Racks", "Add, edit, or delete server racks"},
		{"Manage Hypervisors", "Add, edit, or delete hypervisors"},
		{"Manage Devices", "Initialize devices on hypervisors"},
		{"Manage Partitions", "Create, view, and delete NISD partitions"},
		{"Manage NISDs", "Initialize NISD instances on device partitions"},
		{"Manage Vdevs", "Create and manage virtual devices"},
		{"Manage PFS", "Create and view Parallel File Systems"},
		{"View Configuration", "Display current hierarchical configuration from control plane"},
		{"Exit", "Exit"},
	}...)

	return model{
		state:            stateMenu,
		menuCursor:       0,
		menuItems:        menuItems,
		inputs:           inputs,
		focusedInput:     inputName,
		ipInputs:         ipInputs,
		ipFocusedIndex:   0,
		ipManagementMode: false,
		deviceList:       deviceList,
		devicePage:       0,
		devicesPerPage:   10, // Show 10 devices per page
		configViewport:   vp,
		configCursor:     0,
		expandedHvs:      make(map[int]bool),
		selectedDevices:  make(map[int]bool),
		// Initialize hierarchical expansion maps
		expandedPDUs:          make(map[int]bool),
		expandedRacks:         make(map[string]bool),
		expandedHypers:        make(map[string]bool),
		expandedDevices:       make(map[string]bool),
		deviceMgmtCursor:      0,
		selectedPDUIdx:        -1,
		selectedRackIdx:       -1,
		selectedHypervisorIdx: -1,
		selectedDeviceIdx:     -1,
		deviceFailureDomain:   deviceFailureDomainInput,
		vdevSearchInput:       vdevSearchInput,
		vdevNameInput:         vdevNameInput,
		vdevReplicaInput:      vdevReplicaInput,
		vdevSizeInput:         vdevSizeInput,
		vdevCountInput:        vdevCountInput,
		vdevEntityUUIDInput:   vdevEntityUUIDInput,
		vdevFilterTypeInput:   vdevFilterTypeInput,
		vdevPFSNameInput:      vdevPFSNameInput,
		vdevFormActiveField:   inputVdevName,
		pfsNameInput:          pfsNameInput,
		// Control plane configuration
		logFile:      logFile,
		cpEnabled:    cpEnabled,
		cpRaftUUID:   cpRaftUUID,
		cpGossipPath: cpGossipPath,
		cpClient:     cpClient,
		cpConnected:  isConnected,
		// User management
		authEnabled:        authDetected,
		userUsernameInput:  userUsernameInput,
		userSecretKeyInput: userSecretKeyInput,
		userTokenInput:     userTokenInput,
		loggedInUser:       loggedInUser,
	}
}

// probeServerAuth reports whether the server has authentication enabled.
// It sends a GetPDUs request with an empty token: if the server rejects it
// with an auth error, auth is enabled (returns true); if the request succeeds,
// auth is disabled (returns false).
func probeServerAuth(cpClient *ctlplcl.CliCFuncs) bool {
	_, err := cpClient.GetNisds(ctlplfl.GetReq{ID: uuid.NewString()})
	if err == nil {
		log.Infof("Auth probe: server has authentication disabled error: %v\n", err)
		return false
	}
	if strings.Contains(err.Error(), "Failed to lookup for key") {
		log.Info("Auth probe: server has authentication disabled (no key lookup)")
		return false
	}
	log.Info("Auth probe: server has authentication enabled (", err, ")")
	return true
}

// initControlPlane initializes the control plane client if enabled
func initControlPlane(raftUUID, gossipPath, logFile string) *ctlplcl.CliCFuncs {

	log.Info("initControlPlane")
	if raftUUID == "" || gossipPath == "" {
		log.Warn("Control plane enabled but missing raft UUID or gossip path")
		return nil
	}

	log.Info("Initializing control plane client, raft uuid: ", raftUUID)
	appUUID := uuid.New().String()

	// Initialize control plane client
	cpClient := ctlplcl.InitCliCFuncs(appUUID, raftUUID, gossipPath, logFile)

	// Set connected status - InitCliCFuncs starts async connection
	log.Info("Control plane client initialized with UUID: ", appUUID)
	return cpClient
}

// initUserClient initializes the user management client
func initUserClient(raftUUID, gossipPath, logFile string) (*usercl.Client, func()) {
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

// refreshCPData fetches all data from the control plane and assembles the full hierarchy.
func (m *model) refreshCPData() {
	if m.cpClient == nil {
		return
	}
	token := m.userToken()

	// Fetch flat lists from CP
	m.cpClient.SetToken(token)
	pdus, err := m.cpClient.GetPDUs(&ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("refreshCPData: GetPDUs failed: ", err)
		pdus = []ctlplfl.PDU{}
	}

	racks, err := m.cpClient.GetRacks(&ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("refreshCPData: GetRacks failed: ", err)
		racks = []ctlplfl.Rack{}
	}
	m.cpRacks = racks

	hvs, err := m.cpClient.GetHypervisor(&ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("refreshCPData: GetHypervisor failed: ", err)
		hvs = []ctlplfl.Hypervisor{}
	}

	// Fetch devices and stitch into Hypervisors
	devices, err := m.cpClient.GetDevices(ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("refreshCPData: GetDevices failed: ", err)
		devices = []ctlplfl.Device{}
	}

	// Fetch partitions and stitch into Devices
	partitions, err := m.cpClient.GetPartition(ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("refreshCPData: GetPartition failed: ", err)
		partitions = []ctlplfl.DevicePartition{}
	}

	// Build device map, clear stale partition slices, stitch partitions in
	devMap := make(map[string]*ctlplfl.Device, len(devices))
	for i := range devices {
		devices[i].Partitions = nil
		devMap[devices[i].ID] = &devices[i]
	}
	for _, p := range partitions {
		if d, ok := devMap[p.DevID]; ok {
			d.Partitions = append(d.Partitions, p)
		}
	}

	// Build hypervisor map and clear stale device slices
	hvMap := make(map[string]*ctlplfl.Hypervisor, len(hvs))
	for i := range hvs {
		hvs[i].Dev = nil
		hvMap[hvs[i].ID] = &hvs[i]
	}
	for _, dev := range devices {
		if hv, ok := hvMap[dev.HypervisorID]; ok {
			hv.Dev = append(hv.Dev, dev)
		}
	}

	// Stitch Hypervisors into Racks
	rackMap := make(map[string]*ctlplfl.Rack, len(racks))
	for i := range racks {
		racks[i].Hypervisors = nil
		rackMap[racks[i].ID] = &racks[i]
	}
	var standaloneHVs []ctlplfl.Hypervisor
	for _, hv := range hvs {
		if r, ok := rackMap[hv.RackID]; ok {
			r.Hypervisors = append(r.Hypervisors, hv)
		} else {
			standaloneHVs = append(standaloneHVs, hv)
		}
	}
	m.cpHypervisors = standaloneHVs

	// Stitch Racks into PDUs
	pduMap := make(map[string]*ctlplfl.PDU, len(pdus))
	for i := range pdus {
		pdus[i].Racks = nil
		pduMap[pdus[i].ID] = &pdus[i]
	}
	for _, rack := range racks {
		if p, ok := pduMap[rack.PDUID]; ok {
			p.Racks = append(p.Racks, rack)
		}
	}
	m.cpPDUs = pdus
}

// findHypervisorByUUID searches cpPDUs and cpHypervisors for a hypervisor with the given UUID.
func (m model) findHypervisorByUUID(hvUUID string) (*ctlplfl.Hypervisor, bool) {
	for i := range m.cpPDUs {
		for j := range m.cpPDUs[i].Racks {
			for k := range m.cpPDUs[i].Racks[j].Hypervisors {
				if m.cpPDUs[i].Racks[j].Hypervisors[k].ID == hvUUID {
					return &m.cpPDUs[i].Racks[j].Hypervisors[k], true
				}
			}
		}
	}
	for i := range m.cpHypervisors {
		if m.cpHypervisors[i].ID == hvUUID {
			return &m.cpHypervisors[i], true
		}
	}
	return nil, false
}

// findRackByUUID searches cpPDUs for a rack with the given UUID.
func (m model) findRackByUUID(rackUUID string) (*ctlplfl.Rack, bool) {
	for i := range m.cpPDUs {
		for j := range m.cpPDUs[i].Racks {
			if m.cpPDUs[i].Racks[j].ID == rackUUID {
				return &m.cpPDUs[i].Racks[j], true
			}
		}
	}
	return nil, false
}

// getAllDevicesForPartitioning returns all devices from cpPDUs and cpHypervisors.
func (m model) getAllDevicesForPartitioning() []struct {
	HvUUID string
	HvName string
	Device Device
} {
	var devices []struct {
		HvUUID string
		HvName string
		Device Device
	}
	for _, pdu := range m.cpPDUs {
		for _, rack := range pdu.Racks {
			for _, hv := range rack.Hypervisors {
				for _, dev := range hv.Dev {
					devices = append(devices, struct {
						HvUUID string
						HvName string
						Device Device
					}{HvUUID: hv.ID, HvName: hv.Name, Device: dev})
				}
			}
		}
	}
	for _, hv := range m.cpHypervisors {
		for _, dev := range hv.Dev {
			devices = append(devices, struct {
				HvUUID string
				HvName string
				Device Device
			}{HvUUID: hv.ID, HvName: hv.Name, Device: dev})
		}
	}
	return devices
}

// getAllDevicesWithPartitions returns only initialized devices that have partitions.
func (m model) getAllDevicesWithPartitions() []struct {
	HvUUID string
	HvName string
	Device Device
} {
	var devices []struct {
		HvUUID string
		HvName string
		Device Device
	}
	for _, pdu := range m.cpPDUs {
		for _, rack := range pdu.Racks {
			for _, hv := range rack.Hypervisors {
				for _, dev := range hv.Dev {
					if dev.State == ctlplfl.INITIALIZED {
						devices = append(devices, struct {
							HvUUID string
							HvName string
							Device Device
						}{HvUUID: hv.ID, HvName: hv.Name, Device: dev})
					}
				}
			}
		}
	}
	for _, hv := range m.cpHypervisors {
		for _, dev := range hv.Dev {
			if dev.State == ctlplfl.INITIALIZED {
				devices = append(devices, struct {
					HvUUID string
					HvName string
					Device Device
				}{HvUUID: hv.ID, HvName: hv.Name, Device: dev})
			}
		}
	}
	return devices
}

// getDevicesWithoutPartitions returns devices that have no partitions yet.
func (m model) getDevicesWithoutPartitions() []struct {
	HvUUID string
	HvName string
	Device Device
} {
	all := m.getAllDevicesForPartitioning()
	var result []struct {
		HvUUID string
		HvName string
		Device Device
	}
	for _, d := range all {
		if len(d.Device.Partitions) == 0 {
			result = append(result, d)
		}
	}
	return result
}

// getAllPartitionsForNISD returns all partitions available for NISD initialization from CP data.
func (m model) getAllPartitionsForNISD() []struct {
	HvUUID    string
	HvName    string
	Device    Device
	Partition DevicePartition
} {
	var result []struct {
		HvUUID    string
		HvName    string
		Device    Device
		Partition DevicePartition
	}
	allDevices := m.getAllDevicesForPartitioning()
	for _, deviceInfo := range allDevices {
		for _, partition := range deviceInfo.Device.Partitions {
			result = append(result, struct {
				HvUUID    string
				HvName    string
				Device    Device
				Partition DevicePartition
			}{
				HvUUID:    deviceInfo.HvUUID,
				HvName:    deviceInfo.HvName,
				Device:    deviceInfo.Device,
				Partition: partition,
			})
		}
	}
	return result
}

func (m model) Init() tea.Cmd {
	// Initialize control plane if enabled
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.deviceList.SetSize(msg.Width-4, msg.Height-8)
		m.configViewport.Width = msg.Width - 4
		m.configViewport.Height = msg.Height - 10 // Leave room for header and footer

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.state != stateMenu {
				m.state = stateMenu
				m.message = ""
				return m, nil
			}
		}

	case deviceDiscoveredMsg:
		m.loading = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Error discovering devices: %v", msg.err)
			m.state = stateMenu
		} else {
			m.discoveredDevs = msg.devices
			m.selectedDevices = make(map[int]bool)
			m.deviceCursor = 0
			m.devicePage = 0
			m.state = stateDeviceSelection
			m = m.updateDeviceList()
		}
		return m, nil

	case userCreatedMsg:
		if msg.err != nil {
			m.userCreateErr = msg.err.Error()
			m.userCreateResp = nil
		} else if !msg.resp.Success {
			m.userCreateErr = msg.resp.Error
			m.userCreateResp = nil
		} else {
			m.userCreateResp = msg.resp
			m.userCreateErr = ""
		}
		m.state = stateUserCreateResult
		return m, nil

	case userLoggedInMsg:
		if msg.err != nil {
			m.userLoginErr = msg.err.Error()
			m.state = stateUserLoginResult
		} else if !msg.resp.Success {
			m.userLoginErr = msg.resp.Error
			m.state = stateUserLoginResult
		} else {
			m.loggedInUser = msg.resp
			m.message = "Logged in as " + msg.resp.Username
			m.state = stateMenu
		}
		return m, nil
	}

	switch m.state {
	case stateMenu:
		m, cmd = m.updateMenu(msg)
	// PDU Management
	case statePDUManagement:
		m, cmd = m.updatePDUManagement(msg)
	case statePDUForm:
		m, cmd = m.updatePDUForm(msg)
	case stateEditPDU:
		m, cmd = m.updateEditPDU(msg)
	case stateDeletePDU:
		m, cmd = m.updateDeletePDU(msg)
	case stateViewPDU:
		m, cmd = m.updateViewPDU(msg)
	case stateShowAddedPDU:
		m, cmd = m.updateShowAddedPDU(msg)
	// Rack Management
	case stateRackManagement:
		m, cmd = m.updateRackManagement(msg)
	case stateRackForm:
		m, cmd = m.updateRackForm(msg)
	case stateRackPDUSelection:
		m, cmd = m.updateRackPDUSelection(msg)
	case stateEditRack:
		m, cmd = m.updateEditRack(msg)
	case stateDeleteRack:
		m, cmd = m.updateDeleteRack(msg)
	case stateViewRack:
		m, cmd = m.updateViewRack(msg)
	case stateShowAddedRack:
		m, cmd = m.updateShowAddedRack(msg)
	// Hypervisor Management
	case stateHypervisorManagement:
		m, cmd = m.updateHypervisorManagement(msg)
	case stateHypervisorForm:
		m, cmd = m.updateHypervisorForm(msg)
	case stateHypervisorRackSelection:
		m, cmd = m.updateHypervisorRackSelection(msg)
	case stateDeviceDiscovery:
		// Loading state, no updates needed
	case stateDeviceSelection:
		m, cmd = m.updateDeviceSelection(msg)
	case stateViewHypervisor:
		m, cmd = m.updateViewHypervisor(msg)
	case stateEditHypervisor:
		m, cmd = m.updateEditHypervisor(msg)
	case stateDeleteHypervisor:
		m, cmd = m.updateDeleteHypervisor(msg)
	case stateShowAddedHypervisor:
		m, cmd = m.updateShowAddedHypervisor(msg)
	// Device Management
	case stateDeviceManagement:
		m, cmd = m.updateDeviceManagement(msg)
	case stateDeviceView:
		m, cmd = m.updateDeviceView(msg)
	case stateDeviceDelete:
		m, cmd = m.updateDeviceDelete(msg)
	case stateDevicePartitioning:
		m, cmd = m.updateDevicePartitioning(msg)
	// Partition Management
	case statePartitionManagement:
		m, cmd = m.updatePartitionManagement(msg)
	case statePartitionCreate:
		m, cmd = m.updatePartitionCreate(msg)
	case statePartitionView:
		m, cmd = m.updatePartitionView(msg)
	case statePartitionDelete:
		m, cmd = m.updatePartitionDelete(msg)
	case stateShowAddedPartition:
		m, cmd = m.updateShowAddedPartition(msg)
	case statePartitionKeyCreation:
		m, cmd = m.updatePartitionKeyCreation(msg)
	case stateWholeDevicePrompt:
		m, cmd = m.updateWholeDevicePrompt(msg)
	// NISD Management
	case stateNISDManagement:
		m, cmd = m.updateNISDManagement(msg)
	case stateNISDInitialize:
		m, cmd = m.updateNISDInitialize(msg)
	case stateNISDPartitionSelection:
		m, cmd = m.updateNISDPartitionSelection(msg)
	case stateShowInitializedNISD:
		m, cmd = m.updateShowInitializedNISD(msg)
	case stateNISDStart:
		m, cmd = m.updateNISDStart(msg)
	case stateNISDSelection:
		m, cmd = m.updateNISDSelection(msg)
	case stateShowStartedNISD:
		m, cmd = m.updateShowStartedNISD(msg)
	case stateViewAllNISDs:
		m, cmd = m.updateViewAllNISDs(msg)
	// Vdev Management
	case stateVdevManagement:
		m, cmd = m.updateVdevManagement(msg)
	case stateVdevForm:
		m, cmd = m.updateVdevForm(msg)
	case stateVdevDeviceSelection:
		m, cmd = m.updateVdevDeviceSelection(msg)
	case stateEditVdev:
		m, cmd = m.updateEditVdev(msg)
	case stateViewVdev:
		m, cmd = m.updateViewVdev(msg)
	case stateDeleteVdev:
		m, cmd = m.updateDeleteVdev(msg)
	case stateShowAddedVdev:
		m, cmd = m.updateShowAddedVdev(msg)
	case stateVdevCreationProgress:
		m, cmd = m.updateVdevCreationProgress(msg)
	case stateVdevCreationSummary:
		m, cmd = m.updateVdevCreationSummary(msg)
	case stateSearchVdev:
		m, cmd = m.updateSearchVdev(msg)
	case stateSearchVdevResult:
		m, cmd = m.updateSearchVdevResult(msg)
	// PFS Management
	case statePFSManagement:
		m, cmd = m.updatePFSManagement(msg)
	case statePFSForm:
		m, cmd = m.updatePFSForm(msg)
	case stateShowAddedPFS:
		m, cmd = m.updateShowAddedPFS(msg)
	case stateViewPFS:
		m, cmd = m.updateViewPFS(msg)
	// User Management
	case stateUserCreateForm:
		m, cmd = m.updateUserCreateForm(msg)
	case stateUserCreateAdminKey:
		m, cmd = m.updateUserCreateAdminKey(msg)
	case stateUserCreateToken:
		m, cmd = m.updateUserCreateToken(msg)
	case stateUserCreateResult:
		m, cmd = m.updateUserCreateResult(msg)
	// User Login
	case stateUserLoginForm:
		m, cmd = m.updateUserLoginForm(msg)
	case stateUserLoginSecret:
		m, cmd = m.updateUserLoginSecret(msg)
	case stateUserLoginResult:
		m, cmd = m.updateUserLoginResult(msg)
	// Control Plane
	// Configuration View
	case stateViewConfig:
		m, cmd = m.updateViewConfig(msg)
	}

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) updateMenu(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	// TODO define the macros for the case options.
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.menuCursor > 0 {
				m.menuCursor--
			}
		case "down", "j":
			if m.menuCursor < len(m.menuItems)-1 {
				m.menuCursor++
			}
		case "enter", " ":
			// When auth is disabled the Login and Create User items are absent.
			effectiveCase := m.menuCursor
			if !m.authEnabled {
				effectiveCase += 2
			}
			switch effectiveCase {
			case 0: // Login
				var err error
				m, err = m.ensureUserClient()
				if err != nil {
					m.message = "Error: " + err.Error()
					return m, nil
				}
				m.state = stateUserLoginForm
				m.userUsernameInput.SetValue("")
				m.userSecretKeyInput.SetValue("")
				m.userLoginErr = ""
				m.message = ""
				m.userUsernameInput.Focus()
				return m, textinput.Blink
			case 1: // Create User
				var err error
				m, err = m.ensureUserClient()
				if err != nil {
					m.message = "Error: " + err.Error()
					return m, nil
				}
				m.state = stateUserCreateForm
				m.userUsernameInput.SetValue("")
				m.userSecretKeyInput.SetValue("")
				m.userTokenInput.SetValue("")
				m.userCreateResp = nil
				m.userCreateErr = ""
				m.message = ""
				m.userUsernameInput.Focus()
				return m, textinput.Blink
			case 2: // Manage PDUs
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = statePDUManagement
				m.pduManagementCursor = 0
				m.message = ""
				m.refreshCPData()
				return m, nil
			case 3: // Manage Racks
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = stateRackManagement
				m.rackManagementCursor = 0
				m.selectedPDUIdx = -1
				m.message = ""
				m.refreshCPData()
				return m, nil
			case 4: // Manage Hypervisors
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = stateHypervisorManagement
				m.hvManagementCursor = 0
				m.selectedRackIdx = -1
				m.message = ""
				m.refreshCPData()
				return m, nil
			case 5: // Manage Devices
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = stateDeviceManagement
				m.deviceMgmtCursor = 0
				m.selectedHypervisorIdx = -1
				m.selectedDeviceIdx = -1
				m.message = ""
				m.refreshCPData()
				return m, nil
			case 6: // Manage Partitions
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = statePartitionManagement
				m.partitionMgmtCursor = 0
				m.selectedPartitionIdx = -1
				m.message = ""
				m.refreshCPData()
				return m, nil
			case 7: // Manage NISDs
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = stateNISDManagement
				m.nisdMgmtCursor = 0
				m.selectedNISDHypervisorIdx = -1
				m.selectedNISDDeviceIdx = -1
				m.selectedNISDPartitionIdx = -1
				m.selectedNISDPartitions = make(map[int]bool)
				m.message = ""
				m.refreshCPData()
				return m, nil
			case 8: // Manage Vdevs
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = stateVdevManagement
				m.vdevMgmtCursor = 0
				m.selectedDevicesForVdev = make(map[int]bool)
				m.message = ""
				return m, nil
			case 9: // Manage PFS
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = statePFSManagement
				m.pfsMgmtCursor = 0
				m.message = ""
				return m, nil
			case 10: // View Configuration
				if m.authEnabled && m.loggedInUser == nil {
					m.message = "Error: Please login first"
					return m, nil
				}
				m.state = stateViewConfig
				// Reset config cursor and expand states
				m.configCursor = 0
				m.expandedHvs = make(map[int]bool)
				// Refresh data from control plane
				m.refreshCPData()
				m.message = fmt.Sprintf("Loaded %d PDUs and %d hypervisors from control plane", len(m.cpPDUs), len(m.cpHypervisors))
				m = m.updateConfigView()
				return m, nil
			case 11: // Exit
				m.quitting = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// userToken returns the logged-in user's access token, or empty string if not logged in.
func (m model) userToken() string {
	if m.loggedInUser != nil {
		return m.loggedInUser.AccessToken
	}
	return ""
}

// fetchDeviceStatesFromCP queries the CP for all devices and NISDs, returning a deviceStateRefreshMsg.
func (m model) fetchDeviceStatesFromCP() tea.Msg {
	if m.cpClient == nil || !m.cpConnected {
		return deviceStateRefreshMsg{err: fmt.Errorf("not connected")}
	}
	req := ctlplfl.GetReq{GetAll: true}
	m.cpClient.SetToken(m.userToken())
	devices, err := m.cpClient.GetDevices(req)
	if err != nil {
		return deviceStateRefreshMsg{err: err}
	}
	nisds, _ := m.cpClient.GetNisds(req)
	return deviceStateRefreshMsg{devices: devices, nisds: nisds}
}

// syncDeviceStatesFromCP overlays CP device states onto the local config.
// It matches CP devices to local config devices by ID or Name.
func (m *model) syncDeviceStatesFromCP(cpDevices []ctlplfl.Device) {
	stateByID := make(map[string]uint16, len(cpDevices))
	stateByName := make(map[string]uint16, len(cpDevices))
	for _, d := range cpDevices {
		if d.ID != "" {
			stateByID[d.ID] = d.State
		}
		if d.Name != "" {
			stateByName[d.Name] = d.State
		}
	}

	updateDev := func(dev *Device) {
		if s, ok := stateByID[dev.ID]; ok {
			dev.State = s
		} else if s, ok := stateByName[dev.Name]; ok {
			dev.State = s
		}
	}

	for i := range m.cpPDUs {
		for j := range m.cpPDUs[i].Racks {
			for k := range m.cpPDUs[i].Racks[j].Hypervisors {
				for l := range m.cpPDUs[i].Racks[j].Hypervisors[k].Dev {
					updateDev(&m.cpPDUs[i].Racks[j].Hypervisors[k].Dev[l])
				}
			}
		}
	}
	for i := range m.cpHypervisors {
		for j := range m.cpHypervisors[i].Dev {
			updateDev(&m.cpHypervisors[i].Dev[j])
		}
	}
}

// ensureUserClient lazily initializes the user client. Returns an error message
// string if initialization fails.
func (m model) ensureUserClient() (model, error) {
	if m.cpRaftUUID == "" || m.cpGossipPath == "" {
		return m, fmt.Errorf("control plane not configured. Use -cp flag with -raft-uuid and -gossip-path")
	}
	if m.userClient == nil {
		client, teardown := initUserClient(m.cpRaftUUID, m.cpGossipPath, m.logFile)
		if client == nil {
			return m, fmt.Errorf("failed to initialize user client")
		}
		m.userClient = client
		userClientTeardownFn = teardown
	}
	return m, nil
}

// --- User Management Update Handlers ---

func (m model) updateUserCreateForm(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			username := strings.TrimSpace(m.userUsernameInput.Value())
			if username == "" {
				m.message = "Error: Username cannot be empty"
				return m, nil
			}
			if username == userlib.AdminUsername {
				m.state = stateUserCreateAdminKey
				m.userUsernameInput.Blur()
				m.userSecretKeyInput.Focus()
				m.message = ""
				return m, textinput.Blink
			}
			// Regular user creation: use stored token if logged in
			if m.loggedInUser != nil {
				req := &userlib.UserReq{
					Username: username,
				}
				client := m.userClient
				m.userUsernameInput.Blur()
				m.message = "Creating user..."
				return m, func() tea.Msg {
					resp, err := client.CreateUser(m.userToken(), req)
					return userCreatedMsg{resp: resp, err: err}
				}
			}
			// Not logged in — prompt for manual token
			m.state = stateUserCreateToken
			m.userUsernameInput.Blur()
			m.userTokenInput.Focus()
			m.message = ""
			return m, textinput.Blink
		}
	}
	var cmd tea.Cmd
	m.userUsernameInput, cmd = m.userUsernameInput.Update(msg)
	return m, cmd
}

func (m model) updateUserCreateAdminKey(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			req := &userlib.UserReq{
				Username:     userlib.AdminUsername,
				NewSecretKey: strings.TrimSpace(m.userSecretKeyInput.Value()),
			}
			client := m.userClient
			m.userSecretKeyInput.Blur()
			m.message = "Creating admin user..."
			return m, func() tea.Msg {
				resp, err := client.CreateAdminUser(req)
				return userCreatedMsg{resp: resp, err: err}
			}
		}
	}
	var cmd tea.Cmd
	m.userSecretKeyInput, cmd = m.userSecretKeyInput.Update(msg)
	return m, cmd
}

func (m model) updateUserCreateToken(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			token := strings.TrimSpace(m.userTokenInput.Value())
			if token == "" {
				m.message = "Error: Admin JWT token is required"
				return m, nil
			}
			username := strings.TrimSpace(m.userUsernameInput.Value())
			req := &userlib.UserReq{
				Username: username,
			}
			client := m.userClient
			m.userTokenInput.Blur()
			m.message = "Creating user..."
			return m, func() tea.Msg {
				resp, err := client.CreateUser(token, req)
				return userCreatedMsg{resp: resp, err: err}
			}
		}
	}
	var cmd tea.Cmd
	m.userTokenInput, cmd = m.userTokenInput.Update(msg)
	return m, cmd
}

func (m model) updateUserCreateResult(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc":
			m.state = stateMenu
			m.userCreateResp = nil
			m.userCreateErr = ""
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

// --- User Management View Functions ---

func (m model) viewUserCreateForm() string {
	title := titleStyle.Render("Create User")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	s.WriteString("Enter username:\n\n")
	s.WriteString(m.userUsernameInput.View() + "\n\n")
	s.WriteString(helpStyle.Render("Tip: Enter 'admin' to create the bootstrap admin user (no token needed).") + "\n")
	s.WriteString(helpStyle.Render("     Any other username requires an admin JWT token.") + "\n\n")
	s.WriteString(helpStyle.Render("enter: continue • esc: back to menu"))

	return s.String()
}

func (m model) viewUserCreateAdminKey() string {
	title := titleStyle.Render("Create Admin User")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	s.WriteString(fmt.Sprintf("Username: %s\n\n", selectedStyle.Render(userlib.AdminUsername)))
	s.WriteString("Enter secret key (leave empty to auto-generate):\n\n")
	s.WriteString(m.userSecretKeyInput.View() + "\n\n")
	s.WriteString(helpStyle.Render("enter: create admin user • esc: back to menu"))

	return s.String()
}

func (m model) viewUserCreateToken() string {
	title := titleStyle.Render("Create User")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	username := strings.TrimSpace(m.userUsernameInput.Value())
	s.WriteString(fmt.Sprintf("Username: %s\n\n", selectedStyle.Render(username)))
	s.WriteString("Enter admin JWT token:\n\n")
	s.WriteString(m.userTokenInput.View() + "\n\n")
	s.WriteString(helpStyle.Render("enter: create user • esc: back to menu"))

	return s.String()
}

func (m model) viewUserCreateResult() string {
	title := titleStyle.Render("Create User — Result")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.userCreateErr != "" {
		s.WriteString(errorStyle.Render("Error: "+m.userCreateErr) + "\n\n")
		s.WriteString(helpStyle.Render("enter/esc: back to menu"))
		return s.String()
	}

	if m.userCreateResp != nil {
		s.WriteString(successStyle.Render("User Created Successfully") + "\n\n")
		s.WriteString("User Details:\n")
		s.WriteString(fmt.Sprintf("  Username:   %s\n", m.userCreateResp.Username))
		s.WriteString(fmt.Sprintf("  UserID:     %s\n", m.userCreateResp.UserID))
		s.WriteString(fmt.Sprintf("  Role:       %s\n", m.userCreateResp.UserRole))
		s.WriteString(fmt.Sprintf("  Status:     %s\n\n", m.userCreateResp.Status))

		secretKeyBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#01BE85")).
			Padding(0, 2).
			Render(fmt.Sprintf("Secret Key: %s", m.userCreateResp.SecretKey))
		s.WriteString(secretKeyBox + "\n\n")

		s.WriteString(helpStyle.Render("Copy the secret key above — it is needed for login.") + "\n")
		s.WriteString(helpStyle.Render("This is the only time the secret key will be displayed.") + "\n\n")
	}

	s.WriteString(helpStyle.Render("enter/esc: back to menu"))
	return s.String()
}

// --- User Login Update Handlers ---

func (m model) updateUserLoginForm(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			username := strings.TrimSpace(m.userUsernameInput.Value())
			if username == "" {
				m.message = "Error: Username cannot be empty"
				return m, nil
			}
			m.state = stateUserLoginSecret
			m.userUsernameInput.Blur()
			m.userSecretKeyInput.SetValue("")
			m.userSecretKeyInput.Focus()
			m.message = ""
			return m, textinput.Blink
		}
	}
	var cmd tea.Cmd
	m.userUsernameInput, cmd = m.userUsernameInput.Update(msg)
	return m, cmd
}

func (m model) updateUserLoginSecret(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			secretKey := strings.TrimSpace(m.userSecretKeyInput.Value())
			if secretKey == "" {
				m.message = "Error: Secret key cannot be empty"
				return m, nil
			}
			username := strings.TrimSpace(m.userUsernameInput.Value())
			client := m.userClient
			m.userSecretKeyInput.Blur()
			m.message = "Logging in..."
			return m, func() tea.Msg {
				resp, err := client.Login(username, secretKey)
				return userLoggedInMsg{resp: resp, err: err}
			}
		}
	}
	var cmd tea.Cmd
	m.userSecretKeyInput, cmd = m.userSecretKeyInput.Update(msg)
	return m, cmd
}

func (m model) updateUserLoginResult(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc":
			m.state = stateMenu
			m.userLoginErr = ""
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

// --- User Login View Functions ---

func (m model) viewUserLoginForm() string {
	title := titleStyle.Render("Login")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	s.WriteString("Enter username:\n\n")
	s.WriteString(m.userUsernameInput.View() + "\n\n")
	s.WriteString(helpStyle.Render("enter: continue • esc: back to menu"))

	return s.String()
}

func (m model) viewUserLoginSecret() string {
	title := titleStyle.Render("Login")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	username := strings.TrimSpace(m.userUsernameInput.Value())
	s.WriteString(fmt.Sprintf("Username: %s\n\n", selectedStyle.Render(username)))
	s.WriteString("Enter secret key:\n\n")
	s.WriteString(m.userSecretKeyInput.View() + "\n\n")
	s.WriteString(helpStyle.Render("enter: login • esc: back to menu"))

	return s.String()
}

func (m model) viewUserLoginResult() string {
	title := titleStyle.Render("Login — Error")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.userLoginErr != "" {
		s.WriteString(errorStyle.Render("Error: "+m.userLoginErr) + "\n\n")
	}

	s.WriteString(helpStyle.Render("enter/esc: back to menu"))
	return s.String()
}

func (m model) updateHypervisorForm(msg tea.Msg) (model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "i", "I": // Enter IP management mode
			if !m.ipManagementMode {
				m.ipManagementMode = true
				m.ipFocusedIndex = 0
				// Focus first IP input
				if len(m.ipInputs) > 0 {
					cmds = append(cmds, m.ipInputs[0].Focus())
				}
				// Blur all regular inputs
				for i := range m.inputs {
					m.inputs[i].Blur()
				}
				return m, tea.Batch(cmds...)
			}

		case "f2", "F2": // Submit form (when in IP mode)
			if m.ipManagementMode {
				return m.submitHypervisorForm()
			}

		case "enter":
			if m.ipManagementMode {
				// Handle IP management actions
				if m.ipFocusedIndex == len(m.ipInputs) {
					// Add new IP (only when focused on "Add IP Address" button)
					newInput := createIPInput()
					m.ipInputs = append(m.ipInputs, newInput)
					m.ipFocusedIndex = len(m.ipInputs) - 1
					// Ensure the new input is properly focused
					cmds = append(cmds, m.ipInputs[m.ipFocusedIndex].Focus())
					return m, tea.Batch(cmds...)
				} else {
					// When typing in an IP field, Enter should just move to next field (like Tab)
					// This provides natural form navigation behavior
					return m.updateHypervisorForm(tea.KeyMsg{Type: tea.KeyTab})
				}
			} else {
				// Submit form normally
				return m.submitHypervisorForm()
			}

		case "tab", "shift+tab", "up", "down":
			// Unified navigation for all fields (basic + IP section)
			s := msg.String()
			basicInputs := []int{0, 3, 4} // name, ssh port, port range

			// Current position determination
			var currentPos int
			if m.ipManagementMode {
				// In IP section: position = 3 + ipFocusedIndex
				currentPos = len(basicInputs) + m.ipFocusedIndex
			} else {
				// In basic fields: find position in basicInputs
				currentPos = -1
				for i, inputIdx := range basicInputs {
					if inputIdx == int(m.focusedInput) {
						currentPos = i
						break
					}
				}
				if currentPos == -1 {
					currentPos = 0 // default to first field
				}
			}

			// Total navigation positions
			totalPositions := len(basicInputs) + len(m.ipInputs) + 1 // +1 for "Add IP" button

			// Calculate next position
			var nextPos int
			if s == "up" || s == "shift+tab" {
				nextPos = currentPos - 1
				if nextPos < 0 {
					nextPos = totalPositions - 1
				}
			} else {
				nextPos = currentPos + 1
				if nextPos >= totalPositions {
					nextPos = 0
				}
			}

			// Handle transition to new position
			if nextPos < len(basicInputs) {
				// Moving to basic field
				if m.ipManagementMode {
					// Exit IP management mode
					m.ipManagementMode = false
					for i := range m.ipInputs {
						m.ipInputs[i].Blur()
					}
				}

				m.focusedInput = inputField(basicInputs[nextPos])

				// Focus/blur basic inputs
				for i := range m.inputs {
					if i == int(m.focusedInput) {
						cmds = append(cmds, m.inputs[i].Focus())
					} else {
						m.inputs[i].Blur()
					}
				}
			} else {
				// Moving to IP section
				if !m.ipManagementMode {
					// Enter IP management mode
					m.ipManagementMode = true
					// Blur all basic inputs
					for i := range m.inputs {
						m.inputs[i].Blur()
					}
				}

				m.ipFocusedIndex = nextPos - len(basicInputs)

				// Ensure ipFocusedIndex is within bounds
				maxIPIndex := len(m.ipInputs) // "Add IP" button is at len(m.ipInputs)
				if m.ipFocusedIndex > maxIPIndex {
					m.ipFocusedIndex = maxIPIndex
				} else if m.ipFocusedIndex < 0 {
					m.ipFocusedIndex = 0
				}

				// Focus appropriate IP input or handle Add IP button
				if m.ipFocusedIndex < len(m.ipInputs) {
					// Focus the specific IP input
					for i := range m.ipInputs {
						if i == m.ipFocusedIndex {
							cmds = append(cmds, m.ipInputs[i].Focus())
						} else {
							m.ipInputs[i].Blur()
						}
					}
				} else {
					// On "Add IP" button - blur all IP inputs
					for i := range m.ipInputs {
						m.ipInputs[i].Blur()
					}
				}
			}

			return m, tea.Batch(cmds...)

		case "delete", "backspace":
			if m.ipManagementMode && m.ipFocusedIndex > 0 && len(m.ipInputs) > 1 {
				// Delete the current IP (only for additional IPs, not primary)
				m.ipInputs = append(m.ipInputs[:m.ipFocusedIndex], m.ipInputs[m.ipFocusedIndex+1:]...)
				if m.ipFocusedIndex >= len(m.ipInputs) {
					m.ipFocusedIndex = len(m.ipInputs) - 1
				}
				if m.ipFocusedIndex >= 0 && m.ipFocusedIndex < len(m.ipInputs) {
					cmds = append(cmds, m.ipInputs[m.ipFocusedIndex].Focus())
				}
				return m, tea.Batch(cmds...)
			}

		case "esc":
			if m.ipManagementMode {
				// Exit IP management mode
				m.ipManagementMode = false
				m.ipFocusedIndex = 0
				// Blur IP inputs
				for i := range m.ipInputs {
					m.ipInputs[i].Blur()
				}
				// Focus back to regular form
				cmds = append(cmds, m.inputs[int(m.focusedInput)].Focus())
				return m, tea.Batch(cmds...)
			}

		case "r", "R": // Open Rack selection menu
			allRacks := m.getAllRacks()
			if len(allRacks) > 0 {
				m.rackSelectionCursor = m.selectedRackIdx // Start at currently selected rack
				if m.rackSelectionCursor < 0 {
					m.rackSelectionCursor = 0
				}
				m.state = stateHypervisorRackSelection
				return m, nil
			}
		}
	}

	// Update the focused input
	var cmd tea.Cmd
	if m.ipManagementMode {
		// Update focused IP input
		if m.ipFocusedIndex >= 0 && m.ipFocusedIndex < len(m.ipInputs) {
			m.ipInputs[m.ipFocusedIndex], cmd = m.ipInputs[m.ipFocusedIndex].Update(msg)
		}
	} else {
		// Update focused regular input
		m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)
	}

	return m, cmd
}

// Helper function to submit the hypervisor form
func (m model) submitHypervisorForm() (model, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[0].Value())
	sshPort := strings.TrimSpace(m.inputs[3].Value())
	portRange := strings.TrimSpace(m.inputs[4].Value())

	if name == "" {
		m.message = "Name is required"
		return m, nil
	}

	// Collect and validate all IP addresses
	var allIPs []string
	for i, ipInput := range m.ipInputs {
		ip := strings.TrimSpace(ipInput.Value())
		if ip == "" {
			if i == 0 {
				m.message = "Primary IP address is required"
				return m, nil
			}
			continue // Skip empty additional IPs
		}

		if !isValidIP(ip) {
			ipLabel := "Primary IP"
			if i > 0 {
				ipLabel = fmt.Sprintf("IP Address %d", i+1)
			}
			m.message = fmt.Sprintf("Invalid %s format: %s", ipLabel, ip)
			return m, nil
		}
		allIPs = append(allIPs, ip)
	}

	if len(allIPs) == 0 {
		m.message = "At least one IP address is required"
		return m, nil
	}

	// Set default SSH port if not specified
	if sshPort == "" {
		sshPort = "22"
	}

	// Get the selected rack from getAllRacks() using selectedRackIdx
	allRacks := m.getAllRacks()
	if m.selectedRackIdx < 0 || m.selectedRackIdx >= len(allRacks) {
		m.message = "⚠️  Invalid rack selection. Please select a rack first."
		return m, nil
	}

	selectedRackInfo := allRacks[m.selectedRackIdx]
	hvID := m.editingUUID
	if hvID == "" {
		hvID = uuid.New().String()
	}
	hypervisor := Hypervisor{
		ID:        hvID,
		Name:      name,
		RackID:    selectedRackInfo.Rack.ID, // Use the correct Rack UUID
		IPAddrs:   allIPs,                   // Multiple IP addresses
		SSHPort:   sshPort,
		PortRange: portRange,
	}

	// Store hypervisor data but don't add to config/PumiceDB yet
	// Device discovery and selection will happen first
	m.currentHv = hypervisor
	log.Info("m.currentHv: ", m.currentHv)
	m.state = stateDeviceDiscovery
	m.loading = true
	m.message = ""
	return m, m.discoverDevices()
}

func (m model) updateDeviceSelection(msg tea.Msg) (model, tea.Cmd) {
	totalPages := (len(m.discoveredDevs) + m.devicesPerPage - 1) / m.devicesPerPage
	devicesOnCurrentPage := m.getDevicesOnCurrentPage()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.deviceCursor > 0 {
				m.deviceCursor--
			} else if m.devicePage > 0 {
				// Go to previous page, cursor at bottom
				m.devicePage--
				devicesOnPrevPage := m.getDevicesOnCurrentPage()
				m.deviceCursor = len(devicesOnPrevPage) - 1
			}
		case "down", "j":
			if m.deviceCursor < len(devicesOnCurrentPage)-1 {
				m.deviceCursor++
			} else if m.devicePage < totalPages-1 {
				// Go to next page, cursor at top
				m.devicePage++
				m.deviceCursor = 0
			}
		case "left", "h":
			// Previous page
			if m.devicePage > 0 {
				m.devicePage--
				m.deviceCursor = 0
			}
		case "right", "l":
			// Next page
			if m.devicePage < totalPages-1 {
				m.devicePage++
				m.deviceCursor = 0
			}
		case " ":
			// Toggle selection of current device
			actualIndex := m.devicePage*m.devicesPerPage + m.deviceCursor
			if actualIndex < len(m.discoveredDevs) {
				m.selectedDevices[actualIndex] = !m.selectedDevices[actualIndex]
			}
		case "a":
			// Select all devices (all pages)
			for i := range m.discoveredDevs {
				m.selectedDevices[i] = true
			}
		case "n":
			// Select none (all pages)
			m.selectedDevices = make(map[int]bool)
		case "enter":
			// Confirm selection - devices will be included in hypervisor
			selectedDevices := make([]Device, 0)
			for i, selected := range m.selectedDevices {
				if selected && i < len(m.discoveredDevs) {
					selDev := m.discoveredDevs[i]
					log.Info("Discovered device: ", selDev)
					selDev.HypervisorID = m.currentHv.ID
					selDev.State = ctlplfl.INITIALIZED
					selectedDevices = append(selectedDevices, selDev)
				}
			}
			// Assign selected devices to hypervisor before adding to config/PumiceDB
			m.currentHv.Dev = selectedDevices
			log.Info("currentHv.Dev: ", m.currentHv.Dev)

			// Now add hypervisor with devices to config and PumiceDB
			allRacks := m.getAllRacks()
			if m.selectedRackIdx >= 0 && m.selectedRackIdx < len(allRacks) {
				log.Info("Adding Hypervisor with devices: ", m.currentHv)
				// Add the hypervisor with devices to pumiceDB
				m.cpClient.SetToken(m.userToken())
				hvResp, err := m.cpClient.PutHypervisor(&m.currentHv)
				if err != nil {
					m.message = fmt.Sprintf("Failed to add hypervisor to pumiceDB: %v", err)
					return m, nil
				}
				if hvResp != nil && !hvResp.Success {
					m.message = fmt.Sprintf("Failed to add hypervisor: %s", hvResp.Error)
					return m, nil
				}
				// Immediately initialize devices in the control plane
				deviceErrors := 0
				for _, dev := range selectedDevices {
					deviceInfo := Device{
						ID:            dev.ID,
						Name:          dev.Name,
						DevicePath:    dev.DevicePath,
						SerialNumber:  dev.SerialNumber,
						State:         ctlplfl.INITIALIZED,
						Size:          dev.Size,
						HypervisorID:  m.currentHv.ID,
						FailureDomain: dev.FailureDomain,
					}
					validateDeviceInfo(&deviceInfo)
					m.cpClient.SetToken(m.userToken())
					devResp, err := m.cpClient.PutDevice(&deviceInfo)
					if err != nil {
						log.Warn("Failed to initialize device in control plane: ", err)
						deviceErrors++
					} else if devResp != nil && !devResp.Success {
						log.Warn("Control plane rejected device init: ", devResp.Error)
						deviceErrors++
					}
				}
				m.refreshCPData()
				if deviceErrors > 0 {
					m.message = fmt.Sprintf("Added hypervisor %s with %d devices (%d device(s) failed to initialize)", m.currentHv.ID, len(selectedDevices), deviceErrors)
				} else {
					m.message = fmt.Sprintf("Added hypervisor %s with %d devices", m.currentHv.ID, len(selectedDevices))
				}
			}
			m.discoveredDevs = nil
			m.state = stateShowAddedHypervisor
			return m, nil
		}
	}

	return m, nil
}

func (m model) updateHypervisorManagement(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.hvManagementCursor > 0 {
				m.hvManagementCursor--
			}
		case "down", "j":
			allHypervisors := m.getAllHypervisors()
			maxItems := 4 // Add, View, Edit, Delete
			if len(allHypervisors) == 0 {
				maxItems = 1 // Only Add available
			}
			if m.hvManagementCursor < maxItems-1 {
				m.hvManagementCursor++
			}
		case "enter", " ":
			switch m.hvManagementCursor {
			case 0: // Add Hypervisor
				// Check if racks are available
				m.refreshCPData()
				allRacks := m.getAllRacks()
				if len(allRacks) == 0 {
					m.message = "⚠️  No racks available. Please create racks first before adding hypervisors."
					return m, nil
				}

				// Check if a rack is selected
				if m.selectedRackIdx < 0 || m.selectedRackIdx >= len(allRacks) {
					m.message = "⚠️  Please select a rack first. Use 'R' to select a rack for the hypervisor."
					return m, nil
				}

				m.state = stateHypervisorForm
				m.editingUUID = "" // Clear editing UUID for new hypervisor
				m = m.resetInputs()
				m.message = ""
				return m, textinput.Blink
			case 1: // View Hypervisor
				// Trigger hypervisor refresh from Control Plane when entering view
				m.cpHypervisorRefresh = true
				m.state = stateViewHypervisor
				m.hvListCursor = 0
				m.message = ""
				return m, nil
			case 2: // Edit Hypervisor
				allHypervisors := m.getAllHypervisors()
				if len(allHypervisors) > 0 {
					m.state = stateEditHypervisor
					m.hvListCursor = 0
					m.message = ""
					return m, nil
				}
			case 3: // Delete Hypervisor
				allHypervisors := m.getAllHypervisors()
				if len(allHypervisors) > 0 {
					m.state = stateDeleteHypervisor
					m.hvListCursor = 0
					m.message = ""
					return m, nil
				}
			}
		case "r", "R": // Open Rack selection menu
			allRacks := m.getAllRacks()
			if len(allRacks) > 0 {
				m.rackSelectionCursor = m.selectedRackIdx // Start at currently selected rack
				if m.rackSelectionCursor < 0 {
					m.rackSelectionCursor = 0
				}
				m.state = stateHypervisorRackSelection
				return m, nil
			} else {
				m.message = "⚠️  No racks available. Please create racks first."
				return m, nil
			}
		}
	}
	return m, nil
}

func (m model) updateEditHypervisor(msg tea.Msg) (model, tea.Cmd) {
	allHypervisors := m.getAllHypervisors()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.hvListCursor > 0 {
				m.hvListCursor--
			}
		case "down", "j":
			if m.hvListCursor < len(allHypervisors)-1 {
				m.hvListCursor++
			}
		case "enter", " ":
			// Load selected hypervisor for editing
			if m.hvListCursor < len(allHypervisors) {
				hvInfo := allHypervisors[m.hvListCursor]
				hv := hvInfo.Hypervisor
				m.editingUUID = hv.ID
				m.currentHv = hv
				m.loadHypervisorIntoForm(hv)
				m.state = stateHypervisorForm
				return m, textinput.Blink
			}
		}
	}
	return m, nil
}

func (m model) updateDeleteHypervisor(msg tea.Msg) (model, tea.Cmd) {
	allHypervisors := m.getAllHypervisors()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.hvListCursor > 0 {
				m.hvListCursor--
			}
		case "down", "j":
			if m.hvListCursor < len(allHypervisors)-1 {
				m.hvListCursor++
			}
		case "enter", " ", "y":
			// Delete selected hypervisor
			if m.hvListCursor < len(allHypervisors) {
				m.message = "Hypervisor deletion is not supported via the control plane API"
			}
			m.state = stateHypervisorManagement
			return m, nil
		case "n":
			// Cancel deletion
			m.state = stateHypervisorManagement
			m.message = "Deletion cancelled"
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateShowAddedHypervisor(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ", "esc":
			// Return to Hypervisor management
			m.state = stateHypervisorManagement
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateDeviceManagement(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.deviceMgmtCursor > 0 {
				m.deviceMgmtCursor--
			}
		case "down", "j":
			maxItems := 1 // View Device
			if m.deviceMgmtCursor < maxItems-1 {
				m.deviceMgmtCursor++
			}
		case "enter", " ":
			switch m.deviceMgmtCursor {
			case 0: // View Device
				m.state = stateDeviceView
				m.selectedHypervisorIdx = -1
				m.selectedDeviceIdx = -1
				m.message = ""
				return m, m.fetchDeviceStatesFromCP
			}
		}
	}
	return m, nil
}

func (m model) updateDeviceView(msg tea.Msg) (model, tea.Cmd) {
	// Get all devices across all hypervisors
	allDevices := m.getAllDevicesForPartitioning()

	// Sort devices for consistent ordering (same as in view function)
	sort.Slice(allDevices, func(i, j int) bool {
		// First sort by hypervisor name, then by device ID
		if allDevices[i].HvName != allDevices[j].HvName {
			return allDevices[i].HvName < allDevices[j].HvName
		}
		return allDevices[i].Device.ID < allDevices[j].Device.ID
	})

	switch msg := msg.(type) {
	case deviceStateRefreshMsg:
		if msg.err == nil && len(msg.devices) > 0 {
			m.syncDeviceStatesFromCP(msg.devices)
		}
		if len(msg.nisds) > 0 {
			if m.nisdCache == nil {
				m.nisdCache = make(map[string]ctlplfl.Nisd, len(msg.nisds))
			}
			for _, n := range msg.nisds {
				m.nisdCache[n.ID] = n
			}
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedDeviceIdx > 0 {
				m.selectedDeviceIdx--
			}
		case "down", "j":
			if len(allDevices) > 0 && m.selectedDeviceIdx < len(allDevices)-1 {
				m.selectedDeviceIdx++
			}
		case "esc":
			m.state = stateDeviceManagement
			m.selectedDeviceIdx = -1
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateDeviceDelete(msg tea.Msg) (model, tea.Cmd) {
	// Get all initialized devices across all hypervisors
	initializedDevices := m.getAllInitializedDevices()

	if len(initializedDevices) == 0 {
		m.message = "No initialized devices found."
		m.state = stateDeviceManagement
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedDeviceIdx > 0 {
				m.selectedDeviceIdx--
			}
		case "down", "j":
			if m.selectedDeviceIdx < len(initializedDevices)-1 {
				m.selectedDeviceIdx++
			}
		case "enter", " ", "y":
			// Delete selected device initialization
			if m.selectedDeviceIdx >= 0 && m.selectedDeviceIdx < len(initializedDevices) {
				deviceInfo := initializedDevices[m.selectedDeviceIdx]
				m.message = fmt.Sprintf("Device '%s' initialization removed (refresh from CP to confirm)", deviceInfo.Device.ID)
				m.state = stateDeviceManagement
				return m, nil
			}
		case "n":
			// Cancel deletion
			m.state = stateDeviceManagement
			m.message = "Deletion cancelled"
			return m, nil
		}
	}
	return m, nil
}

/*
func (m model) updateDeviceInitialization(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			failureDomain := strings.TrimSpace(m.deviceFailureDomain.Value())
			if failureDomain == "" {
				m.message = "Failure domain is required for device initialization"
				return m, nil
			}

			if m.selectedHypervisorIdx < 0 || m.selectedHypervisorIdx >= len(m.cpHypervisors) {
				m.message = "No hypervisor selected"
				return m, nil
			}
			hv := m.cpHypervisors[m.selectedHypervisorIdx]
			if m.selectedDeviceIdx < 0 || m.selectedDeviceIdx >= len(hv.Dev) {
				m.message = "No device selected"
				return m, nil
			}
			device := hv.Dev[m.selectedDeviceIdx]

			m.message = fmt.Sprintf("Device %s initialized successfully", device.ID)

			// Send to control plane if available
			if m.cpClient != nil {
				deviceInfo := ctlplfl.Device{
					ID:            device.ID,
					SerialNumber:  device.SerialNumber,
					State:         ctlplfl.INITIALIZED,
					HypervisorID:  hv.ID,
					FailureDomain: failureDomain,
				}
				log.Info("Put device info: ", deviceInfo)

				devResp, cpErr := m.cpClient.PutDevice(&deviceInfo)
				if cpErr != nil {
					log.Warn("Failed to sync device to control plane: ", cpErr)
					m.message += " (Control plane sync failed)"
				} else if devResp != nil && !devResp.Success {
					log.Warn("Control plane rejected device sync: ", devResp.Error)
					m.message += fmt.Sprintf(" (Control plane: %s)", devResp.Error)
				} else {
					log.Info("Successfully synced device to control plane")
					m.message += " and synced to control plane"
					m.refreshCPData()
				}
			} else {
				log.Error("Failed to write DeviceInfo in CP")
			}

			m.state = stateDeviceManagement
			m.selectedDeviceIdx = -1
			return m, nil
		}
	}

	m.deviceFailureDomain, cmd = m.deviceFailureDomain.Update(msg)
	return m, cmd
}
*/

// DeviceInfo holds device and its location in the config
type DeviceInfo struct {
	Device   ctlplfl.Device
	HvIndex  int
	DevIndex int
}

// TreeItem represents a navigable item in the hierarchical configuration tree
type TreeItem struct {
	Type       string // "pdu", "rack", "hypervisor", "device"
	Index      int    // For PDUs and legacy hypervisors
	UUID       string // For racks, hypervisors in racks, and devices
	ParentUUID string // For nested items
	Name       string // Display name
	Level      int    // Indentation level
}

func (m model) getAllInitializedDevices() []DeviceInfo {
	var devices []DeviceInfo
	for hvIndex, hv := range m.cpHypervisors {
		for devIndex, device := range hv.Dev {
			devices = append(devices, DeviceInfo{
				Device:   device,
				HvIndex:  hvIndex,
				DevIndex: devIndex,
			})
		}
	}
	return devices
}

// PartitionInfo holds information about a partition along with its context
type PartitionInfo struct {
	Partition  DevicePartition
	HvUUID     string
	HvName     string
	DeviceName string
}

// Helper function to get all partitions across all devices
func (m model) getAllPartitionsAcrossDevices() []PartitionInfo {
	var allPartitions []PartitionInfo

	devices := m.getAllDevicesForPartitioning()
	for _, deviceInfo := range devices {
		for _, partition := range deviceInfo.Device.Partitions {
			allPartitions = append(allPartitions, PartitionInfo{
				Partition:  partition,
				HvUUID:     deviceInfo.HvUUID,
				HvName:     deviceInfo.HvName,
				DeviceName: deviceInfo.Device.Name,
			})
		}
	}

	return allPartitions
}

// Helper function to calculate optimal items per page based on terminal height
func (m model) calculateItemsPerPage(linesPerItem int) int {
	// Reserve space for:
	// - Title (2 lines)
	// - Pagination info (2 lines)
	// - Help text (2 lines)
	// - Some safety margin (4 lines)
	reservedLines := 10

	// Default to conservative fallback if terminal height is not available
	if m.termHeight <= 0 {
		return 5 // Conservative default when terminal size unknown
	}

	availableLines := m.termHeight - reservedLines

	// Ensure we have some minimum space to work with
	if availableLines < 3*linesPerItem {
		return 3 // Minimum usable items regardless of screen size
	}

	itemsPerPage := availableLines / linesPerItem

	// No arbitrary maximum - let screen size be the natural limit
	// The calculation already ensures items fit within available space
	return itemsPerPage
}

// Helper function to calculate optimal items per page with custom header accounting
func (m model) calculateItemsPerPageWithHeader(linesPerItem int, headerLines int) int {
	// Reserve space for:
	// - Custom header (headerLines)
	// - Pagination info (2 lines)
	// - Help text (2 lines)
	// - Some safety margin (3 lines)
	reservedLines := headerLines + 7

	// Default to conservative fallback if terminal height is not available
	if m.termHeight <= 0 {
		return 7 // Conservative default when terminal size unknown
	}

	availableLines := m.termHeight - reservedLines

	// Ensure we have some minimum space to work with
	if availableLines < 3*linesPerItem {
		return 3 // Minimum usable items regardless of screen size
	}

	// Calculate base items per page using a more conservative estimate
	// Account for potential expanded items by using 2 lines per item estimate
	itemsPerPage := availableLines / 2

	return itemsPerPage
}

// Helper function to format bytes into human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp+1])
}

// Helper function to get NISDs list from control plane with error handling
func (m model) getNISDsList() ([]ctlplfl.Nisd, error) {
	if m.cpClient == nil {
		return nil, fmt.Errorf("control plane client is not initialized")
	}
	return m.cpClient.GetNisds(ctlplfl.GetReq{})
}

// Helper function to get partition name for NISD display
// Attempts to find the partition name by looking up device partitions
func (m model) getNISDPartitionName(nisd ctlplfl.Nisd) string {
	deviceID := nisd.FailureDomain[ctlplfl.DEVICE_IDX]
	hypervisorID := nisd.FailureDomain[ctlplfl.HV_IDX]

	// Look through all devices to find the partition that matches this NISD
	allDevices := m.getAllDevicesForPartitioning()
	for _, deviceInfo := range allDevices {
		if deviceInfo.Device.ID == deviceID && deviceInfo.HvUUID == hypervisorID {
			// Find partition with matching NISD UUID
			for _, partition := range deviceInfo.Device.Partitions {
				if partition.NISDUUID == nisd.ID {
					// Return partition ID which is the partition name
					return partition.PartitionID
				}
			}
		}
	}

	// If no partition found, generate a fallback name based on device
	if deviceID != "" {
		return fmt.Sprintf("partition-on-%s", deviceID)
	}

	// Ultimate fallback
	return fmt.Sprintf("partition-%s", nisd.ID[0:8])
}

// NISDStats holds statistics about NISDs for display
type NISDStats struct {
	TotalNISDs     int
	UniqueHVs      int
	UniqueDevices  int
	TotalSize      int64
	TotalAvailable int64
	RunningNISDs   int
}

// Helper function to calculate NISD statistics
func (m model) calculateNISDStats(nisds []ctlplfl.Nisd) NISDStats {
	stats := NISDStats{}

	if len(nisds) == 0 {
		return stats
	}

	hvSet := make(map[string]bool)
	deviceSet := make(map[string]bool)

	stats.TotalNISDs = len(nisds)

	for _, nisd := range nisds {
		// Count unique hypervisors
		hvID := nisd.FailureDomain[ctlplfl.HV_IDX]
		if hvID != "" {
			hvSet[hvID] = true
		}

		// Count unique devices
		deviceID := nisd.FailureDomain[ctlplfl.DEVICE_IDX]
		if deviceID != "" {
			deviceSet[deviceID] = true
		}

		// Sum up sizes
		stats.TotalSize += nisd.TotalSize
		stats.TotalAvailable += nisd.AvailableSize

		// Count running NISDs (assume all returned NISDs are running)
		// In a real implementation, this would check NISD status
		stats.RunningNISDs++
	}

	stats.UniqueHVs = len(hvSet)
	stats.UniqueDevices = len(deviceSet)

	return stats
}

// Helper function to get devices for the current page
func (m model) getDevicesOnCurrentPage() []ctlplfl.Device {
	start := m.devicePage * m.devicesPerPage
	end := start + m.devicesPerPage

	if start >= len(m.discoveredDevs) {
		return []ctlplfl.Device{}
	}

	if end > len(m.discoveredDevs) {
		end = len(m.discoveredDevs)
	}

	return m.discoveredDevs[start:end]
}

func (m model) updateViewConfig(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.configCursor > 0 {
				m.configCursor--
			}
		case "down", "j":
			treeItems := m.buildTreeItemList()
			if m.configCursor < len(treeItems)-1 {
				m.configCursor++
			}
		case "enter", " ":
			treeItems := m.buildTreeItemList()
			if m.configCursor < len(treeItems) {
				item := treeItems[m.configCursor]
				m.toggleTreeItemExpansion(item)
			}
		case "e":
			// Expand all items
			for i := range m.cpPDUs {
				m.expandedPDUs[i] = true
				for _, rack := range m.cpPDUs[i].Racks {
					m.expandedRacks[rack.ID] = true
					for _, hv := range rack.Hypervisors {
						m.expandedHypers[hv.ID] = true
						for _, device := range hv.Dev {
							m.expandedDevices[fmt.Sprintf("%s-%s", hv.ID, device.ID)] = true
						}
					}
				}
			}
			for i := range m.cpHypervisors {
				m.expandedHvs[i] = true
			}
		case "c":
			// Collapse all items
			m.expandedPDUs = make(map[int]bool)
			m.expandedRacks = make(map[string]bool)
			m.expandedHypers = make(map[string]bool)
			m.expandedDevices = make(map[string]bool)
			m.expandedHvs = make(map[int]bool)
		case "esc":
			m.state = stateMenu
			m.message = "" // Clear debug messages when returning to menu
			return m, nil
		}
	}

	return m, nil
}

// buildTreeItemList creates a flat list of all visible tree items for navigation
func (m model) buildTreeItemList() []TreeItem {
	var items []TreeItem

	// PDU hierarchy
	for pduIdx, pdu := range m.cpPDUs {
		// Add PDU
		items = append(items, TreeItem{
			Type:  "pdu",
			Index: pduIdx,
			Name:  pdu.Name,
			Level: 0,
		})

		// Add racks if PDU is expanded
		if m.expandedPDUs[pduIdx] {
			for _, rack := range pdu.Racks {
				items = append(items, TreeItem{
					Type:  "rack",
					UUID:  rack.ID,
					Level: 1,
				})

				// Add hypervisors if rack is expanded
				if m.expandedRacks[rack.ID] {
					for _, hv := range rack.Hypervisors {
						items = append(items, TreeItem{
							Type:       "hypervisor",
							UUID:       hv.ID,
							ParentUUID: rack.ID,
							Level:      2,
						})

						// Add devices if hypervisor is expanded
						if m.expandedHypers[hv.ID] {
							for _, device := range hv.Dev {
								deviceUUID := fmt.Sprintf("%s-%s", hv.ID, device.ID)
								items = append(items, TreeItem{
									Type:       "device",
									UUID:       deviceUUID,
									ParentUUID: hv.ID,
									Level:      3,
								})

								// Add partitions if device is expanded
								if m.expandedDevices[deviceUUID] {
									for _, partition := range device.Partitions {
										items = append(items, TreeItem{
											Type:       "partition",
											UUID:       partition.PartitionID,
											ParentUUID: deviceUUID,
											Name:       partition.NISDUUID,
											Level:      4,
										})
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Standalone hypervisors (not in any rack)
	for hvIdx, hv := range m.cpHypervisors {
		items = append(items, TreeItem{
			Type:  "legacy_hypervisor",
			Index: hvIdx,
			UUID:  hv.ID,
			Level: 0,
		})

		// Add devices if legacy hypervisor is expanded
		if m.expandedHvs[hvIdx] {
			for _, device := range hv.Dev {
				deviceUUID := fmt.Sprintf("%s-%s", hv.ID, device.ID)
				items = append(items, TreeItem{
					Type:       "legacy_device",
					UUID:       deviceUUID,
					ParentUUID: hv.ID,
					Level:      1,
				})

				// Add partitions if legacy device is expanded
				if m.expandedDevices[deviceUUID] {
					for _, partition := range device.Partitions {
						items = append(items, TreeItem{
							Type:       "partition",
							UUID:       partition.PartitionID,
							ParentUUID: deviceUUID,
							Name:       partition.NISDUUID,
							Level:      2,
						})
					}
				}
			}
		}
	}

	return items
}

// toggleTreeItemExpansion toggles the expansion state of a tree item
func (m *model) toggleTreeItemExpansion(item TreeItem) {
	switch item.Type {
	case "pdu":
		m.expandedPDUs[item.Index] = !m.expandedPDUs[item.Index]
	case "rack":
		m.expandedRacks[item.UUID] = !m.expandedRacks[item.UUID]
	case "hypervisor":
		m.expandedHypers[item.UUID] = !m.expandedHypers[item.UUID]
	case "device":
		m.expandedDevices[item.UUID] = !m.expandedDevices[item.UUID]
	case "legacy_hypervisor":
		m.expandedHvs[item.Index] = !m.expandedHvs[item.Index]
	case "legacy_device":
		m.expandedDevices[item.UUID] = !m.expandedDevices[item.UUID]
	case "partition":
		// Partitions are leaf nodes and don't expand
	}
}

func (m model) discoverDevices() tea.Cmd {
	return func() tea.Msg {
		client, err := NewSSHClient(m.currentHv.IPAddrs)
		if err != nil {
			return deviceDiscoveredMsg{err: err}
		}
		defer client.Close()

		devices, err := client.GetDevices()
		return deviceDiscoveredMsg{devices: devices, err: err}
	}
}

func (m model) resetInputs() model {
	for i := range m.inputs {
		m.inputs[i].SetValue("")
		if i == 0 {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	m.focusedInput = inputName
	return m
}

func (m model) loadHypervisorIntoForm(hv ctlplfl.Hypervisor) model {
	// Ensure IP addresses are properly initialized for backward compatibility

	m.inputs[0].SetValue(hv.ID)

	// Load all IP addresses into dynamic IP inputs
	m.ipInputs = nil // Clear existing inputs
	m.ipFocusedIndex = 0
	m.ipManagementMode = false

	// Create IP inputs for all existing IP addresses
	if len(hv.IPAddrs) > 0 {
		for _, ip := range hv.IPAddrs {
			ipInput := createIPInput()
			ipInput.SetValue(ip)
			m.ipInputs = append(m.ipInputs, ipInput)
		}
	} else {
		// Fallback to old field if no IPAddresses
		ipInput := createIPInput()
		ipInput.SetValue(hv.IPAddrs[0])
		m.ipInputs = append(m.ipInputs, ipInput)
	}

	// If no IP addresses at all, add one empty input
	if len(m.ipInputs) == 0 {
		m.ipInputs = append(m.ipInputs, createIPInput())
	}

	//FIXME hardcoded value of sshport
	if hv.SSHPort != "" {
		m.inputs[3].SetValue(hv.SSHPort)
	} else {
		m.inputs[3].SetValue("22")
	}
	m.inputs[4].SetValue(hv.PortRange)
	m.focusedInput = inputName
	m.inputs[0].Focus()
	return m
}

func (m model) updateDeviceList() model {
	items := make([]list.Item, len(m.discoveredDevs))
	for i, dev := range m.discoveredDevs {
		items[i] = deviceItem{
			device:   dev,
			index:    i,
			selected: m.selectedDevices[i],
		}
	}
	m.deviceList.SetItems(items)
	return m
}

func (m model) updateConfigView() model {
	var content strings.Builder

	content.WriteString("Configuration from Control Plane\n")

	totalHvs := 0
	totalDevices := 0

	// Count hierarchical hypervisors and devices
	for _, pdu := range m.cpPDUs {
		for _, rack := range pdu.Racks {
			totalHvs += len(rack.Hypervisors)
			for _, hv := range rack.Hypervisors {
				totalDevices += len(hv.Dev)
			}
		}
	}

	// Count standalone hypervisors and devices
	totalHvs += len(m.cpHypervisors)
	for _, hv := range m.cpHypervisors {
		totalDevices += len(hv.Dev)
	}

	content.WriteString(fmt.Sprintf("Total PDUs: %d\n", len(m.cpPDUs)))
	content.WriteString(fmt.Sprintf("Total Hypervisors: %d\n", totalHvs))
	content.WriteString(fmt.Sprintf("Total Devices: %d\n\n", totalDevices))

	// Show hierarchical structure
	if len(m.cpPDUs) > 0 {
		content.WriteString("=== Hierarchical Configuration ===\n\n")
		for _, pdu := range m.cpPDUs {
			content.WriteString(fmt.Sprintf("PDU: %s\n", pdu.Name))
			content.WriteString(fmt.Sprintf("  UUID: %s\n", pdu.ID))
			if pdu.Location != "" {
				content.WriteString(fmt.Sprintf("  Location: %s\n", pdu.Location))
			}
			if pdu.PowerCapacity != "0" {
				content.WriteString(fmt.Sprintf("  Power Capacity: %s\n", pdu.PowerCapacity))
			}
			content.WriteString(fmt.Sprintf("  Racks (%d):\n", len(pdu.Racks)))

			for _, rack := range pdu.Racks {
				content.WriteString(fmt.Sprintf("    Rack: %s\n", rack.ID))
				content.WriteString(fmt.Sprintf("      UUID: %s\n", rack.ID))
				if rack.Location != "" {
					content.WriteString(fmt.Sprintf("      Location: %s\n", rack.Location))
				}
				content.WriteString(fmt.Sprintf("      Hypervisors (%d):\n", len(rack.Hypervisors)))

				for _, hv := range rack.Hypervisors {
					content.WriteString(fmt.Sprintf("        Hypervisor: %s (%s)\n", hv.ID, formatIPAddresses(hv)))
					content.WriteString(fmt.Sprintf("          UUID: %s\n", hv.ID))
					content.WriteString(fmt.Sprintf("          Failure Domain: %s/%s/%s\n", pdu.Name, rack.ID, hv.ID))
					if hv.SSHPort != "" {
						content.WriteString(fmt.Sprintf("          SSH Port: %s\n", hv.SSHPort))
					}
					content.WriteString(fmt.Sprintf("          Port Range: %s\n", hv.PortRange))
					content.WriteString(fmt.Sprintf("          Devices (%d):\n", len(hv.Dev)))

					for _, dev := range hv.Dev {
						content.WriteString(fmt.Sprintf("            • %s", dev.String()))
						if dev.State == ctlplfl.INITIALIZED {
							content.WriteString(fmt.Sprintf(" [UUID: %s",
								dev.ID))
						}
						content.WriteString("\n")
						if dev.ID != "" {
							content.WriteString(fmt.Sprintf("              ID: %s\n", dev.ID))
						}
						// Show partitions if any
						for _, partition := range dev.Partitions {
							content.WriteString(fmt.Sprintf("              Partition: %s [UUID: %s]\n",
								partition.NISDUUID, partition.PartitionID))
						}
					}
				}
			}
			content.WriteString("\n")
		}
	}

	// Show standalone hypervisors (not nested in PDUs)
	if len(m.cpHypervisors) > 0 {
		content.WriteString("=== Standalone Hypervisors ===\n\n")
		for i, hv := range m.cpHypervisors {
			if i > 0 {
				content.WriteString("\n")
			}
			content.WriteString(fmt.Sprintf("Hypervisor: %s (%s)\n", hv.ID, formatIPAddresses(hv)))
			content.WriteString(fmt.Sprintf("  UUID: %s\n", hv.ID))
			if hv.SSHPort != "" {
				content.WriteString(fmt.Sprintf("  SSH Port: %s\n", hv.SSHPort))
			}
			if hv.RackID != "" {
				content.WriteString(fmt.Sprintf("  Rack UUID: %s\n", hv.RackID))
			}
			content.WriteString(fmt.Sprintf("  Port Range: %s\n", hv.PortRange))
			content.WriteString(fmt.Sprintf("  Devices (%d):\n", len(hv.Dev)))
			for _, dev := range hv.Dev {
				content.WriteString(fmt.Sprintf("    • %s", dev.String()))
				if dev.State == ctlplfl.INITIALIZED {
					content.WriteString(fmt.Sprintf(" [UUID: %s]", dev.ID))
				}
				content.WriteString("\n")
				if dev.ID != "" {
					content.WriteString(fmt.Sprintf("      ID: %s\n", dev.ID))
				}
				for _, partition := range dev.Partitions {
					content.WriteString(fmt.Sprintf("      Partition: %s [UUID: %s]\n",
						partition.NISDUUID, partition.PartitionID))
				}
			}
		}
	}

	if len(m.cpPDUs) == 0 && len(m.cpHypervisors) == 0 {
		content.WriteString("No PDUs or hypervisors found in control plane.\n")
	}

	m.configViewport.SetContent(content.String())
	return m
}

func (m model) View() string {
	if m.quitting {
		if m.message != "" {
			return successStyle.Render(m.message) + "\nGoodbye!\n"
		}
		return "Goodbye!\n"
	}

	switch m.state {
	case stateMenu:
		return m.viewMenu()
	// PDU Management Views
	case statePDUManagement:
		return m.viewPDUManagement()
	case statePDUForm:
		return m.viewPDUForm()
	case stateEditPDU:
		return m.viewEditPDU()
	case stateDeletePDU:
		return m.viewDeletePDU()
	case stateViewPDU:
		return m.viewViewPDU()
	case stateShowAddedPDU:
		return m.viewShowAddedPDU()
	// Rack Management Views
	case stateRackManagement:
		return m.viewRackManagement()
	case stateRackForm:
		return m.viewRackForm()
	case stateRackPDUSelection:
		return m.viewRackPDUSelection()
	case stateEditRack:
		return m.viewEditRack()
	case stateDeleteRack:
		return m.viewDeleteRack()
	case stateViewRack:
		return m.viewViewRack()
	case stateShowAddedRack:
		return m.viewShowAddedRack()
	// Hypervisor Management Views
	case stateHypervisorManagement:
		return m.viewHypervisorManagement()
	case stateHypervisorForm:
		return m.viewHypervisorForm()
	case stateHypervisorRackSelection:
		return m.viewHypervisorRackSelection()
	case stateDeviceDiscovery:
		return m.viewDeviceDiscovery()
	case stateDeviceSelection:
		return m.viewDeviceSelection()
	case stateViewHypervisor:
		return m.viewViewHypervisor()
	case stateEditHypervisor:
		return m.viewEditHypervisor()
	case stateDeleteHypervisor:
		return m.viewDeleteHypervisor()
	case stateShowAddedHypervisor:
		return m.viewShowAddedHypervisor()
	// Device Management Views
	case stateDeviceManagement:
		return m.viewDeviceManagement()
	case stateDeviceView:
		return m.viewDeviceView()
	case stateDeviceDelete:
		return m.viewDeviceDelete()
	case stateDevicePartitioning:
		return m.viewDevicePartitioning()
	// Partition Management Views
	case statePartitionManagement:
		return m.viewPartitionManagement()
	case statePartitionCreate:
		return m.viewPartitionCreate()
	case statePartitionView:
		return m.viewPartitionView()
	case statePartitionDelete:
		return m.viewPartitionDelete()
	case stateShowAddedPartition:
		return m.viewShowAddedPartition()
	case statePartitionKeyCreation:
		return m.viewPartitionKeyCreation()
	case stateWholeDevicePrompt:
		return m.viewWholeDevicePrompt()
	// NISD Management Views
	case stateNISDManagement:
		return m.viewNISDManagement()
	case stateNISDInitialize:
		return m.viewNISDInitialize()
	case stateNISDPartitionSelection:
		return m.viewNISDPartitionSelection()
	case stateShowInitializedNISD:
		return m.viewShowInitializedNISD()
	case stateNISDStart:
		return m.viewNISDStart()
	case stateNISDSelection:
		return m.viewNISDSelection()
	case stateShowStartedNISD:
		return m.viewShowStartedNISD()
	case stateViewAllNISDs:
		return m.viewAllNISDs()
	// Vdev Management Views
	case stateVdevManagement:
		return m.viewVdevManagement()
	case stateVdevForm:
		return m.viewVdevForm()
	case stateVdevDeviceSelection:
		return m.viewVdevDeviceSelection()
	case stateEditVdev:
		return m.viewEditVdev()
	case stateViewVdev:
		return m.viewViewVdev()
	case stateDeleteVdev:
		return m.viewDeleteVdev()
	case stateShowAddedVdev:
		return m.viewShowAddedVdev()
	case stateVdevCreationProgress:
		return m.viewVdevCreationProgress()
	case stateVdevCreationSummary:
		return m.viewVdevCreationSummary()
	case stateSearchVdev:
		return m.viewSearchVdev()
	case stateSearchVdevResult:
		return m.viewSearchVdevResult()
	// PFS Management Views
	case statePFSManagement:
		return m.viewPFSManagement()
	case statePFSForm:
		return m.viewPFSForm()
	case stateShowAddedPFS:
		return m.viewShowAddedPFS()
	case stateViewPFS:
		return m.viewViewPFS()
	// User Management Views
	case stateUserCreateForm:
		return m.viewUserCreateForm()
	case stateUserCreateAdminKey:
		return m.viewUserCreateAdminKey()
	case stateUserCreateToken:
		return m.viewUserCreateToken()
	case stateUserCreateResult:
		return m.viewUserCreateResult()
	// User Login Views
	case stateUserLoginForm:
		return m.viewUserLoginForm()
	case stateUserLoginSecret:
		return m.viewUserLoginSecret()
	case stateUserLoginResult:
		return m.viewUserLoginResult()
	// Control Plane Views
	// Configuration View
	case stateViewConfig:
		return m.viewConfig()
	}

	return ""
}

func (m model) viewMenu() string {
	title := titleStyle.Render("Niova Backend Configuration Tool")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	// Control plane status
	if m.cpEnabled {
		cpStatus := errorStyle.Render("Disconnected")
		if m.cpConnected {
			cpStatus = successStyle.Render("Connected")
		}
		s.WriteString(fmt.Sprintf("Control Plane: %s\n\n", cpStatus))
	} else {
		s.WriteString(fmt.Sprintf("Control Plane: %s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("Disabled")))
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

	// Login status
	if !m.authEnabled {
		s.WriteString(dimStyle.Render("Authentication: Disabled") + "\n\n")
	} else if m.loggedInUser != nil {
		s.WriteString(successStyle.Render(fmt.Sprintf("Logged in as: %s (role: %s)", m.loggedInUser.Username, m.loggedInUser.UserRole)) + "\n\n")
	} else {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("Not logged in") + "\n\n")
	}

	s.WriteString("Select an option:\n\n")
	for i, item := range m.menuItems {
		// Items 2-9 are locked when not logged in
		locked := m.authEnabled && m.loggedInUser == nil && i >= 2 && i <= 9
		cursor := "  "
		if m.menuCursor == i {
			cursor = "▶ "
			if locked {
				s.WriteString(dimStyle.Render(fmt.Sprintf("%s%s", cursor, item.title)))
			} else {
				s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%s", cursor, item.title)))
			}
		} else {
			if locked {
				s.WriteString(dimStyle.Render(fmt.Sprintf("%s%s", cursor, item.title)))
			} else {
				s.WriteString(fmt.Sprintf("%s%s", cursor, item.title))
			}
		}
		if locked {
			s.WriteString(dimStyle.Render(fmt.Sprintf(" - %s [Login Required]", item.desc)) + "\n")
		} else {
			s.WriteString(fmt.Sprintf(" - %s\n", item.desc))
		}
	}

	s.WriteString("\n")
	s.WriteString("Controls: ↑/↓ navigate, enter/space select, ctrl+c quit")

	return s.String()
}

func (m model) viewHypervisorForm() string {
	var title string
	if m.editingUUID != "" {
		title = titleStyle.Render("Edit Hypervisor")
	} else {
		title = titleStyle.Render("Add New Hypervisor")
	}

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	// Show Rack selection with dropdown-style interface
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("═══ RACK SELECTION ═══") + "\n")
	allRacks := m.getAllRacks()
	if len(allRacks) > 0 {
		if m.selectedRackIdx >= 0 && m.selectedRackIdx < len(allRacks) {
			rack := allRacks[m.selectedRackIdx]
			s.WriteString(fmt.Sprintf("🏠 Selected Rack: %s", selectedItemStyle.Render(rack.Rack.ID)))
			if rack.Rack.Location != "" {
				s.WriteString(fmt.Sprintf(" (%s)", rack.Rack.Location))
			}
			s.WriteString(fmt.Sprintf(" [PDU: %s]", rack.PDUName))
			s.WriteString("\n")
		} else {
			s.WriteString(errorStyle.Render("⚠️  No rack selected - will be added as legacy") + "\n")
		}
		s.WriteString(fmt.Sprintf("   🗂️ %d racks available - Press 'R' to select different rack\n", len(allRacks)))
	} else {
		s.WriteString(errorStyle.Render("⚠️  No racks available - Create racks first") + "\n")
		s.WriteString("   Hypervisor will be added as legacy (not in hierarchy)\n")
	}
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("═══════════════════") + "\n\n")

	// Basic form fields (name, ssh port, port range)
	basicLabels := []string{"Name:", "SSH Port:", "Port Range:"}
	basicInputs := []int{0, 3, 4} // indices in m.inputs array

	for i, labelIdx := range basicInputs {
		if labelIdx < len(m.inputs) {
			s.WriteString(basicLabels[i] + "\n")

			if labelIdx == int(m.focusedInput) && !m.ipManagementMode {
				s.WriteString(focusedStyle.Render(m.inputs[labelIdx].View()) + "\n\n")
			} else {
				s.WriteString(blurredStyle.Render(m.inputs[labelIdx].View()) + "\n\n")
			}
		}
	}

	// Dynamic IP Address Management Section
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("═══ IP ADDRESSES ═══") + "\n")
	s.WriteString("Add one or more IP addresses for this hypervisor:\n\n")

	for i, ipInput := range m.ipInputs {
		label := "Primary IP:"
		if i > 0 {
			label = fmt.Sprintf("IP Address %d:", i+1)
		}
		s.WriteString(label + "\n")

		if m.ipManagementMode && i == m.ipFocusedIndex {
			s.WriteString(focusedStyle.Render(ipInput.View()))
		} else {
			s.WriteString(blurredStyle.Render(ipInput.View()))
		}

		// Show hint for removing additional IPs
		if i > 0 && m.ipManagementMode && i == m.ipFocusedIndex {
			s.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("(Delete/Backspace to remove)"))
		}
		s.WriteString("\n\n")
	}

	// Add IP button
	addButtonStyle := blurredStyle
	if m.ipManagementMode && m.ipFocusedIndex == len(m.ipInputs) {
		addButtonStyle = focusedStyle
	}
	s.WriteString(addButtonStyle.Render("➕ Add IP Address") + "\n\n")

	s.WriteString(lipgloss.NewStyle().Bold(true).Render("═══════════════════") + "\n\n")

	// Controls help
	if m.ipManagementMode {
		s.WriteString("IP Management: tab/↑/↓ navigate, enter on ➕ adds IP, " +
			lipgloss.NewStyle().Bold(true).Render("Delete/Backspace removes IP") + "\n")
		s.WriteString("Other: " +
			lipgloss.NewStyle().Bold(true).Render("R select rack") +
			", " + lipgloss.NewStyle().Bold(true).Render("F2 submit") +
			", esc back to basic fields")
	} else {
		s.WriteString("Controls: tab/↑/↓ navigate all fields, " +
			lipgloss.NewStyle().Bold(true).Render("R select rack") +
			", " + lipgloss.NewStyle().Bold(true).Render("enter/F2 submit") +
			", esc back")
	}

	return s.String()
}

func (m model) viewDeviceDiscovery() string {
	title := titleStyle.Render("Discovering Devices...")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	// Use primary IP for connection
	primaryIP, err := m.currentHv.GetPrimaryIP()
	if err != nil {
		log.Error("viewDeviceDiscovery():failed to fetch network info: ", err)
	}

	s.WriteString(fmt.Sprintf("Connecting to  one of the IP's from: %+v...\n", primaryIP))
	s.WriteString("Scanning for storage devices...\n\n")
	s.WriteString("Please wait...")

	return s.String()
}

func (m model) viewDeviceSelection() string {
	title := titleStyle.Render("Select Storage Devices")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	// Use primary IP for display
	primaryIP, err := m.currentHv.GetPrimaryIP()
	if err != nil {
		log.Error("viewDeviceSelection():failed to fetch network info: ", err)
	}

	s.WriteString(fmt.Sprintf("Hypervisor: %s (%s)\n\n",
		m.currentHv.ID, primaryIP))

	if len(m.discoveredDevs) == 0 {
		s.WriteString("No storage devices found.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	totalPages := (len(m.discoveredDevs) + m.devicesPerPage - 1) / m.devicesPerPage
	currentPageDevices := m.getDevicesOnCurrentPage()

	s.WriteString(fmt.Sprintf("Found %d storage devices (Page %d/%d):\n\n",
		len(m.discoveredDevs), m.devicePage+1, totalPages))

	// Show devices for current page only
	start := m.devicePage * m.devicesPerPage
	for i, dev := range currentPageDevices {
		actualIndex := start + i
		cursor := "  "
		if i == m.deviceCursor {
			cursor = "▶ "
		}

		selection := " "
		if m.selectedDevices[actualIndex] {
			selection = "✓"
		}

		sizeStr := fmt.Sprintf("%.1f GB", float64(dev.Size)/(1024*1024*1024))
		if dev.Size >= 1024*1024*1024*1024 {
			sizeStr = fmt.Sprintf("%.1f TB", float64(dev.Size)/(1024*1024*1024*1024))
		}
		line := fmt.Sprintf("%s[%s] %d. %s (%s)", cursor, selection, actualIndex+1, dev.Name, sizeStr)

		if i == m.deviceCursor {
			line = selectedItemStyle.Render(line)
		}

		s.WriteString(line + "\n")

		// Show device ID as detail
		desc := fmt.Sprintf("    ID: %s", dev.ID)
		if i == m.deviceCursor {
			desc = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(desc)
		} else {
			desc = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render(desc)
		}
		s.WriteString(desc + "\n")
		s.WriteString("\n")
	}

	selectedCount := 0
	for _, selected := range m.selectedDevices {
		if selected {
			selectedCount++
		}
	}

	s.WriteString(fmt.Sprintf("Selected: %d/%d devices\n\n", selectedCount, len(m.discoveredDevs)))

	// Show pagination info if multiple pages
	if totalPages > 1 {
		s.WriteString(fmt.Sprintf("Page %d of %d | ", m.devicePage+1, totalPages))
		if m.devicePage > 0 {
			s.WriteString("← prev ")
		}
		if m.devicePage < totalPages-1 {
			s.WriteString("next →")
		}
		s.WriteString("\n\n")
	}

	s.WriteString("Controls:\n")
	s.WriteString("• ↑/↓ or j/k: navigate devices\n")
	if totalPages > 1 {
		s.WriteString("• ←/→ or h/l: navigate pages\n")
	}
	s.WriteString("• space: toggle device selection\n")
	s.WriteString("• a: select all devices, n: select none\n")
	s.WriteString("• enter: confirm selection, esc: back to menu")

	return s.String()
}

// Render configuration in collapsible hierarchical view with cursor navigation
func (m model) renderHierarchicalTable() string {
	var s strings.Builder

	s.WriteString(lipgloss.NewStyle().Bold(true).Render("INFRASTRUCTURE CONFIGURATION") + "\n")
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("═══════════════════════════════") + "\n\n")

	// Styles for tree view
	expandedIcon := "▼"
	collapsedIcon := "▶"
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("#444444")).Foreground(lipgloss.Color("#FFFFFF"))

	// Build the flat list of visible tree items
	treeItems := m.buildTreeItemList()

	// Display summary
	s.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#87CEEB")).Render("PDUs") +
		fmt.Sprintf(" (%d)\n", len(m.cpPDUs)))

	// Render each tree item
	for i, item := range treeItems {
		indent := strings.Repeat("  ", item.Level+1) // +1 for base indentation

		var line string

		// Determine if this item can be expanded and its current state
		canExpand := false
		isExpanded := false

		switch item.Type {
		case "pdu":
			pdu := m.cpPDUs[item.Index]
			canExpand = len(pdu.Racks) > 0
			isExpanded = m.expandedPDUs[item.Index]

			icon := collapsedIcon
			if isExpanded && canExpand {
				icon = expandedIcon
			} else if !canExpand {
				icon = "  " // No icon for non-expandable items
			}

			line = fmt.Sprintf("%s %s", icon, pdu.Name)
			if pdu.Location != "" {
				line += fmt.Sprintf(" (%s)", pdu.Location)
			}
			if pdu.PowerCapacity != "0" {
				line += fmt.Sprintf(" - %s", pdu.PowerCapacity)
			}
			line += fmt.Sprintf(" [%d racks]", len(pdu.Racks))

		case "rack":
			// Find the rack by UUID
			var rack ctlplfl.Rack
			for _, pdu := range m.cpPDUs {
				for _, r := range pdu.Racks {
					if r.ID == item.UUID {
						rack = r
						break
					}
				}
			}

			canExpand = len(rack.Hypervisors) > 0
			isExpanded = m.expandedRacks[item.UUID]

			icon := collapsedIcon
			if isExpanded && canExpand {
				icon = expandedIcon
			} else if !canExpand {
				icon = "  "
			}

			line = fmt.Sprintf("%s %s", icon, rack.ID)
			if rack.Location != "" {
				line += fmt.Sprintf(" (%s)", rack.Location)
			}
			line += fmt.Sprintf(" [%d hypervisors]", len(rack.Hypervisors))

		case "hypervisor":
			// Find the hypervisor by UUID
			var hv ctlplfl.Hypervisor
			for _, pdu := range m.cpPDUs {
				for _, rack := range pdu.Racks {
					for _, h := range rack.Hypervisors {
						if h.ID == item.UUID {
							hv = h
							break
						}
					}
				}
			}

			canExpand = len(hv.Dev) > 0
			isExpanded = m.expandedHypers[item.UUID]

			icon := collapsedIcon
			if isExpanded && canExpand {
				icon = expandedIcon
			} else if !canExpand {
				icon = "  "
			}

			line = fmt.Sprintf("%s %s (%s) [%d devices]", icon, hv.ID, formatIPAddresses(hv), len(hv.Dev))

		case "device":
			// Find the device by parsing the ID (format: hypervisor-uuid-device-name)
			parts := strings.Split(item.UUID, "-")
			if len(parts) >= 2 {
				deviceName := strings.Join(parts[1:], "-") // In case device name contains hyphens
				hvUUID := parts[0]

				var device Device
				for _, pdu := range m.cpPDUs {
					for _, rack := range pdu.Racks {
						for _, hv := range rack.Hypervisors {
							if hv.ID == hvUUID {
								for _, d := range hv.Dev {
									if d.ID == deviceName {
										device = d
										break
									}
								}
							}
						}
					}
				}

				canExpand = len(device.Partitions) > 0
				isExpanded = m.expandedDevices[item.UUID]

				icon := collapsedIcon
				if isExpanded && canExpand {
					icon = expandedIcon
				} else if !canExpand {
					icon = "  "
				}

				line = fmt.Sprintf("%s %s", icon, item.Name)
				if device.Size != 0 {
					line += fmt.Sprintf(" (%s)", formatBytes(device.Size))
				}
				if len(device.Partitions) > 0 {
					line += fmt.Sprintf(" [%d partitions]", len(device.Partitions))
				}
			}

		case "legacy_hypervisor":
			hv := m.cpHypervisors[item.Index]
			canExpand = len(hv.Dev) > 0
			isExpanded = m.expandedHvs[item.Index]

			icon := collapsedIcon
			if isExpanded && canExpand {
				icon = expandedIcon
			} else if !canExpand {
				icon = "  "
			}

			line = fmt.Sprintf("%s %s (%s) [%d devices]", icon, hv.ID, formatIPAddresses(hv), len(hv.Dev))

		case "legacy_device":
			// Similar to device handling but for standalone hypervisors
			parts := strings.Split(item.UUID, "-")
			if len(parts) >= 2 {
				deviceName := strings.Join(parts[1:], "-")
				hvUUID := parts[0]

				var device Device
				for _, hv := range m.cpHypervisors {
					if hv.ID == hvUUID {
						for _, d := range hv.Dev {
							if d.ID == deviceName {
								device = d
								break
							}
						}
					}
				}

				canExpand = len(device.Partitions) > 0
				isExpanded = m.expandedDevices[item.UUID]

				icon := collapsedIcon
				if isExpanded && canExpand {
					icon = expandedIcon
				} else if !canExpand {
					icon = "  "
				}

				line = fmt.Sprintf("%s %s", icon, item.Name)
				if device.Size != 0 {
					line += fmt.Sprintf(" (%s)", formatBytes(device.Size))
				}
				if len(device.Partitions) > 0 {
					line += fmt.Sprintf(" [%d partitions]", len(device.Partitions))
				}
			}

		case "partition":
			// Find the partition details
			var partition DevicePartition
			found := false

			// Find the partition by UUID across all devices
			for _, pdu := range m.cpPDUs {
				if found {
					break
				}
				for _, rack := range pdu.Racks {
					if found {
						break
					}
					for _, hv := range rack.Hypervisors {
						if found {
							break
						}
						for _, device := range hv.Dev {
							if found {
								break
							}
							for _, p := range device.Partitions {
								if p.PartitionID == item.UUID {
									partition = p
									found = true
									break
								}
							}
						}
					}
				}
			}

			// Check standalone hypervisors too
			if !found {
				for _, hv := range m.cpHypervisors {
					if found {
						break
					}
					for _, device := range hv.Dev {
						if found {
							break
						}
						for _, p := range device.Partitions {
							if p.PartitionID == item.UUID {
								partition = p
								found = true
								break
							}
						}
					}
				}
			}

			line = fmt.Sprintf("  %s", partition.NISDUUID)
			if partition.Size > 0 {
				line += fmt.Sprintf(" (%s)", formatBytes(partition.Size))
			}
			if partition.PartitionID != "" {
				line += fmt.Sprintf(" (ID: %s)", partition.PartitionID[:8]+"...")
			}
		}

		// Apply cursor highlighting
		fullLine := indent + line
		if i == m.configCursor {
			fullLine = "▶ " + selectedStyle.Render(fullLine)
		} else {
			fullLine = "  " + fullLine
		}

		s.WriteString(fullLine + "\n")
	}

	// Standalone hypervisors section header if any exist
	if len(m.cpHypervisors) > 0 {
		s.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFA500")).Render("Standalone Hypervisors") +
			fmt.Sprintf(" (%d)\n", len(m.cpHypervisors)))
	}

	return s.String()
}

func (m model) viewConfig() string {
	title := titleStyle.Render("Infrastructure Configuration")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	// Show debug message if any
	if m.message != "" {
		if strings.Contains(m.message, "Failed") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	// Display configuration summary
	totalRacks := 0
	totalHypervisors := 0
	totalDevices := 0

	for _, pdu := range m.cpPDUs {
		totalRacks += len(pdu.Racks)
		for _, rack := range pdu.Racks {
			totalHypervisors += len(rack.Hypervisors)
			for _, hv := range rack.Hypervisors {
				totalDevices += len(hv.Dev)
			}
		}
	}

	// Add standalone hypervisors to totals
	totalHypervisors += len(m.cpHypervisors)
	for _, hv := range m.cpHypervisors {
		totalDevices += len(hv.Dev)
	}

	s.WriteString(fmt.Sprintf("📊 Summary: %d PDUs, %d Racks, %d Hypervisors, %d Devices\n\n",
		len(m.cpPDUs), totalRacks, totalHypervisors, totalDevices))

	// Hierarchical Table Display
	if len(m.cpPDUs) == 0 && len(m.cpHypervisors) == 0 {
		s.WriteString(errorStyle.Render("⚠️  No infrastructure configured") + "\n\n")
		s.WriteString("Add PDUs, Racks, and Hypervisors from the main menu to build your infrastructure.\n")
		s.WriteString("Use the management options to create a hierarchical configuration.")
	} else {
		s.WriteString(m.renderHierarchicalTable())
	}

	s.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render("Controls:") + "\n")
	s.WriteString("• ↑/↓: Navigate items\n")
	s.WriteString("• Enter/Space: Toggle expand/collapse\n")
	s.WriteString("• E: Expand all, C: Collapse all\n")
	s.WriteString("• ESC: Back to main menu")
	return s.String()
}

func (m model) viewHypervisorManagement() string {
	title := titleStyle.Render("Hypervisor Management")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Select an action:\n\n")

	managementItems := []string{
		"Add Hypervisor - Create new hypervisor configuration",
	}

	allHypervisors := m.getAllHypervisors()
	if len(allHypervisors) > 0 {
		managementItems = append(managementItems,
			"View Hypervisor - Display hypervisor details",
			"Edit Hypervisor - Modify existing hypervisor",
			"Delete Hypervisor - Remove hypervisor from configuration")
	}

	for i, item := range managementItems {
		cursor := "  "
		if i == m.hvManagementCursor {
			cursor = "▶ "
			s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, item)))
		} else {
			s.WriteString(fmt.Sprintf("%s%d. %s", cursor, i+1, item))
		}
		s.WriteString("\n")
	}

	// Show rack selection status
	s.WriteString("\n")
	allRacks := m.getAllRacks()
	if len(allRacks) == 0 {
		s.WriteString(errorStyle.Render("⚠️  No racks available - Create racks first") + "\n")
	} else {
		if m.selectedRackIdx >= 0 && m.selectedRackIdx < len(allRacks) {
			rack := allRacks[m.selectedRackIdx]
			s.WriteString(fmt.Sprintf("🏠 Selected Rack: %s", selectedItemStyle.Render(rack.Rack.ID)))
			if rack.Rack.Location != "" {
				s.WriteString(fmt.Sprintf(" (%s)", rack.Rack.Location))
			}
			s.WriteString("\n")
		} else {
			s.WriteString(errorStyle.Render("⚠️  No rack selected") + " - Press 'R' to select a rack\n")
		}
	}

	s.WriteString(fmt.Sprintf("\nCurrent hypervisors: %d\n\n", len(m.getAllHypervisors())))
	s.WriteString("Controls: ↑/↓ navigate, enter select, R rack selection, esc back to main menu")

	return s.String()
}

func (m model) updateViewHypervisor(msg tea.Msg) (model, tea.Cmd) {
	// Query CP for hypervisor data on first load or refresh
	if m.cpClient != nil && m.cpHypervisorRefresh {
		log.Info("  cpRaftUUID: ", m.cpRaftUUID)
		log.Info("  cpGossipPath: ", m.cpGossipPath)

		if m.cpClient != nil {
			log.Info("Calling GetHypervisor with GetAll=true")
			cpHypervisors, err := m.cpClient.GetHypervisor(&ctlplfl.GetReq{GetAll: true})
			if err != nil {
				// Log error and show empty list
				log.Error("Failed to query Hypervisors from Control Plane: ", err)
				m.message = "Error: Could not fetch Hypervisor data from Control Plane: " + err.Error()
				m.cpHypervisors = []ctlplfl.Hypervisor{} // Show empty list on error
			} else {
				// Show only CP data, no merging with config file
				m.cpHypervisors = cpHypervisors
				log.Info("Hypervisor data from CP: ", m.cpHypervisors)
				m.message = fmt.Sprintf("Hypervisor data loaded from Control Plane (%d Hypervisors)", len(cpHypervisors))
				log.Info("Successfully retrieved ", len(cpHypervisors), " Hypervisors from Control Plane")
				log.Info(m.cpHypervisors)
			}
			m.cpHypervisorRefresh = false
		} else {
			m.cpHypervisors = []ctlplfl.Hypervisor{} // Show empty list when no CP client
			log.Warn("cpClient is nil, cannot query Control Plane")
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.hvListCursor > 0 {
				m.hvListCursor--
			}
		case "down", "j":
			if m.hvListCursor < len(m.cpHypervisors)-1 {
				m.hvListCursor++
			}
		case "enter", " ":
			// Just stay on the current hypervisor for viewing
			return m, nil
		case "esc":
			m.state = stateHypervisorManagement
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewViewHypervisor() string {
	title := titleStyle.Render("View Hypervisor Details")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	// Get hypervisors from Control Plane
	hypervisors := m.cpHypervisors
	log.Info("m.cpHypervisors in viewViewHypervisor: ", m.cpHypervisors)

	// Show data source indicator
	if m.cpClient != nil && len(m.cpHypervisors) > 0 {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("📡 Data from Control Plane") + "\n")
	} else if m.cpClient != nil {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("⚠️  Control Plane connected, no Hypervisor data available") + "\n")
	} else if m.cpEnabled {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("❌ Control Plane not connected") + "\n")
	} else {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("ℹ️  Control Plane disabled - using config file data") + "\n")
		// Fall back to config file data when CP is disabled
		allHypervisors := m.getAllHypervisors()
		if len(allHypervisors) == 0 {
			s.WriteString(errorStyle.Render("No hypervisors found") + "\n\n")
			s.WriteString("Controls: esc back")
			return s.String()
		}

		// Show hypervisor list from config
		s.WriteString("Select a hypervisor to view details:\n\n")
		for i, hvInfo := range allHypervisors {
			cursor := "  "
			if i == m.hvListCursor {
				cursor = "▶ "
			}

			hvName := hvInfo.Hypervisor.Name
			if hvName == "" {
				hvName = "Unnamed"
			}

			line := fmt.Sprintf("%s%s (%s)", cursor, hvName, formatIPAddresses(hvInfo.Hypervisor))
			if i == m.hvListCursor {
				s.WriteString(selectedItemStyle.Render(line))
			} else {
				s.WriteString(line)
			}
			s.WriteString(fmt.Sprintf(" - %s\n", hvInfo.Location))
		}

		// Show detailed info for selected hypervisor from config
		if m.hvListCursor < len(allHypervisors) {
			selectedHv := allHypervisors[m.hvListCursor]
			hv := selectedHv.Hypervisor

			s.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render("═══════════════════") + "\n")
			s.WriteString(fmt.Sprintf("📋 Hypervisor Details:\n\n"))
			s.WriteString(fmt.Sprintf("Name: %s\n", hv.Name))
			s.WriteString(fmt.Sprintf("UUID: %s\n", hv.ID))
			s.WriteString(fmt.Sprintf("%s\n", formatDetailedIPAddresses(hv)))
			s.WriteString(fmt.Sprintf("SSH Port: %s\n", hv.SSHPort))
			s.WriteString(fmt.Sprintf("Port Range: %s\n", hv.PortRange))
			if hv.RackID != "" {
				s.WriteString(fmt.Sprintf("Rack ID: %s\n", hv.RackID))
			}
			s.WriteString(fmt.Sprintf("Location: %s\n", selectedHv.Location))

			if selectedHv.RackInfo != nil {
				s.WriteString(fmt.Sprintf("PDU: %s\n", selectedHv.RackInfo.PDUName))
			}

			s.WriteString(fmt.Sprintf("\nDevices (%d):\n", len(hv.Dev)))
			if len(hv.Dev) == 0 {
				s.WriteString("  No devices configured\n")
			} else {
				for i, device := range hv.Dev {
					s.WriteString(fmt.Sprintf("  %d. %s", i+1, device.Name))
					if device.DevicePath != "" {
						s.WriteString(fmt.Sprintf(" (%s)", device.DevicePath))
					}
					if device.State == ctlplfl.INITIALIZED {
						s.WriteString(" [Initialized]")
					}
					s.WriteString("\n")

					if len(device.Partitions) > 0 {
						s.WriteString(fmt.Sprintf("     Partitions (%d):\n", len(device.Partitions)))
						for j, partition := range device.Partitions {
							s.WriteString(fmt.Sprintf("       %d. %s (Size: %s)\n",
								j+1, partition.PartitionID, formatBytes(partition.Size)))
						}
					}
				}
			}
		}

		s.WriteString("\n\nControls: ↑/↓ navigate hypervisors, esc back to menu")
		return s.String()
	}

	if len(hypervisors) == 0 {
		s.WriteString(errorStyle.Render("No hypervisors found in Control Plane") + "\n\n")
		s.WriteString("Controls: esc back")
		return s.String()
	}

	// Show hypervisor list from Control Plane
	s.WriteString("Select a hypervisor to view details:\n\n")
	for i, hv := range hypervisors {
		cursor := "  "
		if i == m.hvListCursor {
			cursor = "▶ "
		}

		hvName := hv.Name
		if hvName == "" {
			hvName = "Unnamed"
		}

		line := fmt.Sprintf("%s%s (%s)", cursor, hvName, formatIPAddresses(hv))
		if i == m.hvListCursor {
			s.WriteString(selectedItemStyle.Render(line))
		} else {
			s.WriteString(line)
		}
		if hv.RackID != "" {
			s.WriteString(fmt.Sprintf(" - Rack: %s\n", hv.RackID))
		} else {
			s.WriteString("\n")
		}
	}

	// Show detailed info for selected hypervisor from Control Plane
	if m.hvListCursor < len(hypervisors) {
		hv := hypervisors[m.hvListCursor]

		s.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render("═══════════════════") + "\n")
		s.WriteString(fmt.Sprintf("📋 Hypervisor Details (from Control Plane):\n\n"))
		s.WriteString(fmt.Sprintf("Name: %s\n", hv.Name))
		s.WriteString(fmt.Sprintf("UUID: %s\n", hv.ID))
		s.WriteString(fmt.Sprintf("%s\n", formatDetailedIPAddresses(hv)))
		if hv.RackID != "" {
			s.WriteString(fmt.Sprintf("Rack ID: %s\n", hv.RackID))
		}
		// Note: Control plane hypervisor data may have different fields than config file hypervisors
		// Only show the fields available in the ctlplfl.Hypervisor struct
	}

	s.WriteString("\n\nControls: ↑/↓ navigate hypervisors, esc back to menu")
	return s.String()
}

func (m model) viewEditHypervisor() string {
	title := titleStyle.Render("Edit Hypervisor")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	allHypervisors := m.getAllHypervisors()
	if len(allHypervisors) == 0 {
		s.WriteString("No hypervisors available to edit.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("Select hypervisor to edit:\n\n")

	for i, hvInfo := range allHypervisors {
		cursor := "  "
		if i == m.hvListCursor {
			cursor = "▶ "
		}

		hvName := hvInfo.Hypervisor.Name
		if hvName == "" {
			hvName = "Unnamed"
		}

		line := fmt.Sprintf("%s%d. %s (%s) - %s", cursor, i+1, hvName, formatIPAddresses(hvInfo.Hypervisor), hvInfo.Location)
		if i == m.hvListCursor {
			line = selectedItemStyle.Render(line)
		}
		s.WriteString(line + "\n")

		// Show additional details for selected item
		if i == m.hvListCursor {
			hv := hvInfo.Hypervisor
			details := fmt.Sprintf("    UUID: %s", hv.ID)
			if hv.RackID != "" {
				details += fmt.Sprintf(" | Rack: %s", hv.RackID)
			}
			details += fmt.Sprintf(" | Devices: %d", len(hv.Dev))
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(details) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate, enter edit selected, esc back")

	return s.String()
}

func (m model) viewDeleteHypervisor() string {
	title := titleStyle.Render("Delete Hypervisor")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	allHypervisors := m.getAllHypervisors()
	if len(allHypervisors) == 0 {
		s.WriteString("No hypervisors available to delete.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("⚠️  WARNING: This will permanently delete the hypervisor!\n\n")
	s.WriteString("Select hypervisor to delete:\n\n")

	for i, hvInfo := range allHypervisors {
		cursor := "  "
		if i == m.hvListCursor {
			cursor = "▶ "
		}

		line := fmt.Sprintf("%s%d. %s (%s)", cursor, i+1, hvInfo.Hypervisor.ID, formatIPAddresses(hvInfo.Hypervisor))
		if i == m.hvListCursor {
			line = errorStyle.Render(line) // Show selected item in red for deletion
		}
		s.WriteString(line + "\n")

		// Show additional details for selected item
		if i == m.hvListCursor {
			details := fmt.Sprintf("    UUID: %s", hvInfo.Hypervisor.ID)
			details += fmt.Sprintf(" | Location: %s", hvInfo.Location)
			details += fmt.Sprintf(" | Source: %s", hvInfo.Source)
			details += fmt.Sprintf(" | %d devices will be removed", len(hvInfo.Hypervisor.Dev))
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(details) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate, enter/y delete, n cancel, esc back")

	return s.String()
}

func (m model) viewDeviceManagement() string {
	title := titleStyle.Render("Device Management")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Select a device management option:\n\n")

	managementItems := []string{
		"View Device - Display device details and status",
	}

	for i, item := range managementItems {
		cursor := "  "
		if i == m.deviceMgmtCursor {
			cursor = "▶ "
			s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, item)))
		} else {
			s.WriteString(fmt.Sprintf("%s%d. %s", cursor, i+1, item))
		}
		s.WriteString("\n")
	}

	// Show stats
	initializedCount := 0
	totalDevices := 0
	for _, hv := range m.cpHypervisors {
		totalDevices += len(hv.Dev)
		for _, device := range hv.Dev {
			if device.State == ctlplfl.INITIALIZED {
				initializedCount++
			}
		}
	}

	s.WriteString(fmt.Sprintf("\nDevice Statistics:\n"))
	s.WriteString(fmt.Sprintf("• Total devices: %d\n", totalDevices))
	s.WriteString(fmt.Sprintf("• Initialized devices: %d\n", initializedCount))
	s.WriteString(fmt.Sprintf("• Uninitialized devices: %d\n", totalDevices-initializedCount))

	s.WriteString("\nControls: ↑/↓ navigate, enter select, esc back to main menu")

	return s.String()
}

func (m model) viewDeviceView() string {
	title := titleStyle.Render("View All Devices")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	allDevices := m.getAllDevicesForPartitioning()

	if len(allDevices) == 0 {
		s.WriteString("No devices found.\n")
		s.WriteString("Please add devices to hypervisors first.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	// Sort devices for consistent ordering
	sort.Slice(allDevices, func(i, j int) bool {
		// First sort by hypervisor name, then by device ID
		if allDevices[i].HvName != allDevices[j].HvName {
			return allDevices[i].HvName < allDevices[j].HvName
		}
		return allDevices[i].Device.ID < allDevices[j].Device.ID
	})

	s.WriteString(fmt.Sprintf("Found %d devices across all hypervisors:\n\n", len(allDevices)))

	// Calculate dynamic pagination based on terminal height
	// Each device takes ~1 line normally, ~5-8 lines when selected with details
	// Use conservative estimate of 2 lines per item for calculation
	itemsPerPage := m.calculateItemsPerPage(2)
	totalPages := (len(allDevices) + itemsPerPage - 1) / itemsPerPage

	// Ensure cursor is within bounds
	if m.selectedDeviceIdx < 0 {
		m.selectedDeviceIdx = 0
	}
	if m.selectedDeviceIdx >= len(allDevices) {
		m.selectedDeviceIdx = len(allDevices) - 1
	}

	// Calculate which page the cursor is on
	currentPage := m.selectedDeviceIdx / itemsPerPage
	startIdx := currentPage * itemsPerPage
	endIdx := startIdx + itemsPerPage
	if endIdx > len(allDevices) {
		endIdx = len(allDevices)
	}

	// Show pagination info when there are multiple pages
	if totalPages > 1 {
		s.WriteString(fmt.Sprintf("Page %d of %d (showing items %d-%d of %d) | Cursor at position %d\n\n",
			currentPage+1, totalPages, startIdx+1, endIdx, len(allDevices), m.selectedDeviceIdx+1))
	} else {
		// Show cursor position for debugging even on single page
		s.WriteString(fmt.Sprintf("Cursor at position %d of %d\n\n", m.selectedDeviceIdx+1, len(allDevices)))
	}

	for i := startIdx; i < endIdx; i++ {
		deviceInfo := allDevices[i]
		cursor := "  "
		if i == m.selectedDeviceIdx {
			cursor = "▶ "
		}

		device := deviceInfo.Device

		// Build status indicator
		status := ""
		if device.State == ctlplfl.INITIALIZED {
			status = "[INITIALIZED]"
		} else {
			status = "[UNINITIALIZED]"
		}

		// Header line
		headerLine := fmt.Sprintf("%s%d. %s %s on %s", cursor, i+1, device.Name, status, deviceInfo.HvName)
		if device.Size != 0 {
			headerLine += fmt.Sprintf(" (%s)", formatBytes(device.Size))
		}
		if len(device.Partitions) > 0 {
			headerLine += fmt.Sprintf(" [%d partitions]", len(device.Partitions))
		}

		if i == m.selectedDeviceIdx {
			s.WriteString(selectedItemStyle.Render(headerLine) + "\n")

			// Detailed view for selected device
			s.WriteString(fmt.Sprintf("    Hypervisor: %s (UUID: %s)\n", deviceInfo.HvName, deviceInfo.HvUUID))
			if device.ID != "" {
				s.WriteString(fmt.Sprintf("    Device UUID: %s\n", device.ID))
			}
			if device.Name != "" && device.Name != device.ID {
				s.WriteString(fmt.Sprintf("    Device Name: %s\n", device.Name))
			}
			if device.SerialNumber != "" {
				s.WriteString(fmt.Sprintf("    Serial Number: %s\n", device.SerialNumber))
			}
			if device.Size != 0 {
				s.WriteString(fmt.Sprintf("    Size: %s\n", formatBytes(device.Size)))
			}
			if device.State == ctlplfl.INITIALIZED {
				if device.DevicePath != "" {
					s.WriteString(fmt.Sprintf("    Device Path: %s\n", device.DevicePath))
				}
				if device.FailureDomain != "" {
					s.WriteString(fmt.Sprintf("    Failure Domain: %s\n", device.FailureDomain))
				}
			} else {
				s.WriteString("    Status: Device not initialized\n")
			}

			// Show partitions if any
			if len(device.Partitions) > 0 {
				// The Device ID is the entity UUID to use when filtering Vdev
				// allocation at the device level (FD type = device). All partitions
				// of this device share the same DevID == device.ID.
				if device.ID != "" {
					s.WriteString(fmt.Sprintf("    Device Entity UUID (use for FD=device filter): %s\n", device.ID))
				}
				s.WriteString(fmt.Sprintf("    Partitions (%d):\n", len(device.Partitions)))
				for j, partition := range device.Partitions {
					s.WriteString(fmt.Sprintf("      %d. %s", j+1, partition.NISDUUID))
					if partition.Size > 0 {
						s.WriteString(fmt.Sprintf(" (%s)", formatBytes(partition.Size)))
					}
					s.WriteString("\n")
				}
			}
		} else {
			s.WriteString(headerLine + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate • esc back to device management")

	return s.String()
}

func (m model) viewDeviceDelete() string {
	title := titleStyle.Render("Delete Device Initialization")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	initializedDevices := m.getAllInitializedDevices()

	if len(initializedDevices) == 0 {
		s.WriteString("No initialized devices found.\n")
		s.WriteString("Nothing to delete.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("⚠️  WARNING: This will remove device initialization!\n")
	s.WriteString("The device will become uninitialized and lose its UUID, ports, and configuration.\n\n")
	s.WriteString("Select device to uninitialize:\n\n")

	for i, deviceInfo := range initializedDevices {
		cursor := "  "
		if i == m.selectedDeviceIdx {
			cursor = "▶ "
		}

		hv := m.cpHypervisors[deviceInfo.HvIndex]
		device := deviceInfo.Device

		line := fmt.Sprintf("%s%d. %s on %s (%s)", cursor, i+1,
			device.String(), hv.ID, formatIPAddresses(hv))

		if i == m.selectedDeviceIdx {
			line = errorStyle.Render(line) // Show selected item in red for deletion
		}
		s.WriteString(line + "\n")

		// Show additional details for selected item
		if i == m.selectedDeviceIdx {
			details := fmt.Sprintf("    UUID: %s", device.ID)
			details += fmt.Sprintf(" | Domain: %s", device.FailureDomain)
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(details) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate, enter/y delete, n cancel, esc back")

	return s.String()
}
func (m model) updatePartitionManagement(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.partitionMgmtCursor > 0 {
				m.partitionMgmtCursor--
			}
		case "down", "j":
			maxItems := 3 // Create, View, Delete
			if m.partitionMgmtCursor < maxItems-1 {
				m.partitionMgmtCursor++
			}
		case "enter", " ":
			switch m.partitionMgmtCursor {
			case 0: // Create Partition
				m.state = statePartitionCreate
				m.selectedPartitionIdx = -1
				m.message = ""
				// Initialize partition count input
				m.partitionCountInput = textinput.New()
				m.partitionCountInput.Placeholder = "Enter number of partitions (e.g., 4)"
				m.partitionCountInput.Focus()
				return m, nil
			case 1: // View Partitions
				m.state = statePartitionView
				m.selectedPartitionIdx = -1
				m.message = ""
				return m, nil
			case 2: // Delete Partition
				m.state = statePartitionDelete
				m.selectedPartitionIdx = -1
				m.message = ""
				return m, nil
			}
		case "esc":
			m.state = stateDeviceManagement
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewPartitionManagement() string {
	title := titleStyle.Render("Partition Management")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Select a partition management option:\n\n")

	managementItems := []string{
		"Create Partition - Add new NISD partition to device",
		"View Partitions - Display existing NISD partitions",
		"Delete Partition - Remove NISD partition",
	}

	for i, item := range managementItems {
		cursor := "  "
		if i == m.partitionMgmtCursor {
			cursor = "▶ "
			s.WriteString(selectedItemStyle.Render(cursor+item) + "\n")
		} else {
			s.WriteString(cursor + item + "\n")
		}
	}

	// Show summary of devices with partitions
	devices := m.getAllDevicesWithPartitions()
	totalPartitions := 0
	for _, deviceInfo := range devices {
		totalPartitions += len(deviceInfo.Device.Partitions)
	}

	s.WriteString(fmt.Sprintf("\nSummary: %d initialized devices, %d total partitions\n", len(devices), totalPartitions))
	s.WriteString("\nControls: ↑/↓ navigate, enter select, esc back to device management")

	return s.String()
}

// Partition Creation Implementation
func (m model) updatePartitionCreate(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedHypervisorIdx > 0 {
				m.selectedHypervisorIdx--
				m.selectedDeviceIdx = -1 // Reset device selection
			}
		case "down", "j":
			devices := m.getDevicesWithoutPartitions()
			if m.selectedHypervisorIdx < len(devices)-1 {
				m.selectedHypervisorIdx++
				m.selectedDeviceIdx = -1 // Reset device selection
			}
		case "enter", " ":
			devices := m.getDevicesWithoutPartitions()
			if m.selectedHypervisorIdx >= 0 && m.selectedHypervisorIdx < len(devices) {
				if m.selectedDeviceIdx == -1 {
					// Device selected, now fetch existing partitions and show them
					m.selectedDeviceIdx = m.selectedHypervisorIdx
					m.selectedDeviceForPartition = devices[m.selectedHypervisorIdx].Device
					m.selectedHvForPartition = Hypervisor{ID: devices[m.selectedHypervisorIdx].HvUUID, Name: devices[m.selectedHypervisorIdx].HvName}

					// Fetch existing partitions from the device
					hvFull, hvOk := m.findHypervisorByUUID(m.selectedHvForPartition.ID)
					if !hvOk {
						m.message = fmt.Sprintf("Hypervisor %s not found", m.selectedHvForPartition.ID)
						return m, nil
					}
					existingParts, err := GetDevicePartitionInfo(*hvFull, m.selectedDeviceForPartition.Name)
					if err != nil {
						m.message = fmt.Sprintf("Failed to get existing partitions: %v", err)
						return m, nil
					}

					if len(existingParts) == 0 {
						// No partitions found, ask user if they want to use whole device
						m.state = stateWholeDevicePrompt
						m.message = ""
						return m, nil
					}

					m.existingPartitions = existingParts
					m.selectedExistingPartitionIdx = 0
					m.selectedPartitions = make(map[int]bool) // Initialize selection map
					m.state = statePartitionKeyCreation
					return m, nil
				} else {
					// Number of partitions entered, create multiple partitions
					countStr := m.partitionCountInput.Value()
					if countStr == "" {
						m.message = "Number of partitions is required"
						return m, nil
					}

					numPartitions, err := strconv.Atoi(countStr)
					if err != nil || numPartitions <= 0 {
						m.message = "Invalid number of partitions. Please enter a positive number"
						return m, nil
					}

					if numPartitions > 20 {
						m.message = "Too many partitions. Maximum is 20 partitions"
						return m, nil
					}

					// Fetch existing partitions from the device
					hvFull2, hvOk2 := m.findHypervisorByUUID(m.selectedHvForPartition.ID)
					if !hvOk2 {
						m.message = fmt.Sprintf("Hypervisor %s not found", m.selectedHvForPartition.ID)
						return m, nil
					}
					existingParts, err := GetDevicePartitionInfo(*hvFull2, m.selectedDeviceForPartition.Name)
					if err != nil {
						m.message = fmt.Sprintf("Failed to get existing partitions: %v", err)
						return m, nil
					}

					if len(existingParts) == 0 {
						// No partitions found, ask user if they want to use whole device
						m.state = stateWholeDevicePrompt
						m.message = ""
						return m, nil
					}

					m.existingPartitions = existingParts
					m.selectedExistingPartitionIdx = 0
					m.selectedPartitions = make(map[int]bool) // Initialize selection map
					m.state = statePartitionKeyCreation
					return m, nil
				}
			}
		case "esc":
			if m.selectedDeviceIdx != -1 {
				// Cancel count input, go back to device selection
				m.selectedDeviceIdx = -1
				m.partitionCountInput.SetValue("")
				m.partitionCountInput.Blur()
			} else {
				// Exit to partition management
				m.state = statePartitionManagement
				m.selectedHypervisorIdx = -1
				m.message = ""
			}
			return m, nil
		}
	}

	// Update count input if it's focused
	if m.selectedDeviceIdx != -1 {
		m.partitionCountInput, cmd = m.partitionCountInput.Update(msg)
	}

	return m, cmd
}

func (m model) viewPartitionCreate() string {
	title := titleStyle.Render("Create NISD Partition")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") || strings.Contains(m.message, "Invalid") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	devices := m.getDevicesWithoutPartitions()

	if len(devices) == 0 {
		s.WriteString("No devices without partitions found.\n")
		s.WriteString("All devices already have partitions, or no devices have been added yet.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	// Device selection phase
	s.WriteString(fmt.Sprintf("Select device to partition (%d device(s) without partitions):\n\n", len(devices)))

	for i, deviceInfo := range devices {
		cursor := "  "
		if i == m.selectedHypervisorIdx {
			cursor = "▶ "
		}

		// Build device info line with better status display
		status := ""
		if deviceInfo.Device.State == ctlplfl.INITIALIZED {
			status = "[INIT]"
		} else {
			status = "[UNINITIALIZED]"
		}

		line := fmt.Sprintf("%s%s: %s %s", cursor, deviceInfo.HvName, deviceInfo.Device.Name, status)
		if deviceInfo.Device.Size != 0 {
			line += fmt.Sprintf(" (%s)", formatBytes(deviceInfo.Device.Size))
		}

		if i == m.selectedHypervisorIdx {
			line = selectedItemStyle.Render(line)
		}
		s.WriteString(line + "\n")
	}

	s.WriteString("\nControls: ↑/↓ navigate, enter select device, esc back")

	return s.String()
}

func (m model) updatePartitionView(msg tea.Msg) (model, tea.Cmd) {
	allPartitions := m.getAllPartitionsAcrossDevices()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedPartitionIdx > 0 {
				m.selectedPartitionIdx--
			}
		case "down", "j":
			if m.selectedPartitionIdx < len(allPartitions)-1 {
				m.selectedPartitionIdx++
			}
		case "esc":
			m.state = statePartitionManagement
			m.selectedPartitionIdx = -1
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewPartitionView() string {
	title := titleStyle.Render("View All NISD Partitions")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	allPartitions := m.getAllPartitionsAcrossDevices()

	if len(allPartitions) == 0 {
		s.WriteString("No NISD partitions found.\n")
		s.WriteString("Use 'Create Partition' to add NISD partitions to your devices.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	// Calculate dynamic pagination based on terminal height
	// Each partition takes ~2-3 lines (partition info + optional NISD UUID)
	itemsPerPage := m.calculateItemsPerPage(3)
	totalPages := (len(allPartitions) + itemsPerPage - 1) / itemsPerPage

	// Ensure cursor is within bounds
	if m.selectedPartitionIdx < 0 {
		m.selectedPartitionIdx = 0
	}
	if m.selectedPartitionIdx >= len(allPartitions) {
		m.selectedPartitionIdx = len(allPartitions) - 1
	}

	// Calculate which page the cursor is on
	currentPage := m.selectedPartitionIdx / itemsPerPage
	startIdx := currentPage * itemsPerPage
	endIdx := startIdx + itemsPerPage
	if endIdx > len(allPartitions) {
		endIdx = len(allPartitions)
	}

	s.WriteString(fmt.Sprintf("Found %d NISD partitions across all devices:\n", len(allPartitions)))

	// Show pagination info when there are multiple pages
	if totalPages > 1 {
		s.WriteString(fmt.Sprintf("Page %d of %d (showing items %d-%d) | Cursor at position %d\n\n",
			currentPage+1, totalPages, startIdx+1, endIdx, m.selectedPartitionIdx+1))
	} else {
		s.WriteString(fmt.Sprintf("Cursor at position %d of %d\n\n", m.selectedPartitionIdx+1, len(allPartitions)))
	}

	// Group by hypervisor and device for better organization
	hvDeviceGroups := make(map[string]map[string][]PartitionInfo)
	for _, partition := range allPartitions {
		if hvDeviceGroups[partition.HvName] == nil {
			hvDeviceGroups[partition.HvName] = make(map[string][]PartitionInfo)
		}
		hvDeviceGroups[partition.HvName][partition.DeviceName] = append(hvDeviceGroups[partition.HvName][partition.DeviceName], partition)
	}

	// Sort hypervisor names to ensure consistent ordering
	var hvNames []string
	for hvName := range hvDeviceGroups {
		hvNames = append(hvNames, hvName)
	}
	sort.Strings(hvNames)

	// Create a flat list of partitions for pagination, but maintain grouping info
	type DisplayPartition struct {
		PartitionInfo
		DisplayIndex int
		HvHeader     string
		DeviceHeader string
		IsFirstInHv  bool
		IsFirstInDev bool
	}

	var displayPartitions []DisplayPartition
	partitionIndex := 0
	for _, hvName := range hvNames {
		devices := hvDeviceGroups[hvName]

		// Sort device names to ensure consistent ordering
		var deviceNames []string
		for deviceName := range devices {
			deviceNames = append(deviceNames, deviceName)
		}
		sort.Strings(deviceNames)

		for _, deviceName := range deviceNames {
			partitions := devices[deviceName]

			// Sort partitions by PartitionID to ensure consistent ordering
			sort.Slice(partitions, func(i, j int) bool {
				return partitions[i].Partition.PartitionID < partitions[j].Partition.PartitionID
			})

			for j, partition := range partitions {
				displayPartitions = append(displayPartitions, DisplayPartition{
					PartitionInfo: partition,
					DisplayIndex:  partitionIndex,
					HvHeader:      hvName,
					DeviceHeader:  deviceName,
					IsFirstInHv:   partitionIndex == 0 || (partitionIndex > 0 && displayPartitions[partitionIndex-1].HvHeader != hvName),
					IsFirstInDev:  j == 0,
				})
				partitionIndex++
			}
		}
	}

	// Show only the partitions for the current page
	currentHv := ""
	currentDevice := ""

	for i := startIdx; i < endIdx && i < len(displayPartitions); i++ {
		dp := displayPartitions[i]

		// Show hypervisor header if needed
		if dp.HvHeader != currentHv {
			s.WriteString(fmt.Sprintf("Hypervisor: %s\n", dp.HvHeader))
			currentHv = dp.HvHeader
			currentDevice = ""
		}

		// Show device header if needed
		if dp.DeviceHeader != currentDevice {
			s.WriteString(fmt.Sprintf("  Device: %s\n", dp.DeviceHeader))
			currentDevice = dp.DeviceHeader
		}

		cursor := "    "
		if dp.DisplayIndex == m.selectedPartitionIdx {
			cursor = "  ▶ "
		}

		partitionLine := fmt.Sprintf("%sPartition: %s", cursor, dp.Partition.PartitionID)
		if dp.Partition.Size > 0 {
			partitionLine += fmt.Sprintf(" (%s)", formatBytes(dp.Partition.Size))
		}

		if dp.DisplayIndex == m.selectedPartitionIdx {
			partitionLine = selectedItemStyle.Render(partitionLine)
		}

		s.WriteString(partitionLine + "\n")
		if dp.DisplayIndex == m.selectedPartitionIdx {
			s.WriteString(fmt.Sprintf("      NISD UUID: %s\n", dp.Partition.NISDUUID))
		}

		// Add spacing after each partition
		if i < endIdx-1 && i < len(displayPartitions)-1 {
			nextDp := displayPartitions[i+1]
			if nextDp.DeviceHeader != dp.DeviceHeader || nextDp.HvHeader != dp.HvHeader {
				s.WriteString("\n")
			}
		}
	}

	s.WriteString("\nControls: ↑/↓ navigate, esc back")

	return s.String()
}

func (m model) updatePartitionDelete(msg tea.Msg) (model, tea.Cmd) {
	allPartitions := m.getAllPartitionsAcrossDevices()

	if len(allPartitions) == 0 {
		m.message = "No NISD partitions found to delete"
		m.state = statePartitionManagement
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedPartitionIdx > 0 {
				m.selectedPartitionIdx--
			}
		case "down", "j":
			if m.selectedPartitionIdx < len(allPartitions)-1 {
				m.selectedPartitionIdx++
			}
		case "enter", " ":
			if m.selectedPartitionIdx >= 0 && m.selectedPartitionIdx < len(allPartitions) {
				// Delete the selected partition
				selectedPartition := allPartitions[m.selectedPartitionIdx]

				hvDel, hvDelOk := m.findHypervisorByUUID(selectedPartition.HvUUID)
				if !hvDelOk {
					m.message = fmt.Sprintf("Hypervisor %s not found", selectedPartition.HvUUID)
				} else {
					err := RemoveDevicePartition(*hvDel, selectedPartition.DeviceName, selectedPartition.Partition.PartitionID)
					if err != nil {
						m.message = fmt.Sprintf("Failed to delete partition: %v", err)
					} else {
						m.message = fmt.Sprintf("Successfully deleted NISD partition: %s", selectedPartition.Partition.NISDUUID)
						// Reset selection index if it's now out of bounds
						newPartitions := m.getAllPartitionsAcrossDevices()
						if m.selectedPartitionIdx >= len(newPartitions) {
							m.selectedPartitionIdx = len(newPartitions) - 1
						}
						if m.selectedPartitionIdx < 0 {
							m.selectedPartitionIdx = 0
						}
					}
				}
			}
		case "esc":
			m.state = statePartitionManagement
			m.selectedPartitionIdx = -1
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewPartitionDelete() string {
	title := titleStyle.Render("Delete NISD Partition")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	allPartitions := m.getAllPartitionsAcrossDevices()

	if len(allPartitions) == 0 {
		s.WriteString("No NISD partitions found to delete.\n")
		s.WriteString("Use 'Create Partition' to add NISD partitions to your devices.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("Select partition to delete:\n")
	s.WriteString(errorStyle.Render("⚠️  WARNING: This action cannot be undone!") + "\n\n")

	// Group by hypervisor and device for better organization
	hvDeviceGroups := make(map[string]map[string][]PartitionInfo)
	for _, partition := range allPartitions {
		if hvDeviceGroups[partition.HvName] == nil {
			hvDeviceGroups[partition.HvName] = make(map[string][]PartitionInfo)
		}
		hvDeviceGroups[partition.HvName][partition.DeviceName] = append(hvDeviceGroups[partition.HvName][partition.DeviceName], partition)
	}

	// Sort hypervisor names to ensure consistent ordering
	var hvNames []string
	for hvName := range hvDeviceGroups {
		hvNames = append(hvNames, hvName)
	}
	sort.Strings(hvNames)

	partitionIndex := 0
	for _, hvName := range hvNames {
		devices := hvDeviceGroups[hvName]
		s.WriteString(fmt.Sprintf("Hypervisor: %s\n", hvName))

		// Sort device names to ensure consistent ordering
		var deviceNames []string
		for deviceName := range devices {
			deviceNames = append(deviceNames, deviceName)
		}
		sort.Strings(deviceNames)

		for _, deviceName := range deviceNames {
			partitions := devices[deviceName]
			s.WriteString(fmt.Sprintf("  Device: %s\n", deviceName))

			// Sort partitions by PartitionID to ensure consistent ordering
			sort.Slice(partitions, func(i, j int) bool {
				return partitions[i].Partition.PartitionID < partitions[j].Partition.PartitionID
			})

			for _, partition := range partitions {
				cursor := "    "
				if partitionIndex == m.selectedPartitionIdx {
					cursor = "  ▶ "
				}

				partitionLine := fmt.Sprintf("%sPartition: %s", cursor, partition.Partition.NISDUUID)
				if partition.Partition.Size > 0 {
					partitionLine += fmt.Sprintf(" (%s)", formatBytes(partition.Partition.Size))
				}

				if partitionIndex == m.selectedPartitionIdx {
					partitionLine = selectedItemStyle.Render(partitionLine)
					// Add warning for selected item
					partitionLine += " " + errorStyle.Render("← WILL BE DELETED")
				}

				s.WriteString(partitionLine + "\n")
				if partitionIndex == m.selectedPartitionIdx {
					s.WriteString(fmt.Sprintf("      UUID: %s\n", partition.Partition.PartitionID))
				}
				partitionIndex++
			}
			s.WriteString("\n")
		}
	}

	s.WriteString("Controls: ↑/↓ navigate, enter DELETE selected partition, esc cancel")

	return s.String()
}

func (m model) updateShowAddedPartition(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc":
			m.state = statePartitionManagement
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewShowAddedPartition() string {
	title := titleStyle.Render("Partition Key Created Successfully")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(successStyle.Render(m.message) + "\n\n")
	}

	s.WriteString("Partition Details:\n\n")
	s.WriteString(fmt.Sprintf("• Partition ID: %s\n", m.currentPartition.PartitionID))
	s.WriteString(fmt.Sprintf("• NISD Instance: %s\n", m.currentPartition.NISDUUID))
	if m.currentPartition.Size > 0 {
		s.WriteString(fmt.Sprintf("• Size: %s\n", formatBytes(m.currentPartition.Size)))
	}

	s.WriteString("\nUse 'View Partitions' to see all created partitions.")
	s.WriteString("\n\nPress enter or esc to return to partition management")

	return s.String()
}

// Partition Key Creation Methods
func (m model) updatePartitionKeyCreation(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedExistingPartitionIdx > 0 {
				m.selectedExistingPartitionIdx--
			}
		case "down", "j":
			if m.selectedExistingPartitionIdx < len(m.existingPartitions)-1 {
				m.selectedExistingPartitionIdx++
			}
		case " ":
			// Toggle selection for current partition
			if m.selectedExistingPartitionIdx >= 0 && m.selectedExistingPartitionIdx < len(m.existingPartitions) {
				m.selectedPartitions[m.selectedExistingPartitionIdx] = !m.selectedPartitions[m.selectedExistingPartitionIdx]
			}
		case "a":
			// Select all partitions
			for i := 0; i < len(m.existingPartitions); i++ {
				m.selectedPartitions[i] = true
			}
		case "n":
			// Deselect all partitions
			m.selectedPartitions = make(map[int]bool)
		case "enter":
			// Create partition keys for all selected partitions
			selectedCount := 0
			for _, selected := range m.selectedPartitions {
				if selected {
					selectedCount++
				}
			}

			if selectedCount == 0 {
				m.message = "No partitions selected. Use space to select partitions or 'a' to select all."
				return m, nil
			}

			var createdPartitions []DevicePartition
			var errorMessages []string

			// Process each selected partition
			for i, partitionInfo := range m.existingPartitions {
				if !m.selectedPartitions[i] {
					continue // Skip unselected partitions
				}

				// Create DevicePartition struct with PartitionID = partition name
				partition := DevicePartition{
					PartitionID: partitionInfo.Name,
					NISDUUID:    uuid.New().String(), // Generate new UUID for NISD instance
					Size:        partitionInfo.Size,  // Use the actual partition size
					DevID:       m.selectedDeviceForPartition.ID,
				}

				// Call PutPartition to create the partition key
				ptResp, err := m.cpClient.PutPartition(&partition)
				if err != nil {
					log.Infof("Failed to add partition to pumiceDB: %v", err)
					errorMessages = append(errorMessages, fmt.Sprintf("Failed to create key for %s: %v", partitionInfo.Name, err))
					continue
				} else if ptResp != nil && !ptResp.Success {
					errorMessages = append(errorMessages, fmt.Sprintf("Failed to create key for %s: %s", partitionInfo.Name, ptResp.Error))
					continue
				}

				// Save to configuration
				err = m.addPartitionToConfig(partition)
				if err != nil {
					errorMessages = append(errorMessages, fmt.Sprintf("Failed to save %s to config: %v", partitionInfo.Name, err))
					continue
				}

				createdPartitions = append(createdPartitions, partition)
			}

			if len(createdPartitions) > 0 {
				m.currentPartition = createdPartitions[0] // Set first created partition for display
				m.refreshCPData()
				m.state = stateShowAddedPartition
				if len(errorMessages) > 0 {
					m.message = fmt.Sprintf("Created %d partition keys successfully. Errors: %s", len(createdPartitions), strings.Join(errorMessages, "; "))
				} else {
					m.message = fmt.Sprintf("Successfully created %d partition keys", len(createdPartitions))
				}
			} else {
				m.message = fmt.Sprintf("Failed to create any partition keys. Errors: %s", strings.Join(errorMessages, "; "))
			}
			return m, nil
		case "esc":
			m.state = statePartitionManagement
			m.selectedExistingPartitionIdx = 0
			m.existingPartitions = nil
			m.selectedPartitions = nil
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewPartitionKeyCreation() string {
	title := titleStyle.Render("Create Partition Keys for Existing Partitions")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(fmt.Sprintf("Status: %s\n\n", m.message))
	}

	s.WriteString(fmt.Sprintf("Device: %s on %s\n\n", m.selectedDeviceForPartition.Name, m.selectedHvForPartition.Name))

	if len(m.existingPartitions) == 0 {
		s.WriteString("No existing partitions found.\n")
		s.WriteString("Please create partitions manually on the device first.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString(fmt.Sprintf("Found %d existing partitions. Select partitions to create keys:\n\n", len(m.existingPartitions)))

	for i, partitionInfo := range m.existingPartitions {
		cursor := "  "
		if i == m.selectedExistingPartitionIdx {
			cursor = selectedItemStyle.Render("→ ")
		}

		// Show selection status
		checkbox := "[ ]"
		if m.selectedPartitions[i] {
			checkbox = "[✓]"
		}

		// Format partition info with size
		sizeInfo := ""
		if partitionInfo.Size > 0 {
			sizeInfo = fmt.Sprintf(" (%s)", formatBytes(partitionInfo.Size))
		}

		s.WriteString(fmt.Sprintf("%s%s %d. %s%s\n", cursor, checkbox, i+1, partitionInfo.Name, sizeInfo))
	}

	s.WriteString("\n")
	s.WriteString("This will create partition keys with PartitionID = partition name\n")
	s.WriteString("and call PutPartition() to register them in the control plane.\n\n")
	s.WriteString("Controls:\n")
	s.WriteString("  ↑/↓ navigate\n")
	s.WriteString("  space toggle selection\n")
	s.WriteString("  a select all\n")
	s.WriteString("  n deselect all\n")
	s.WriteString("  enter create keys for selected partitions\n")
	s.WriteString("  esc back")

	return s.String()
}

// Helper function to add a single partition to the config
func (m model) addPartitionToConfig(_ DevicePartition) error {
	// Partition data is stored in the control plane; no local config needed.
	return nil
}

// Whole Device Prompt Methods
func (m model) updateWholeDevicePrompt(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			// User wants to use whole device - create partition key for it
			partition, err := m.createWholeDevicePartitionKey()
			if err != nil {
				m.message = fmt.Sprintf("Failed to create partition key for whole device: %v", err)
				m.state = statePartitionCreate
				return m, nil
			}

			m.refreshCPData()
			m.message = "Successfully created partition key for whole device"
			m.currentPartition = partition // Set the partition for display
			m.state = stateShowAddedPartition
			return m, nil
		case "n", "N", "esc":
			// User doesn't want to use whole device - go back to partition creation
			m.state = statePartitionCreate
			m.message = "No partition key created. Please create partitions manually or select a different device."
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewWholeDevicePrompt() string {
	title := titleStyle.Render("Device Has No Partitions")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString(fmt.Sprintf("Device: %s on %s\n", m.selectedDeviceForPartition.Name, m.selectedHvForPartition.Name))
	if m.selectedDeviceForPartition.Size > 0 {
		s.WriteString(fmt.Sprintf("Device Size: %s\n\n", formatBytes(m.selectedDeviceForPartition.Size)))
	} else {
		s.WriteString("\n")
	}
	s.WriteString("This device has no existing partitions.\n\n")
	s.WriteString("Do you want to use the whole device as a single partition for NISD?\n\n")
	s.WriteString("This will:\n")
	s.WriteString("• Create a partition key using the device name from /dev/disk/by-id/\n")
	s.WriteString("• Use the entire device capacity")
	if m.selectedDeviceForPartition.Size > 0 {
		s.WriteString(fmt.Sprintf(" (%s)", formatBytes(m.selectedDeviceForPartition.Size)))
	}
	s.WriteString("\n• Allow NISD to manage the whole device\n\n")
	s.WriteString("Controls:\n")
	s.WriteString("  Y yes, use whole device\n")
	s.WriteString("  N no, go back\n")
	s.WriteString("  esc cancel")

	return s.String()
}

// createWholeDevicePartitionKey creates a partition key for the whole device
func (m model) createWholeDevicePartitionKey() (DevicePartition, error) {
	// Get the device name from /dev/disk/by-id/
	hvWD, hvWDOk := m.findHypervisorByUUID(m.selectedHvForPartition.ID)
	if !hvWDOk {
		return DevicePartition{}, fmt.Errorf("hypervisor %s not found", m.selectedHvForPartition.ID)
	}
	deviceByIdName, err := getDeviceByIdName(*hvWD, m.selectedDeviceForPartition.Name)
	if err != nil {
		return DevicePartition{}, fmt.Errorf("failed to get device by-id name: %v", err)
	}

	// Create DevicePartition struct with device name as PartitionID and device size
	devicePartition := DevicePartition{
		PartitionID: deviceByIdName,
		NISDUUID:    uuid.New().String(),
		Size:        m.selectedDeviceForPartition.Size, // Store the entire device size
		DevID:       m.selectedDeviceForPartition.ID,
	}

	// Try to call PutPartition if control plane is enabled
	if m.cpClient != nil {
		log.Info("PutPartition for whole device: ", devicePartition)
		ptResp, err := m.cpClient.PutPartition(&devicePartition)
		if err != nil {
			return DevicePartition{}, fmt.Errorf("failed to call PutPartition for whole device: %v", err)
		}
		if ptResp != nil && !ptResp.Success {
			return DevicePartition{}, fmt.Errorf("PutPartition for whole device failed: %s", ptResp.Error)
		}
	} else {
		log.Error("Failed to write partition key in CP: ", devicePartition)
	}

	// Add partition to configuration
	err = m.addPartitionToConfig(devicePartition)
	if err != nil {
		return DevicePartition{}, fmt.Errorf("failed to add whole device partition to config: %v", err)
	}

	// Return the created partition for display
	return devicePartition, nil
}

// PDU Management Methods
func (m model) updatePDUManagement(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.pduManagementCursor > 0 {
				m.pduManagementCursor--
			}
		case "down", "j":
			maxItems := 4 // Add, View, Edit, Delete
			if len(m.cpPDUs) == 0 {
				maxItems = 1 // Only Add available
			}
			if m.pduManagementCursor < maxItems-1 {
				m.pduManagementCursor++
			}
		case "enter", " ":
			switch m.pduManagementCursor {
			case 0: // Add PDU
				m.state = statePDUForm
				m.editingUUID = "" // Clear editing UUID for new PDU
				m.resetInputsForPDU()
				m.message = ""
				return m, textinput.Blink
			case 1: // View PDU
				m.state = stateViewPDU
				m.pduListCursor = 0
				m.cpPDURefresh = true // Trigger CP query
				m.message = "Loading PDU details from Control Plane..."
				return m, nil
			case 2: // Edit PDU
				if len(m.cpPDUs) > 0 {
					m.state = stateEditPDU
					m.pduListCursor = 0
					m.message = ""
					return m, nil
				}
			case 3: // Delete PDU
				if len(m.cpPDUs) > 0 {
					m.state = stateDeletePDU
					m.pduListCursor = 0
					m.message = ""
					return m, nil
				}
			}
		}
	}
	return m, nil
}

func (m model) updatePDUForm(msg tea.Msg) (model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			s := msg.String()
			if s == "up" || s == "shift+tab" {
				if m.focusedInput > 0 {
					m.focusedInput--
				} else {
					m.focusedInput = inputPowerCapacity
				}
			} else {
				if m.focusedInput < inputPowerCapacity {
					m.focusedInput++
				} else {
					m.focusedInput = inputName
				}
			}

			for i := range m.inputs {
				if i == int(m.focusedInput) {
					cmds = append(cmds, m.inputs[i].Focus())
					continue
				}
				m.inputs[i].Blur()
			}
			return m, tea.Batch(cmds...)

		case "enter":
			name := strings.TrimSpace(m.inputs[0].Value())
			description := strings.TrimSpace(m.inputs[1].Value())   // description - TODO: use when implementing full PDU creation
			location := strings.TrimSpace(m.inputs[2].Value())      // location - TODO: use when implementing full PDU creation
			powerCapacity := strings.TrimSpace(m.inputs[3].Value()) // powerCapacity - TODO: use when implementing full PDU creation

			if name == "" {
				m.message = "PDU Name is required"
				return m, nil
			}

			pduID := m.editingUUID
			if pduID == "" {
				pduID = uuid.New().String()
			}
			pdu := PDU{
				ID:            pduID,
				Name:          name,
				Location:      location,
				PowerCapacity: powerCapacity,
				Specification: description,
				Racks:         []Rack{},
			}

			log.Info("Adding PDU: ", pdu)
			pduResp, err := m.cpClient.PutPDU(&pdu)
			if err != nil {
				m.message = fmt.Sprintf("Failed to add PDU %s: %v", pdu.Name, err)
			} else if pduResp != nil && !pduResp.Success {
				m.message = fmt.Sprintf("Failed to add PDU: %s", pduResp.Error)
			} else {
				m.currentPDU = pdu
				m.message = fmt.Sprintf("Added PDU %s to control plane", m.currentPDU.Name)
				m.refreshCPData()
				m.state = stateShowAddedPDU
				return m, nil
			}
		}
	}

	// Update the focused input
	var cmd tea.Cmd
	m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)

	return m, cmd
}

func (m model) updateEditPDU(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.pduListCursor > 0 {
				m.pduListCursor--
			}
		case "down", "j":
			if m.pduListCursor < len(m.cpPDUs)-1 {
				m.pduListCursor++
			}
		case "enter", " ":
			// Load selected PDU for editing
			if m.pduListCursor < len(m.cpPDUs) {
				pdu := m.cpPDUs[m.pduListCursor]
				m.editingUUID = pdu.ID
				m.currentPDU = pdu
				m.loadPDUIntoForm(pdu)
				m.state = statePDUForm
				return m, textinput.Blink
			}
		}
	}
	return m, nil
}

func (m model) updateDeletePDU(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.pduListCursor > 0 {
				m.pduListCursor--
			}
		case "down", "j":
			if m.pduListCursor < len(m.cpPDUs)-1 {
				m.pduListCursor++
			}
		case "enter", " ", "y":
			// Delete selected PDU (CP has no delete API - show informational message)
			if m.pduListCursor < len(m.cpPDUs) {
				pdu := m.cpPDUs[m.pduListCursor]
				m.message = fmt.Sprintf("PDU '%s' deletion not supported via control plane API", pdu.Name)
			}
			m.state = statePDUManagement
			return m, nil
		case "n":
			// Cancel deletion
			m.state = statePDUManagement
			m.message = "Deletion cancelled"
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateViewPDU(msg tea.Msg) (model, tea.Cmd) {
	// Query Control Plane for PDU data when refresh is needed
	if m.cpPDURefresh {
		log.Info("Refreshing PDUs from Control Plane...")
		log.Info("  cpEnabled: ", m.cpEnabled)
		log.Info("  cpClient is nil: ", m.cpClient == nil)
		log.Info("  cpRaftUUID: ", m.cpRaftUUID)
		log.Info("  cpGossipPath: ", m.cpGossipPath)

		// If control plane client is available, fetch PDU data from CP
		if m.cpClient != nil {
			log.Info("Calling GetPDUs with GetAll=true")
			cpPDUs, err := m.cpClient.GetPDUs(&ctlplfl.GetReq{GetAll: true})
			if err != nil {
				// Log error and show empty list
				log.Error("Failed to query PDUs from Control Plane: ", err)
				m.message = "Error: Could not fetch PDU data from Control Plane: " + err.Error()
				m.cpPDUs = []ctlplfl.PDU{} // Show empty list on error
			} else {
				// Show only CP data, no merging with config file
				m.cpPDUs = cpPDUs
				m.message = fmt.Sprintf("PDU data loaded from Control Plane (%d PDUs)", len(cpPDUs))
				log.Info("Successfully retrieved ", len(cpPDUs), " PDUs from Control Plane")
			}
		} else {
			m.message = "Control Plane client not initialized"
			m.cpPDUs = []ctlplfl.PDU{} // Show empty list when no CP client
			log.Warn("cpClient is nil, cannot query Control Plane")
		}

		m.cpPDURefresh = false
	}

	// Always use CP PDUs (empty if CP unavailable)
	pdus := m.cpPDUs

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.pduListCursor > 0 {
				m.pduListCursor--
			}
		case "down", "j":
			if m.pduListCursor < len(pdus)-1 {
				m.pduListCursor++
			}
		case "r":
			// Refresh PDU data from Control Plane
			m.cpPDURefresh = true
			m.message = "Refreshing PDU data from Control Plane..."
			return m, nil
		case "enter", " ":
			// Just navigate through PDUs, no action needed
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateShowAddedPDU(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ", "esc":
			// Return to PDU management
			m.state = statePDUManagement
			return m, nil
		}
	}
	return m, nil
}

// Rack Management Methods
func (m model) updateRackManagement(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.rackManagementCursor > 0 {
				m.rackManagementCursor--
			}
		case "down", "j":
			maxItems := 4 // Add, View, Edit, Delete
			totalRacks := 0
			for _, pdu := range m.cpPDUs {
				totalRacks += len(pdu.Racks)
			}
			if totalRacks == 0 {
				maxItems = 1 // Only Add available
			}
			if m.rackManagementCursor < maxItems-1 {
				m.rackManagementCursor++
			}
		case "enter", " ":
			switch m.rackManagementCursor {
			case 0: // Add Rack
				m.refreshCPData()
				if len(m.cpPDUs) > 0 {
					m.state = stateRackForm
					m.editingUUID = "" // Clear editing UUID for new rack
					m.resetInputsForRack()
					m.selectedPDUIdx = 0
					m.message = ""
					return m, textinput.Blink
				} else {
					m.message = "No PDUs configured. Please add a PDU first."
				}
			case 1: // View Rack
				m.state = stateViewRack
				m.rackListCursor = 0
				m.message = ""
				// Trigger initial CP refresh when entering View Rack
				m.cpRackRefresh = true
				return m, nil
			case 2: // Edit Rack
				totalRacks := 0
				for _, pdu := range m.cpPDUs {
					totalRacks += len(pdu.Racks)
				}
				if totalRacks > 0 {
					m.state = stateEditRack
					m.rackListCursor = 0
					m.message = ""
					return m, nil
				}
			case 3: // Delete Rack
				totalRacks := 0
				for _, pdu := range m.cpPDUs {
					totalRacks += len(pdu.Racks)
				}
				if totalRacks > 0 {
					m.state = stateDeleteRack
					m.rackListCursor = 0
					m.message = ""
					return m, nil
				}
			}
		}
	}
	return m, nil
}

func (m model) updateRackForm(msg tea.Msg) (model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			s := msg.String()
			if s == "up" || s == "shift+tab" {
				if m.focusedInput > 0 {
					m.focusedInput--
				} else {
					m.focusedInput = inputLocation
				}
			} else {
				if m.focusedInput < inputLocation {
					m.focusedInput++
				} else {
					m.focusedInput = inputName
				}
			}

			for i := range m.inputs {
				if i == int(m.focusedInput) {
					cmds = append(cmds, m.inputs[i].Focus())
					continue
				}
				m.inputs[i].Blur()
			}
			return m, tea.Batch(cmds...)

		case "enter":
			name := strings.TrimSpace(m.inputs[0].Value())
			description := strings.TrimSpace(m.inputs[1].Value()) // description - TODO: use when implementing full Rack creation
			location := strings.TrimSpace(m.inputs[2].Value())    // location - TODO: use when implementing full Rack creation

			if name == "" {
				m.message = "Rack Name is required"
				return m, nil
			}

			if m.selectedPDUIdx >= len(m.cpPDUs) {
				m.message = "Invalid PDU selection"
				return m, nil
			}

			pdu := m.cpPDUs[m.selectedPDUIdx]
			rackID := m.editingUUID
			if rackID == "" {
				rackID = uuid.New().String()
			}
			rack := Rack{
				ID:            rackID,
				Name:          name,
				PDUID:         pdu.ID,
				Location:      location,
				Specification: description,
			}

			log.Info("Adding Rack: ", rack)
			rackResp, err := m.cpClient.PutRack(&rack)
			if err != nil {
				m.message = fmt.Sprintf("Failed to add rack to pumiceDB: %v", err)
				return m, nil
			}
			if rackResp != nil && !rackResp.Success {
				m.message = fmt.Sprintf("Failed to add rack: %s", rackResp.Error)
				return m, nil
			}
			m.currentRack = rack
			m.refreshCPData()
			m.message = fmt.Sprintf("Added rack %s to PDU %s", m.currentRack.Name, pdu.Name)
			m.state = stateShowAddedRack
			return m, nil
		case "p", "P": // Open PDU selection menu
			if len(m.cpPDUs) > 0 {
				m.pduSelectionCursor = m.selectedPDUIdx // Start at currently selected PDU
				m.state = stateRackPDUSelection
				return m, nil
			}
		}
	}

	// Update the focused input
	var cmd tea.Cmd
	m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)

	return m, cmd
}

func (m model) updateEditRack(msg tea.Msg) (model, tea.Cmd) {
	allRacks := m.getAllRacks()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.rackListCursor > 0 {
				m.rackListCursor--
			}
		case "down", "j":
			if m.rackListCursor < len(allRacks)-1 {
				m.rackListCursor++
			}
		case "enter", " ":
			// Load selected rack for editing
			if m.rackListCursor < len(allRacks) {
				rackInfo := allRacks[m.rackListCursor]
				rack := rackInfo.Rack
				m.editingUUID = rack.ID
				m.currentRack = rack
				m.selectedPDUIdx = rackInfo.PDUIndex
				m.loadRackIntoForm(rack)
				m.state = stateRackForm
				return m, textinput.Blink
			}
		}
	}
	return m, nil
}

func (m model) updateDeleteRack(msg tea.Msg) (model, tea.Cmd) {
	allRacks := m.getAllRacks()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.rackListCursor > 0 {
				m.rackListCursor--
			}
		case "down", "j":
			if m.rackListCursor < len(allRacks)-1 {
				m.rackListCursor++
			}
		case "enter", " ", "y":
			// Delete selected rack
			if m.rackListCursor < len(allRacks) {
				m.message = "Rack deletion is not supported via the control plane API"
				m.state = stateRackManagement
				return m, nil
			}
		case "n":
			// Cancel deletion
			m.state = stateRackManagement
			m.message = "Deletion cancelled"
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateViewRack(msg tea.Msg) (model, tea.Cmd) {
	// Query CP for rack data if needed
	if m.cpClient != nil && m.cpRackRefresh {
		log.Info("  cpRaftUUID: ", m.cpRaftUUID)
		log.Info("  cpGossipPath: ", m.cpGossipPath)

		// If control plane client is available, fetch Rack data from CP
		if m.cpClient != nil {
			log.Info("Calling GetRacks with GetAll=true")
			cpRacks, err := m.cpClient.GetRacks(&ctlplfl.GetReq{GetAll: true})
			if err != nil {
				// Log error and show empty list
				log.Error("Failed to query Racks from Control Plane: ", err)
				m.message = "Error: Could not fetch Rack data from Control Plane: " + err.Error()
				m.cpRacks = []ctlplfl.Rack{} // Show empty list on error
			} else {
				// Show only CP data, no merging with config file
				m.cpRacks = cpRacks
				m.message = fmt.Sprintf("Rack data loaded from Control Plane (%d Racks)", len(cpRacks))
				log.Info("Successfully retrieved ", len(cpRacks), " Racks from Control Plane")
			}
		} else {
			m.message = "Control Plane client not initialized"
			m.cpRacks = []ctlplfl.Rack{} // Show empty list when no CP client
			log.Warn("cpClient is nil, cannot query Control Plane")
		}

		m.cpRackRefresh = false
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.rackListCursor > 0 {
				m.rackListCursor--
			}
		case "down", "j":
			if m.rackListCursor < len(m.cpRacks)-1 {
				m.rackListCursor++
			}
		case "r":
			// Refresh rack data from Control Plane
			m.cpRackRefresh = true
			m.message = "Refreshing rack data from Control Plane..."
		case "enter", " ":
			// Just navigate through racks, no action needed
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateShowAddedRack(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ", "esc":
			// Return to Rack management
			m.state = stateRackManagement
			return m, nil
		}
	}
	return m, nil
}

// Device Partitioning Method
func (m model) updateDevicePartitioning(msg tea.Msg) (model, tea.Cmd) {
	// Handle NISD device partitioning
	return m, nil
}

// PDU View Methods
func (m model) viewPDUManagement() string {
	title := titleStyle.Render("PDU Management")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Select an action:\n\n")

	managementItems := []string{
		"Add PDU - Create new Power Distribution Unit",
	}

	if len(m.cpPDUs) > 0 {
		managementItems = append(managementItems,
			"View PDU - Display PDU details and hierarchy",
			"Edit PDU - Modify existing PDU",
			"Delete PDU - Remove PDU and all its racks from configuration")
	}

	for i, item := range managementItems {
		cursor := "  "
		if i == m.pduManagementCursor {
			cursor = "▶ "
			s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, item)))
		} else {
			s.WriteString(fmt.Sprintf("%s%d. %s", cursor, i+1, item))
		}
		s.WriteString("\n")
	}

	s.WriteString(fmt.Sprintf("\nCurrent PDUs: %d\n\n", len(m.cpPDUs)))
	s.WriteString("Controls: ↑/↓ navigate, enter select, esc back to main menu")

	return s.String()
}

func (m model) viewPDUForm() string {
	var title string
	if m.editingUUID != "" {
		title = titleStyle.Render("Edit PDU")
	} else {
		title = titleStyle.Render("Add New PDU")
	}

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	labels := []string{"Name:", "Description:", "Location:", "Power Capacity:"}

	for i, input := range m.inputs[:4] { // Only show PDU-relevant inputs
		if i < len(labels) {
			s.WriteString(labels[i] + "\n")

			if i == int(m.focusedInput) {
				s.WriteString(focusedStyle.Render(input.View()) + "\n\n")
			} else {
				s.WriteString(blurredStyle.Render(input.View()) + "\n\n")
			}
		}
	}

	s.WriteString("Controls: tab/↑/↓ navigate fields, enter submit, esc back")

	return s.String()
}

func (m model) viewEditPDU() string {
	title := titleStyle.Render("Edit PDU")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	if len(m.cpPDUs) == 0 {
		s.WriteString("No PDUs available to edit.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("Select PDU to edit:\n\n")

	for i, pdu := range m.cpPDUs {
		cursor := "  "
		if i == m.pduListCursor {
			cursor = "▶ "
		}

		line := fmt.Sprintf("%s%d. %s", cursor, i+1, pdu.Name)
		if pdu.Location != "" {
			line += fmt.Sprintf(" (%s)", pdu.Location)
		}

		if i == m.pduListCursor {
			line = selectedItemStyle.Render(line)
		}
		s.WriteString(line + "\n")

		// Show additional details for selected item
		if i == m.pduListCursor {
			details := fmt.Sprintf("    UUID: %s", pdu.ID)
			if pdu.PowerCapacity != "0" {
				details += fmt.Sprintf(" | Power: %s", pdu.PowerCapacity)
			}
			details += fmt.Sprintf(" | Racks: %d", len(pdu.Racks))
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(details) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate, enter edit selected, esc back")

	return s.String()
}

func (m model) viewDeletePDU() string {
	title := titleStyle.Render("Delete PDU")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	if len(m.cpPDUs) == 0 {
		s.WriteString("No PDUs available to delete.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("⚠️  WARNING: This will permanently delete the PDU and ALL its racks and hypervisors!\n\n")
	s.WriteString("Select PDU to delete:\n\n")

	for i, pdu := range m.cpPDUs {
		cursor := "  "
		if i == m.pduListCursor {
			cursor = "▶ "
		}

		line := fmt.Sprintf("%s%d. %s", cursor, i+1, pdu.Name)
		if pdu.Location != "" {
			line += fmt.Sprintf(" (%s)", pdu.Location)
		}

		if i == m.pduListCursor {
			line = errorStyle.Render(line) // Show selected item in red for deletion
		}
		s.WriteString(line + "\n")

		// Show additional details for selected item
		if i == m.pduListCursor {
			rackCount := len(pdu.Racks)
			hvCount := 0
			for _, rack := range pdu.Racks {
				hvCount += len(rack.Hypervisors)
			}
			details := fmt.Sprintf("    UUID: %s", pdu.ID)
			details += fmt.Sprintf(" | %d racks, %d hypervisors will be removed", rackCount, hvCount)
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(details) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate, enter/y delete, n cancel, esc back")

	return s.String()
}

func (m model) viewViewPDU() string {
	title := titleStyle.Render("View PDU Details")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	// Always use only CP PDUs - no fallback to config file
	pdus := m.cpPDUs
	log.Info("m.cpPDUs in viewViewPDU: ", m.cpPDUs)

	// Show data source indicator and CP status
	if m.cpClient != nil && len(m.cpPDUs) > 0 {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("📡 Data from Control Plane") + "\n")
	} else if m.cpClient != nil {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("⚠️  Control Plane connected, no PDU data available") + "\n")
	} else if m.cpEnabled {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("❌ Control Plane enabled but client not initialized") + "\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(fmt.Sprintf("    RaftUUID: %s", m.cpRaftUUID)) + "\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(fmt.Sprintf("    GossipPath: %s", m.cpGossipPath)) + "\n")
	} else {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("❌ Control Plane not enabled") + "\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("    Use: ./niova-ctl -cp -raft-uuid <uuid> -gossip-path <path>") + "\n")
	}

	if len(pdus) == 0 {
		s.WriteString("\nNo PDUs available from Control Plane.\n\n")
		s.WriteString("Press r to refresh, esc to go back")
		return s.String()
	}
	s.WriteString("\n")

	s.WriteString("PDU Details:\n\n")

	for i, pdu := range pdus {
		cursor := "  "
		if i == m.pduListCursor {
			cursor = "▶ "
		}

		// PDU header
		headerLine := fmt.Sprintf("%s%d. %s", cursor, i+1, pdu.Name)
		if pdu.Location != "" {
			headerLine += fmt.Sprintf(" (%s)", pdu.Location)
		}

		if i == m.pduListCursor {
			s.WriteString(selectedItemStyle.Render(headerLine) + "\n")

			// Detailed view for selected PDU
			s.WriteString(fmt.Sprintf("    Name: %s\n", pdu.Name))
			s.WriteString(fmt.Sprintf("    UUID: %s\n", pdu.ID))
			if pdu.Specification != "" {
				s.WriteString(fmt.Sprintf("    Description: %s\n", pdu.Specification))
			}
			if pdu.Location != "" {
				s.WriteString(fmt.Sprintf("    Location: %s\n", pdu.Location))
			}
			if pdu.PowerCapacity != "0" {
				s.WriteString(fmt.Sprintf("    Power Capacity: %s\n", pdu.PowerCapacity))
			}

			rackCount := len(pdu.Racks)
			s.WriteString(fmt.Sprintf("    Racks: %d\n", rackCount))

			if rackCount > 0 {
				totalHypervisors := 0
				totalDevices := 0
				for _, rack := range pdu.Racks {
					totalHypervisors += len(rack.Hypervisors)
					for _, hv := range rack.Hypervisors {
						totalDevices += len(hv.Dev)
					}
				}
				s.WriteString(fmt.Sprintf("    Total Hypervisors: %d\n", totalHypervisors))
				s.WriteString(fmt.Sprintf("    Total Devices: %d\n", totalDevices))

				s.WriteString("\n    Rack Details:\n")
				for j, rack := range pdu.Racks {
					s.WriteString(fmt.Sprintf("      %d. %s", j+1, rack.ID))
					if rack.Location != "" {
						s.WriteString(fmt.Sprintf(" (%s)", rack.Location))
					}
					s.WriteString(fmt.Sprintf(" - %d hypervisors\n", len(rack.Hypervisors)))

					for k, hv := range rack.Hypervisors {
						s.WriteString(fmt.Sprintf("         %d. %s (%s) - %d devices\n",
							k+1, hv.ID, formatIPAddresses(hv), len(hv.Dev)))
					}
				}
			} else {
				s.WriteString("    No racks configured\n")
			}
		} else {
			s.WriteString(headerLine + "\n")
			// Show summary when not selected
			summary := fmt.Sprintf("    %d racks", len(pdu.Racks))
			if pdu.PowerCapacity != "0" {
				summary += fmt.Sprintf(" | %s", pdu.PowerCapacity)
			}
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(summary) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate PDUs, r refresh from Control Plane, esc back to PDU management")

	return s.String()
}

// Rack View Methods
func (m model) viewRackManagement() string {
	title := titleStyle.Render("Rack Management")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Select an action:\n\n")

	managementItems := []string{
		"Add Rack - Create new server rack",
	}

	totalRacks := 0
	for _, pdu := range m.cpPDUs {
		totalRacks += len(pdu.Racks)
	}

	if totalRacks > 0 {
		managementItems = append(managementItems,
			"View Rack - Display rack details and hierarchy",
			"Edit Rack - Modify existing rack",
			"Delete Rack - Remove rack and all its hypervisors from configuration")
	}

	for i, item := range managementItems {
		cursor := "  "
		if i == m.rackManagementCursor {
			cursor = "▶ "
			s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, item)))
		} else {
			s.WriteString(fmt.Sprintf("%s%d. %s", cursor, i+1, item))
		}
		s.WriteString("\n")
	}

	s.WriteString(fmt.Sprintf("\nCurrent PDUs: %d\n", len(m.cpPDUs)))
	s.WriteString(fmt.Sprintf("Current Racks: %d\n\n", totalRacks))
	s.WriteString("Controls: ↑/↓ navigate, enter select, esc back to main menu")

	return s.String()
}

func (m model) viewRackForm() string {
	var title string
	if m.editingUUID != "" {
		title = titleStyle.Render("Edit Rack")
	} else {
		title = titleStyle.Render("Add New Rack")
	}

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	// Show PDU selection with dropdown-style interface
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("═══ PDU SELECTION ═══") + "\n")
	if len(m.cpPDUs) > 0 {
		if m.selectedPDUIdx >= 0 && m.selectedPDUIdx < len(m.cpPDUs) {
			pdu := m.cpPDUs[m.selectedPDUIdx]
			s.WriteString(fmt.Sprintf("📍 Selected PDU: %s", selectedItemStyle.Render(pdu.Name)))
			if pdu.Location != "" {
				s.WriteString(fmt.Sprintf(" (%s)", pdu.Location))
			}
			s.WriteString("\n")
		} else {
			s.WriteString(errorStyle.Render("⚠️  No PDU selected") + "\n")
		}
		if len(m.cpPDUs) > 1 {
			s.WriteString(fmt.Sprintf("   📋 %d PDUs available - Press 'P' to select different PDU\n", len(m.cpPDUs)))
		}
	} else {
		s.WriteString(errorStyle.Render("⚠️  No PDUs available - Create PDUs first") + "\n")
	}
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("═══════════════════") + "\n\n")

	labels := []string{"Name:", "Description:", "Location:"}

	for i, input := range m.inputs[:3] { // Only show rack-relevant inputs
		if i < len(labels) {
			s.WriteString(labels[i] + "\n")

			if i == int(m.focusedInput) {
				s.WriteString(focusedStyle.Render(input.View()) + "\n\n")
			} else {
				s.WriteString(blurredStyle.Render(input.View()) + "\n\n")
			}
		}
	}

	s.WriteString("Controls: tab/↑/↓ navigate fields, " +
		lipgloss.NewStyle().Bold(true).Render("P select PDU") +
		", enter submit, esc back")

	return s.String()
}

func (m model) viewEditRack() string {
	title := titleStyle.Render("Edit Rack")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	allRacks := m.getAllRacks()

	if len(allRacks) == 0 {
		s.WriteString("No racks available to edit.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("Select rack to edit:\n\n")

	for i, rackInfo := range allRacks {
		cursor := "  "
		if i == m.rackListCursor {
			cursor = "▶ "
		}

		line := fmt.Sprintf("%s%d. %s", cursor, i+1, rackInfo.Rack.Name)
		if rackInfo.Rack.Location != "" {
			line += fmt.Sprintf(" (%s)", rackInfo.Rack.Location)
		}
		line += fmt.Sprintf(" [PDU: %s]", rackInfo.PDUName)

		if i == m.rackListCursor {
			line = selectedItemStyle.Render(line)
		}
		s.WriteString(line + "\n")

		// Show additional details for selected item
		if i == m.rackListCursor {
			rack := rackInfo.Rack
			details := fmt.Sprintf("    UUID: %s", rack.ID)
			if rack.Specification != "" {
				details += fmt.Sprintf(" | Desc: %s", rack.Specification)
			}
			details += fmt.Sprintf(" | Hypervisors: %d", len(rack.Hypervisors))
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(details) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate, enter edit selected, esc back")

	return s.String()
}

func (m model) viewDeleteRack() string {
	title := titleStyle.Render("Delete Rack")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	allRacks := m.getAllRacks()

	if len(allRacks) == 0 {
		s.WriteString("No racks available to delete.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("⚠️  WARNING: This will permanently delete the rack and ALL its hypervisors!\n\n")
	s.WriteString("Select rack to delete:\n\n")

	for i, rackInfo := range allRacks {
		cursor := "  "
		if i == m.rackListCursor {
			cursor = "▶ "
		}

		line := fmt.Sprintf("%s%d. %s", cursor, i+1, rackInfo.Rack.Name)
		if rackInfo.Rack.Location != "" {
			line += fmt.Sprintf(" (%s)", rackInfo.Rack.Location)
		}
		line += fmt.Sprintf(" [PDU: %s]", rackInfo.PDUName)

		if i == m.rackListCursor {
			line = errorStyle.Render(line) // Show selected item in red for deletion
		}
		s.WriteString(line + "\n")

		// Show additional details for selected item
		if i == m.rackListCursor {
			rack := rackInfo.Rack
			hvCount := len(rack.Hypervisors)
			deviceCount := 0
			for _, hv := range rack.Hypervisors {
				deviceCount += len(hv.Dev)
			}
			details := fmt.Sprintf("    UUID: %s", rack.ID)
			details += fmt.Sprintf(" | %d hypervisors, %d devices will be removed", hvCount, deviceCount)
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(details) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate, enter/y delete, n cancel, esc back")

	return s.String()
}

func (m model) viewViewRack() string {
	title := titleStyle.Render("View Rack Details")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	// Always use only CP Racks - no fallback to config file
	racks := m.cpRacks
	log.Info("m.cpRacks in viewViewRack: ", m.cpRacks)

	// Show data source indicator and CP status
	if m.cpClient != nil && len(m.cpRacks) > 0 {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("📡 Data from Control Plane") + "\n")
	} else if m.cpClient != nil {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("⚠️  Control Plane connected, no Rack data available") + "\n")
	} else if m.cpEnabled {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("❌ Control Plane enabled but client not initialized") + "\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(fmt.Sprintf("    RaftUUID: %s", m.cpRaftUUID)) + "\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(fmt.Sprintf("    GossipPath: %s", m.cpGossipPath)) + "\n")
	} else {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("❌ Control Plane not enabled") + "\n")
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("    Use: ./niova-ctl -cp -raft-uuid <uuid> -gossip-path <path>") + "\n")
	}

	if len(racks) == 0 {
		s.WriteString("\nNo Racks available from Control Plane.\n\n")
		s.WriteString("Press r to refresh, esc to go back")
		return s.String()
	}

	s.WriteString("Rack Details:\n\n")

	for i, rack := range racks {
		cursor := "  "
		if i == m.rackListCursor {
			cursor = "▶ "
		}

		// Rack header
		headerLine := fmt.Sprintf("%s%d. %s", cursor, i+1, rack.Name)
		if rack.Location != "" {
			headerLine += fmt.Sprintf(" (%s)", rack.Location)
		}
		headerLine += fmt.Sprintf(" [ID: %s]", rack.ID)

		if i == m.rackListCursor {
			s.WriteString(selectedItemStyle.Render(headerLine) + "\n")

			// Detailed view for selected rack
			s.WriteString(fmt.Sprintf("    UUID: %s\n", rack.ID))
			s.WriteString(fmt.Sprintf("    Name: %s\n", rack.Name))
			s.WriteString(fmt.Sprintf("    PDUID: %s\n", rack.PDUID))
			if rack.Specification != "" {
				s.WriteString(fmt.Sprintf("    Description: %s\n", rack.Specification))
			}
			if rack.Location != "" {
				s.WriteString(fmt.Sprintf("    Location: %s\n", rack.Location))
			}

			hvCount := len(rack.Hypervisors)
			s.WriteString(fmt.Sprintf("    Hypervisors: %d\n", hvCount))

			if hvCount > 0 {
				totalDevices := 0
				for _, hv := range rack.Hypervisors {
					totalDevices += len(hv.Dev)
				}
				s.WriteString(fmt.Sprintf("    Total Devices: %d\n", totalDevices))

				s.WriteString("\n    Hypervisor Details:\n")
				for j, hv := range rack.Hypervisors {
					s.WriteString(fmt.Sprintf("      %d. %s (%s) - %d devices\n",
						j+1, hv.ID, formatIPAddresses(hv), len(hv.Dev)))
					if hv.PortRange != "" {
						s.WriteString(fmt.Sprintf("         Port Range: %s\n", hv.PortRange))
					}
				}
			} else {
				s.WriteString("    No hypervisors configured\n")
			}
		} else {
			s.WriteString(headerLine + "\n")
			// Show summary when not selected
			summary := fmt.Sprintf("    %d hypervisors", len(rack.Hypervisors))
			if rack.Specification != "" {
				summary += fmt.Sprintf(" | %s", rack.Specification)
			}
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(summary) + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate racks, r refresh, esc back to rack management")

	return s.String()
}

func (m model) viewDevicePartitioning() string {
	return "Device Partitioning - Coming Soon"
}

// Helper Methods
func (m model) resetInputsForPDU() model {
	for i := range m.inputs {
		m.inputs[i].SetValue("")
		if i == 0 {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	m.focusedInput = inputName
	return m
}

func (m model) loadPDUIntoForm(pdu PDU) model {
	m.inputs[0].SetValue(pdu.Name)
	m.inputs[1].SetValue(pdu.Specification)
	m.inputs[2].SetValue(pdu.Location)
	m.inputs[3].SetValue(pdu.PowerCapacity)
	m.focusedInput = inputName
	m.inputs[0].Focus()
	return m
}

// Show Added Unit View Methods
func (m model) viewShowAddedPDU() string {
	title := titleStyle.Render("PDU Added Successfully")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	pdu := m.currentPDU
	if pdu.ID != "" {
		s.WriteString(lipgloss.NewStyle().Bold(true).Render("PDU Details:") + "\n\n")
		s.WriteString(fmt.Sprintf("Name: %s\n", pdu.Name))
		s.WriteString(fmt.Sprintf("UUID: %s\n", pdu.ID))

		if pdu.Location != "" {
			s.WriteString(fmt.Sprintf("Location: %s\n", pdu.Location))
		}
		if pdu.PowerCapacity != "0" {
			s.WriteString(fmt.Sprintf("Power Capacity: %s\n", pdu.PowerCapacity))
		}
		if pdu.Specification != "" {
			s.WriteString(fmt.Sprintf("Description: %s\n", pdu.Specification))
		}

		s.WriteString(fmt.Sprintf("Racks: %d\n", len(pdu.Racks)))
	} else {
		s.WriteString(errorStyle.Render("Error: PDU not found") + "\n")
	}

	s.WriteString("\nPress Enter to continue or ESC to go to main menu")
	return s.String()
}

func (m model) viewShowAddedRack() string {
	title := titleStyle.Render("Rack Added Successfully")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	rack := m.currentRack

	if rack.ID != "" {
		s.WriteString(lipgloss.NewStyle().Bold(true).Render("Rack Details:") + "\n\n")
		s.WriteString(fmt.Sprintf("Name: %s\n", rack.ID))
		s.WriteString(fmt.Sprintf("UUID: %s\n", rack.ID))

		// Find parent PDU name
		var parentPDUName string
		for _, pdu := range m.cpPDUs {
			for _, r := range pdu.Racks {
				if r.ID == rack.ID {
					parentPDUName = pdu.Name
					break
				}
			}
			if parentPDUName != "" {
				break
			}
		}
		if parentPDUName != "" {
			s.WriteString(fmt.Sprintf("Parent PDU: %s\n", parentPDUName))
		}

		if rack.Location != "" {
			s.WriteString(fmt.Sprintf("Location: %s\n", rack.Location))
		}
		if rack.Specification != "" {
			s.WriteString(fmt.Sprintf("Description: %s\n", rack.Specification))
		}

		s.WriteString(fmt.Sprintf("Hypervisors: %d\n", len(rack.Hypervisors)))
	} else {
		s.WriteString(errorStyle.Render("Error: Rack not found") + "\n")
	}

	s.WriteString("\nPress Enter to continue or ESC to go to main menu")
	return s.String()
}

func (m model) viewShowAddedHypervisor() string {
	title := titleStyle.Render("Hypervisor Added Successfully")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	hv := m.currentHv
	if hv.ID != "" {
		s.WriteString(lipgloss.NewStyle().Bold(true).Render("Hypervisor Details:") + "\n\n")
		s.WriteString(fmt.Sprintf("Name: %s\n", hv.ID))
		s.WriteString(fmt.Sprintf("UUID: %s\n", hv.ID))
		s.WriteString(fmt.Sprintf("%s\n", formatDetailedIPAddresses(hv)))
		s.WriteString(fmt.Sprintf("SSH Port: %s\n", hv.SSHPort))

		// Find parent hierarchy
		var parentPDUName, parentRackName string
		var isLegacy bool

		// Check hierarchical hypervisors first
		for _, pdu := range m.cpPDUs {
			for _, rack := range pdu.Racks {
				for _, hvInRack := range rack.Hypervisors {
					if hvInRack.ID == hv.ID {
						parentPDUName = pdu.Name
						parentRackName = rack.ID
						break
					}
				}
				if parentRackName != "" {
					break
				}
			}
			if parentPDUName != "" {
				break
			}
		}

		// If not found, check legacy hypervisors
		if parentPDUName == "" {
			for _, legacyHv := range m.cpHypervisors {
				if legacyHv.ID == hv.ID {
					isLegacy = true
					break
				}
			}
		}

		if !isLegacy && parentPDUName != "" && parentRackName != "" {
			s.WriteString(fmt.Sprintf("Parent Rack: %s\n", parentRackName))
			s.WriteString(fmt.Sprintf("Parent PDU: %s\n", parentPDUName))

			// Show failure domain hierarchy
			failureDomain := fmt.Sprintf("%s → %s → %s", parentPDUName, parentRackName, hv.ID)
			s.WriteString(fmt.Sprintf("Failure Domain: %s\n", failureDomain))
		} else {
			s.WriteString("Type: Legacy (not in hierarchy)\n")
		}

		if hv.PortRange != "" {
			s.WriteString(fmt.Sprintf("Port Range: %s\n", hv.PortRange))
		}

		s.WriteString(fmt.Sprintf("Devices: %d\n", len(hv.Dev)))
	} else {
		s.WriteString(errorStyle.Render("Error: Hypervisor not found") + "\n")
	}

	s.WriteString("\nPress Enter to continue or ESC to go to main menu")
	return s.String()
}

// Rack helper methods
func (m model) resetInputsForRack() model {
	for i := range m.inputs {
		m.inputs[i].SetValue("")
		if i == 0 {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	m.focusedInput = inputName
	return m
}

func (m model) loadRackIntoForm(rack Rack) model {
	m.inputs[0].SetValue(rack.ID)
	m.inputs[1].SetValue(rack.Specification)
	m.inputs[2].SetValue(rack.Location)
	m.focusedInput = inputName
	m.inputs[0].Focus()
	return m
}

// RackInfo holds rack and its location in the config
type RackInfo struct {
	Rack     Rack
	PDUIndex int
	PDUName  string
}

// Helper function to get all racks across all PDUs
func (m model) getAllRacks() []RackInfo {
	var racks []RackInfo
	for pduIndex, pdu := range m.cpPDUs {
		for _, rack := range pdu.Racks {
			racks = append(racks, RackInfo{
				Rack:     rack,
				PDUIndex: pduIndex,
				PDUName:  pdu.Name,
			})
		}
	}
	return racks
}

// HypervisorInfo contains hypervisor information with location context
type HypervisorInfo struct {
	Hypervisor ctlplfl.Hypervisor
	Location   string    // "Rack: <RackID>" or "Legacy"
	Source     string    // "rack" or "legacy"
	RackInfo   *RackInfo // nil for legacy hypervisors
}

// Helper function to get all hypervisors from both hierarchical and legacy sources
func (m model) getAllHypervisors() []HypervisorInfo {
	var hypervisors []HypervisorInfo

	// Get hypervisors from hierarchical structure (in racks)
	for pduIndex, pdu := range m.cpPDUs {
		for _, rack := range pdu.Racks {
			for _, hv := range rack.Hypervisors {
				rackInfo := &RackInfo{
					Rack:     rack,
					PDUIndex: pduIndex,
					PDUName:  pdu.Name,
				}
				hypervisors = append(hypervisors, HypervisorInfo{
					Hypervisor: hv,
					Location:   fmt.Sprintf("Rack: %s", rack.ID),
					Source:     "rack",
					RackInfo:   rackInfo,
				})
			}
		}
	}

	// Get legacy hypervisors
	for _, hv := range m.cpHypervisors {
		hypervisors = append(hypervisors, HypervisorInfo{
			Hypervisor: hv,
			Location:   "Legacy",
			Source:     "legacy",
			RackInfo:   nil,
		})
	}

	return hypervisors
}

// PDU Selection for Rack Form
func (m model) updateRackPDUSelection(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.pduSelectionCursor > 0 {
				m.pduSelectionCursor--
			}
		case "down", "j":
			if m.pduSelectionCursor < len(m.cpPDUs)-1 {
				m.pduSelectionCursor++
			}
		case "enter", " ":
			// Select the PDU and return to rack form
			m.selectedPDUIdx = m.pduSelectionCursor
			m.state = stateRackForm
			return m, nil
		case "esc":
			// Cancel selection and return to rack form
			m.state = stateRackForm
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewRackPDUSelection() string {
	title := titleStyle.Render("Select PDU for Rack")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if len(m.cpPDUs) == 0 {
		s.WriteString(errorStyle.Render("No PDUs available") + "\n")
		s.WriteString("Press ESC to go back\n")
		return s.String()
	}

	s.WriteString("Choose a PDU to assign this rack to:\n\n")

	for i, pdu := range m.cpPDUs {
		cursor := "  "
		if i == m.pduSelectionCursor {
			cursor = "▶ "
		}

		style := lipgloss.NewStyle()
		if i == m.pduSelectionCursor {
			style = selectedItemStyle
		}

		line := fmt.Sprintf("%s%s", cursor, pdu.Name)
		if pdu.Location != "" {
			line += fmt.Sprintf(" (%s)", pdu.Location)
		}
		if pdu.PowerCapacity != "0" {
			line += fmt.Sprintf(" - %s", pdu.PowerCapacity)
		}

		// Show rack count
		line += fmt.Sprintf(" [%d racks]", len(pdu.Racks))

		s.WriteString(style.Render(line) + "\n")
	}

	s.WriteString("\nControls: ↑/↓ navigate, Enter select, ESC cancel")
	return s.String()
}

// Rack Selection for Hypervisor Form
func (m model) updateHypervisorRackSelection(msg tea.Msg) (model, tea.Cmd) {
	allRacks := m.getAllRacks()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.rackSelectionCursor > 0 {
				m.rackSelectionCursor--
			}
		case "down", "j":
			if m.rackSelectionCursor < len(allRacks)-1 {
				m.rackSelectionCursor++
			}
		case "enter", " ":
			// Select the rack and return to hypervisor form
			if m.rackSelectionCursor < len(allRacks) {
				m.selectedRackIdx = m.rackSelectionCursor
			}
			m.state = stateHypervisorForm
			return m, nil
		case "esc":
			// Cancel selection and return to hypervisor form
			m.state = stateHypervisorForm
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewHypervisorRackSelection() string {
	title := titleStyle.Render("Select Rack for Hypervisor")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	allRacks := m.getAllRacks()
	if len(allRacks) == 0 {
		s.WriteString(errorStyle.Render("No racks available") + "\n")
		s.WriteString("Hypervisor will be added as legacy (not in hierarchy)\n")
		s.WriteString("Press ESC to go back\n")
		return s.String()
	}

	s.WriteString("Choose a rack to assign this hypervisor to:\n\n")

	for i, rackInfo := range allRacks {
		cursor := "  "
		if i == m.rackSelectionCursor {
			cursor = "▶ "
		}

		style := lipgloss.NewStyle()
		if i == m.rackSelectionCursor {
			style = selectedItemStyle
		}

		line := fmt.Sprintf("%s%s", cursor, rackInfo.Rack.Name)
		if rackInfo.Rack.Location != "" {
			line += fmt.Sprintf(" (%s)", rackInfo.Rack.Location)
		}
		line += fmt.Sprintf(" [PDU: %s]", rackInfo.PDUName)

		// Show hypervisor count
		line += fmt.Sprintf(" - %d hypervisors", len(rackInfo.Rack.Hypervisors))

		s.WriteString(style.Render(line) + "\n")
	}

	s.WriteString("\nControls: ↑/↓ navigate, Enter select, ESC cancel")
	return s.String()
}

/*
// WriteDevice update functions
func (m model) updateWriteDevice(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.state = stateMenu
		m.message = ""
		return m, nil
	case "enter":
		m.state = stateWriteDeviceForm
		// Focus first input
		for i := range m.writeDeviceInputs {
			m.writeDeviceInputs[i].Blur()
		}
		if len(m.writeDeviceInputs) > 0 {
			m.writeDeviceInputs[0].Focus()
		}
		m.writeDeviceCursor = 0
		return m, textinput.Blink
	}
	return m, nil
}

func (m model) updateWriteDeviceForm(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.state = stateWriteDevice
		m.message = ""
		return m, nil
	case "tab", "shift+tab", "up", "down":
		s := msg.String()

		// Navigate between inputs
		if s == "up" || s == "shift+tab" {
			m.writeDeviceCursor--
		} else {
			m.writeDeviceCursor++
		}

		if m.writeDeviceCursor < 0 {
			m.writeDeviceCursor = len(m.writeDeviceInputs) - 1
		} else if m.writeDeviceCursor >= len(m.writeDeviceInputs) {
			m.writeDeviceCursor = 0
		}

		// Update focus
		for i := range m.writeDeviceInputs {
			if i == m.writeDeviceCursor {
				m.writeDeviceInputs[i].Focus()
			} else {
				m.writeDeviceInputs[i].Blur()
			}
		}
		return m, textinput.Blink
	case "enter":
		// Submit the form
		return m.submitWriteDevice()
	}

	// Handle input updates
	var cmd tea.Cmd
	m.writeDeviceInputs[m.writeDeviceCursor], cmd = m.writeDeviceInputs[m.writeDeviceCursor].Update(msg)
	return m, cmd
}

func (m model) submitWriteDevice() (model, tea.Cmd) {
	// Check if control plane is available
	if !m.cpEnabled || m.cpClient == nil {
		m.message = "Control plane not available - enable with -cp flag and provide -raft-uuid and -gossip-path"
		m.state = stateWriteDevice
		return m, nil
	}

	// Create Device from form inputs
	deviceInfo := ctlplfl.Device{
		ID:            m.writeDeviceInputs[0].Value(),
		SerialNumber:  m.writeDeviceInputs[2].Value(),
		HypervisorID:  m.writeDeviceInputs[4].Value(),
		FailureDomain: m.writeDeviceInputs[5].Value(),
	}

	// Parse status field
	if statusStr := m.writeDeviceInputs[3].Value(); statusStr != "" {
		if status, err := strconv.ParseUint(statusStr, 10, 16); err == nil {
			deviceInfo.Status = uint16(status)
		}
	}

	log.Info("Sending device info to control plane: ", deviceInfo)

	devResp, err := m.cpClient.PutDeviceInfo(&deviceInfo)
	if err != nil {
		m.message = fmt.Sprintf("Error writing device to control plane: %v", err)
		log.Error("Failed to write device info: ", err)
	} else if devResp != nil && !devResp.Success {
		m.message = fmt.Sprintf("Error writing device to control plane: %s", devResp.Error)
		log.Error("Control plane rejected device write: ", devResp.Error)
	} else {
		m.message = fmt.Sprintf("Successfully wrote device %s to control plane", deviceInfo.ID)
		log.Info("Successfully wrote device info to control plane")

		// Clear form inputs
		for i := range m.writeDeviceInputs {
			m.writeDeviceInputs[i].SetValue("")
		}
	}

	m.state = stateWriteDevice
	return m, nil
}

// WriteDevice view functions
func (m model) viewWriteDevice() string {
	title := titleStyle.Render("Write Device to Control Plane")

	content := fmt.Sprintf(`
%s

This will send device configuration data to the niova-mdsvc control plane.

Press 'enter' to continue or 'q' to go back.

%s`, title, m.message)

	return content
}

func (m model) viewWriteDeviceForm() string {
	title := titleStyle.Render("Device Information Form")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	labels := []string{"Device ID:", "NISD ID:", "Serial Number:", "Status:", "HyperVisor ID:", "Failure Domain:"}

	for i, label := range labels {
		if i == m.writeDeviceCursor {
			s.WriteString(focusedStyle.Render(fmt.Sprintf("%-15s", label)) + " ")
		} else {
			s.WriteString(fmt.Sprintf("%-15s ", label))
		}
		s.WriteString(m.writeDeviceInputs[i].View() + "\n\n")
	}

	s.WriteString("\nPress 'tab' to navigate, 'enter' to submit, 'q' to cancel\n")

	if m.message != "" {
		s.WriteString("\n" + m.message + "\n")
	}

	return s.String()
}
*/

// NISD Management Functions

func (m model) updateNISDManagement(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.nisdMgmtCursor > 0 {
				m.nisdMgmtCursor--
			}
		case "down", "j":
			maxItems := 3 // Initialize NISD, Start NISD, View All NISDs
			if m.nisdMgmtCursor < maxItems-1 {
				m.nisdMgmtCursor++
			}
		case "enter", " ":
			switch m.nisdMgmtCursor {
			case 0: // Initialize NISD
				m.state = stateNISDPartitionSelection
				m.selectedNISDHypervisorIdx = -1
				m.selectedNISDDeviceIdx = -1
				m.selectedNISDPartitionIdx = -1
				m.selectedNISDPartitions = make(map[int]bool)
				m.message = ""
				return m, nil
			case 1: // Start NISD
				m.state = stateNISDSelection
				m.selectedNISDForStart = -1
				m.message = ""
				return m, nil
			case 2: // View All NISDs
				m.state = stateViewAllNISDs
				m.selectedNISDIdx = 0 // Initialize cursor
				m.message = ""
				return m, nil
			}
		case "esc":
			m.state = stateMenu
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewNISDManagement() string {
	title := titleStyle.Render("NISD Management")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Select a NISD management option:\n\n")

	managementItems := []string{
		"Initialize NISD - Initialize a NISD instance on a device partition",
		"Start NISD - Start an existing NISD process",
		"View All NISDs - Query and display all NISD details from control plane",
	}

	for i, item := range managementItems {
		cursor := "  "
		if i == m.nisdMgmtCursor {
			cursor = "▶ "
			s.WriteString(selectedItemStyle.Render(cursor+item) + "\n")
		} else {
			s.WriteString(cursor + item + "\n")
		}
	}

	s.WriteString("\nControls: ↑/↓ navigate, enter select, esc back to main menu")

	return s.String()
}

func (m model) updateNISDPartitionSelection(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedNISDHypervisorIdx > 0 {
				m.selectedNISDHypervisorIdx--
				m.selectedNISDDeviceIdx = -1
				m.selectedNISDPartitionIdx = -1
			}
		case "down", "j":
			partitions := m.getAllPartitionsForNISD()
			if m.selectedNISDHypervisorIdx < len(partitions)-1 {
				m.selectedNISDHypervisorIdx++
				m.selectedNISDDeviceIdx = -1
				m.selectedNISDPartitionIdx = -1
			}
		case " ":
			// Toggle selection of current partition
			partitions := m.getAllPartitionsForNISD()
			if m.selectedNISDHypervisorIdx >= 0 && m.selectedNISDHypervisorIdx < len(partitions) {
				if m.selectedNISDPartitions[m.selectedNISDHypervisorIdx] {
					delete(m.selectedNISDPartitions, m.selectedNISDHypervisorIdx)
				} else {
					m.selectedNISDPartitions[m.selectedNISDHypervisorIdx] = true
				}
			}
			return m, nil
		case "a", "A":
			// Select all partitions
			partitions := m.getAllPartitionsForNISD()
			m.selectedNISDPartitions = make(map[int]bool)
			for i := range partitions {
				m.selectedNISDPartitions[i] = true
			}
			return m, nil
		case "d", "D":
			// Deselect all partitions
			m.selectedNISDPartitions = make(map[int]bool)
			return m, nil
		case "enter":
			// Proceed to confirmation if at least one partition is selected
			selectedCount := 0
			for _, selected := range m.selectedNISDPartitions {
				if selected {
					selectedCount++
				}
			}
			if selectedCount == 0 {
				m.message = "Please select at least one partition"
				return m, nil
			}
			m.state = stateNISDInitialize
			m.message = ""
			return m, nil
		case "esc":
			m.state = stateNISDManagement
			m.selectedNISDHypervisorIdx = -1
			m.selectedNISDDeviceIdx = -1
			m.selectedNISDPartitionIdx = -1
			m.selectedNISDPartitions = make(map[int]bool)
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewNISDPartitionSelection() string {
	title := titleStyle.Render("Select Partition for NISD")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	partitions := m.getAllPartitionsForNISD()

	if len(partitions) == 0 {
		s.WriteString("No partitions available for NISD initialization.\n")
		s.WriteString("Please create device partitions first.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("Select partitions to initialize as NISD:\n\n")

	for i, partitionInfo := range partitions {
		cursor := "  "
		if i == m.selectedNISDHypervisorIdx {
			cursor = "▶ "
		}

		// Show checkbox for selection state
		checkbox := "[ ]"
		if m.selectedNISDPartitions[i] {
			checkbox = "[✓]"
		}

		// Display partition info
		info := fmt.Sprintf("%s HV: %s | Device: %s | Partition: %s | Size: %d bytes",
			checkbox,
			partitionInfo.HvName,
			partitionInfo.Device.Name,
			partitionInfo.Partition.PartitionID,
			partitionInfo.Partition.Size)

		if i == m.selectedNISDHypervisorIdx {
			s.WriteString(selectedItemStyle.Render(cursor+info) + "\n")
		} else {
			s.WriteString(cursor + info + "\n")
		}
	}

	selectedCount := 0
	for _, selected := range m.selectedNISDPartitions {
		if selected {
			selectedCount++
		}
	}
	s.WriteString(fmt.Sprintf("\nSelected: %d partition(s)", selectedCount))
	s.WriteString("\nControls: ↑/↓ navigate, space toggle, enter confirm, a select all, d deselect all, esc back")

	return s.String()
}

func (m model) updateNISDInitialize(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			err := (&m).initializeSelectedNISDs()
			if err != nil {
				m.message = fmt.Sprintf("Failed to initialize NISDs: %v", err)
				return m, nil
			}

			m.state = stateShowInitializedNISD
			selectedCount := 0
			for _, selected := range m.selectedNISDPartitions {
				if selected {
					selectedCount++
				}
			}
			m.message = fmt.Sprintf("%d NISD(s) initialized successfully!", selectedCount)
			return m, nil
		case "esc":
			m.state = stateNISDPartitionSelection
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewNISDInitialize() string {
	title := titleStyle.Render("Initialize NISD")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	partitions := m.getAllPartitionsForNISD()
	selectedCount := 0
	for _, selected := range m.selectedNISDPartitions {
		if selected {
			selectedCount++
		}
	}

	s.WriteString(fmt.Sprintf("Ready to initialize NISD on %d selected partition(s):\n\n", selectedCount))

	// Display selected partition details
	for i, partitionInfo := range partitions {
		if m.selectedNISDPartitions[i] {
			s.WriteString(fmt.Sprintf("• HV: %s | Device: %s | Partition: %s | Size: %d bytes\n",
				partitionInfo.HvName,
				partitionInfo.Device.Name,
				partitionInfo.Partition.NISDUUID,
				partitionInfo.Partition.Size))
		}
	}

	s.WriteString("\nThis will initialize NISD instances using each partition's existing UUID and PartitionID.\n")
	s.WriteString("Each NISD will be registered with the control plane using:\n")
	s.WriteString("- DevID: partition ID\n")
	s.WriteString("- NISD ID: partition UUID\n\n")
	s.WriteString("Press ENTER to initialize all selected NISDs, or ESC to cancel")

	return s.String()
}

func (m model) updateShowInitializedNISD(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ", "esc":
			m.state = stateNISDManagement
			m.selectedNISDHypervisorIdx = -1
			m.selectedNISDDeviceIdx = -1
			m.selectedNISDPartitionIdx = -1
			m.selectedNISDPartitions = make(map[int]bool)
			m.selectedNISDForStart = -1
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewShowInitializedNISD() string {
	title := titleStyle.Render("NISD Initialized Successfully")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(successStyle.Render(m.message) + "\n\n")
	}

	s.WriteString("NISD Details:\n")
	s.WriteString(fmt.Sprintf("  NISD UUID: %s\n", m.currentNISD.ID))
	s.WriteString(fmt.Sprintf("  Device ID: %s\n", m.currentNISD.FailureDomain[ctlplfl.DEVICE_IDX]))
	s.WriteString(fmt.Sprintf("  Hypervisor ID: %s\n", m.currentNISD.FailureDomain[ctlplfl.HV_IDX]))
	s.WriteString(fmt.Sprintf("  Client Port: %d\n", m.currentNISD.NetInfo[0].Port)) // Add support to store multiple Network Interfaces
	s.WriteString(fmt.Sprintf("  Server Port: %d\n", m.currentNISD.PeerPort))
	s.WriteString(fmt.Sprintf("  IP Address: %s\n", m.currentNISD.NetInfo[0].IPAddr))
	s.WriteString(fmt.Sprintf("  Total Size: %d bytes\n", m.currentNISD.TotalSize))

	s.WriteString("\nThe NISD instance has been registered with the control plane.\n\n")
	s.WriteString("Press any key to return to NISD management")

	return s.String()
}

/*
// Helper function to initialize NISD
func (m *model) initializeNISD() error {

	if m.cpClient == nil {
		return fmt.Errorf("control plane client is not initialized")
	}

	// Use NISD UUID from the selected partition
	nisdUUID := m.selectedPartitionForNISD.NISDUUID

	// Get hypervisor info
	hv, found := m.findHypervisorByUUID(m.selectedHvForNISD.ID)
	if !found {
		return fmt.Errorf("hypervisor %s not found", m.selectedHvForNISD.ID)
	}

	// Get Rack info to populate PDU-ID in NISD
	rack, found := m.findRackByUUID(hv.RackID)
	if !found {
		return fmt.Errorf("rack %s not found", hv.RackID)
	}

	// Allocate ports for NISD
	clientPort, serverPort, err := AllocatePortPair(m.selectedHvForNISD.ID, hv.PortRange, m.userToken(), m.cpClient)
	if err != nil {
		return fmt.Errorf("failed to allocate ports for NISD: %v", err)
	}
	netInfos := make([]ctlplfl.NetworkInfo, 0, len(hv.IPAddrs))
	for _, ip := range hv.IPAddrs {
		netInfos = append(netInfos, ctlplfl.NetworkInfo{
			IPAddr: ip,
			Port:   uint16(clientPort),
		})
	}

	// Create NISD struct using DevicePartition data
	nisd := &ctlplfl.Nisd{
		ID:            nisdUUID, // Use NISD UUID from partition
		PeerPort:      uint16(serverPort),
		TotalSize:     m.selectedPartitionForNISD.Size,
		AvailableSize: m.selectedPartitionForNISD.Size,
		NetInfo:       netInfos,
		FailureDomain: []string{
			rack.PDUID,
			hv.RackID,
			m.selectedHvForNISD.ID,
			m.selectedPartitionForNISD.DevID,
			m.selectedPartitionForNISD.PartitionID,
		},
		NetInfoCnt: len(netInfos),
	}

	// Call PutNisd
	resp, err := m.cpClient.PutNisd(nisd)
	if err != nil {
		return fmt.Errorf("failed to register NISD with control plane: %v", err)
	}

	log.Info("PutNisd: ", nisd)

	if resp == nil {
		return fmt.Errorf("received nil response from control plane")
	}
	if !resp.Success {
		return fmt.Errorf("failed to register NISD: %s", resp.Error)
	}

	// Store the initialized NISD for display
	m.currentNISD = *nisd

	return nil
}
*/

func (m *model) initializeSelectedNISDs() error {
	if m.cpClient == nil {
		return fmt.Errorf("control plane client is not initialized")
	}

	partitions := m.getAllPartitionsForNISD()
	var errors []string
	successCount := 0

	for i, partitionInfo := range partitions {
		if !m.selectedNISDPartitions[i] {
			continue // Skip unselected partitions
		}

		// Get hypervisor info
		hv, found := m.findHypervisorByUUID(partitionInfo.HvUUID)
		if !found {
			errors = append(errors, fmt.Sprintf("hypervisor %s not found for partition %s", partitionInfo.HvName, partitionInfo.Partition.NISDUUID))
			continue
		}

		// Get Rack info to populate PDU-ID in NISD
		rack, found := m.findRackByUUID(hv.RackID)
		if !found {
			return fmt.Errorf("rack %s not found", hv.RackID)
		}

		// Allocate ports for NISD
		clientPort, serverPort, err := AllocatePortPair(partitionInfo.HvUUID, hv.PortRange, m.userToken(), m.cpClient)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to allocate ports for NISD %s: %v", partitionInfo.Partition.NISDUUID, err))
			continue
		}

		netInfos := make([]ctlplfl.NetworkInfo, 0, len(hv.IPAddrs))
		for _, ip := range hv.IPAddrs {
			netInfos = append(netInfos, ctlplfl.NetworkInfo{
				IPAddr: ip,
				Port:   uint16(clientPort),
			})
		}

		// Create NISD struct using DevicePartition data
		nisd := &ctlplfl.Nisd{
			ID:            partitionInfo.Partition.NISDUUID, // Use NISD UUID from partition
			PeerPort:      uint16(serverPort),
			TotalSize:     partitionInfo.Partition.Size,
			AvailableSize: partitionInfo.Partition.Size,
			NetInfo:       netInfos,
			FailureDomain: []string{
				rack.PDUID,
				hv.RackID,
				partitionInfo.HvUUID,
				partitionInfo.Partition.DevID,
				partitionInfo.Partition.PartitionID,
			},
			NetInfoCnt: len(netInfos),
		}

		// Call PutNisd
		resp, err := m.cpClient.PutNisd(nisd)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to register NISD %s with control plane: %v", partitionInfo.Partition.NISDUUID, err))
			continue
		}

		log.Info("PutNisd: ", nisd)

		if resp == nil {
			errors = append(errors, fmt.Sprintf("received nil response from control plane for NISD %s", partitionInfo.Partition.NISDUUID))
			continue
		}
		if !resp.Success {
			errors = append(errors, fmt.Sprintf("failed to register NISD %s: %s", partitionInfo.Partition.NISDUUID, resp.Error))
			continue
		}

		successCount++
		// Store the last successfully initialized NISD for display (could be enhanced to store multiple)
		m.currentNISD = *nisd
	}

	if len(errors) > 0 {
		if successCount == 0 {
			return fmt.Errorf("all NISD initializations failed: %s", strings.Join(errors, "; "))
		} else {
			log.Warnf("Some NISD initializations failed: %s", strings.Join(errors, "; "))
		}
	}

	return nil
}

// NISD Selection for Starting

func (m model) updateNISDSelection(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedNISDForStart > 0 {
				m.selectedNISDForStart--
			}
		case "down", "j":
			nisdList, _ := m.getNISDsList()
			if m.selectedNISDForStart < len(nisdList)-1 {
				m.selectedNISDForStart++
			}
		case "enter", " ":
			nisdList, _ := m.getNISDsList()
			if m.selectedNISDForStart >= 0 && m.selectedNISDForStart < len(nisdList) {
				m.selectedNISDToStart = nisdList[m.selectedNISDForStart]
				m.state = stateNISDStart
				m.message = ""
				return m, nil
			}
		case "esc":
			m.state = stateNISDManagement
			m.selectedNISDForStart = -1
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewNISDSelection() string {
	title := titleStyle.Render("Select NISD to Start")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	nisdList, _ := m.getNISDsList()

	if len(nisdList) == 0 {
		s.WriteString("No initialized NISD instances found.\n")
		s.WriteString("Please initialize a NISD first using 'Initialize NISD' option.\n\n")
		s.WriteString("Press esc to go back")
		return s.String()
	}

	s.WriteString("Select NISD instance to start:\n\n")

	for i, nisd := range nisdList {
		cursor := "  "
		if i == m.selectedNISDForStart {
			cursor = "▶ "
		}

		// Display NISD info
		// TODO: Print IPAddr/Port Info
		info := fmt.Sprintf("UUID: %s | Device: %s | HV: %s | Ports: %d",
			nisd.ID,
			nisd.FailureDomain[ctlplfl.DEVICE_IDX],
			nisd.FailureDomain[ctlplfl.HV_IDX],
			nisd.PeerPort)

		if i == m.selectedNISDForStart {
			s.WriteString(selectedItemStyle.Render(cursor+info) + "\n")
		} else {
			s.WriteString(cursor + info + "\n")
		}
	}

	s.WriteString("\nControls: ↑/↓ navigate, enter select, esc back")

	return s.String()
}

func (m model) updateNISDStart(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			// Start the NISD process
			err := m.startNISDProcess()
			if err != nil {
				m.message = fmt.Sprintf("Failed to start NISD: %v", err)
				return m, nil
			}

			m.state = stateShowStartedNISD
			m.message = "NISD process started successfully!"
			return m, nil
		case "esc":
			m.state = stateNISDSelection
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewNISDStart() string {
	title := titleStyle.Render("Start NISD Process")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Ready to start NISD process with the following configuration:\n\n")

	// Display selected NISD details
	s.WriteString(fmt.Sprintf("NISD UUID: %s\n", m.selectedNISDToStart.ID))
	s.WriteString(fmt.Sprintf("Device ID: %s\n", m.selectedNISDToStart.FailureDomain[ctlplfl.DEVICE_IDX]))
	s.WriteString(fmt.Sprintf("Hypervisor ID: %s\n", m.selectedNISDToStart.FailureDomain[ctlplfl.HV_IDX]))
	// s.WriteString(fmt.Sprintf("IP Address: %s\n", m.selectedNISDToStart.IPAddr))
	// s.WriteString(fmt.Sprintf("Client Port: %d\n", m.selectedNISDToStart.ClientPort))
	s.WriteString(fmt.Sprintf("Server Port: %d\n", m.selectedNISDToStart.PeerPort))

	s.WriteString("\nThis will start the NISD process on the target hypervisor.\n\n")
	s.WriteString("Press ENTER to start NISD process, or ESC to cancel")

	return s.String()
}

func (m model) updateShowStartedNISD(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ", "esc":
			m.state = stateNISDManagement
			m.selectedNISDForStart = -1
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewShowStartedNISD() string {
	title := titleStyle.Render("NISD Process Started")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(successStyle.Render(m.message) + "\n\n")
	}

	s.WriteString("NISD Process Details:\n")
	s.WriteString(fmt.Sprintf("  NISD UUID: %s\n", m.selectedNISDToStart.ID))
	s.WriteString(fmt.Sprintf("  Device ID: %s\n", m.selectedNISDToStart.FailureDomain[ctlplfl.DEVICE_IDX]))
	s.WriteString(fmt.Sprintf("  Hypervisor ID: %s\n", m.selectedNISDToStart.FailureDomain[ctlplfl.HV_IDX]))
	// s.WriteString(fmt.Sprintf("  IP Address: %s\n", m.selectedNISDToStart.IPAddr))
	// s.WriteString(fmt.Sprintf("  Client Port: %d\n", m.selectedNISDToStart.ClientPort))
	s.WriteString(fmt.Sprintf("  Server Port: %d\n", m.selectedNISDToStart.PeerPort))
	s.WriteString(fmt.Sprintf("  Status: Running\n"))

	s.WriteString("\nThe NISD process has been started on the target hypervisor.\n")
	s.WriteString("The process should now be accepting connections.\n\n")
	s.WriteString("Press any key to return to NISD management")

	return s.String()
}

// Helper function to start NISD process - placeholder implementation
func (m model) startNISDProcess() error {
	// TODO: Implement actual NISD process starting logic
	// This would typically involve:
	// 1. SSH into the target hypervisor
	// 2. Execute NISD start command with proper configuration
	// 3. Verify the process started successfully
	// 4. Monitor process status

	// For now, this is a placeholder that simulates success
	// In a real implementation, this would:
	// - Connect to the hypervisor via SSH
	// - Start the NISD daemon with the configured parameters
	// - Verify the process is running and listening on the correct ports

	// Simulate process start logic
	fmt.Printf("Starting NISD process:\n")
	fmt.Printf("  UUID: %s\n", m.selectedNISDToStart.ID)
	fmt.Printf("  Device: %s\n", m.selectedNISDToStart.FailureDomain[ctlplfl.DEVICE_IDX])
	fmt.Printf("  Hypervisor: %s (%+v)\n", m.selectedNISDToStart.FailureDomain[ctlplfl.HV_IDX], m.selectedNISDToStart.NetInfo)
	fmt.Printf("  PeerPort: %d\n", m.selectedNISDToStart.PeerPort)

	// Placeholder for actual implementation
	// Real implementation would:
	// 1. SSH to hypervisor
	// 2. Run NISD start command:
	//    nisd_daemon -uuid <UUID> -device <DEVICE> -client-port <PORT> -peer-port <PORT> -config <CONFIG>
	// 3. Check if process started successfully
	// 4. Return error if failed

	return nil
}

func (m model) updateViewAllNISDs(msg tea.Msg) (model, tea.Cmd) {
	// Get all NISDs for navigation
	nisds, err := m.getNISDsList()
	if err != nil || len(nisds) == 0 {
		// If no NISDs or error, keep basic navigation
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.state = stateNISDManagement
				m.selectedNISDIdx = -1
				m.message = ""
				return m, nil
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selectedNISDIdx > 0 {
				m.selectedNISDIdx--
			}
		case "down", "j":
			if len(nisds) > 0 && m.selectedNISDIdx < len(nisds)-1 {
				m.selectedNISDIdx++
			}
		case "esc":
			m.state = stateNISDManagement
			m.selectedNISDIdx = -1
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewAllNISDs() string {
	title := titleStyle.Render("View All NISDs")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	// Query all NISDs from control plane
	nisds, err := m.getNISDsList()
	if err != nil {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Failed to query NISDs from control plane: %v", err)) + "\n\n")
		s.WriteString("Please check:\n")
		s.WriteString("• Control plane is running and accessible\n")
		s.WriteString("• Network connectivity to control plane\n")
		s.WriteString("• Control plane configuration is correct\n\n")
		s.WriteString("Controls: ↑/↓ navigate • esc back to NISD management")
		return s.String()
	}

	if len(nisds) == 0 {
		s.WriteString("No NISDs found in the control plane.\n\n")
		s.WriteString("NISDs will appear here after they are:\n")
		s.WriteString("• Initialized on device partitions\n")
		s.WriteString("• Registered with the control plane\n")
		s.WriteString("• Started and running\n\n")
		s.WriteString("Use 'Initialize NISD' to create new NISD instances.\n\n")
		s.WriteString("Press esc to return to NISD management")
		return s.String()
	}

	// Sort NISDs for consistent ordering
	sort.Slice(nisds, func(i, j int) bool {
		return nisds[i].ID < nisds[j].ID
	})

	// Calculate and display statistics
	stats := m.calculateNISDStats(nisds)

	// Header with statistics
	s.WriteString("═══ NISD Overview ═══\n")
	s.WriteString(fmt.Sprintf("• Total NISDs: %d\n", stats.TotalNISDs))
	s.WriteString(fmt.Sprintf("• Running NISDs: %d\n", stats.RunningNISDs))
	s.WriteString(fmt.Sprintf("• Hypervisors: %d\n", stats.UniqueHVs))
	s.WriteString(fmt.Sprintf("• Devices: %d\n", stats.UniqueDevices))
	if stats.TotalSize > 0 {
		s.WriteString(fmt.Sprintf("• Total Capacity: %s\n", formatBytes(stats.TotalSize)))
	}
	if stats.TotalAvailable > 0 {
		s.WriteString(fmt.Sprintf("• Available Space: %s\n", formatBytes(stats.TotalAvailable)))
	}
	s.WriteString("\n")

	// Calculate dynamic pagination based on terminal height
	// Account for header (8 lines) + pagination info (2 lines) + controls (1 line) = 11 reserved lines
	// Each NISD takes ~1 line normally, ~6-8 lines when selected with details
	// Use conservative estimate to account for potential expanded views
	itemsPerPage := m.calculateItemsPerPageWithHeader(1, 11)

	// Ensure reasonable bounds for usability and screen constraints
	if itemsPerPage < 3 {
		itemsPerPage = 3 // Minimum for usability
	} else if itemsPerPage > 15 {
		itemsPerPage = 15 // Reasonable maximum to prevent overwhelming display
	}
	totalPages := (len(nisds) + itemsPerPage - 1) / itemsPerPage

	// Ensure cursor is within bounds
	if m.selectedNISDIdx < 0 {
		m.selectedNISDIdx = 0
	}
	if m.selectedNISDIdx >= len(nisds) {
		m.selectedNISDIdx = len(nisds) - 1
	}

	// Calculate which page the cursor is on
	currentPage := m.selectedNISDIdx / itemsPerPage
	startIdx := currentPage * itemsPerPage
	endIdx := startIdx + itemsPerPage
	if endIdx > len(nisds) {
		endIdx = len(nisds)
	}

	// Show pagination info when there are multiple pages
	if totalPages > 1 {
		s.WriteString(fmt.Sprintf("Page %d of %d (showing items %d-%d of %d) | Cursor at position %d\n\n",
			currentPage+1, totalPages, startIdx+1, endIdx, len(nisds), m.selectedNISDIdx+1))
	} else {
		// Show cursor position for debugging even on single page
		s.WriteString(fmt.Sprintf("Cursor at position %d of %d\n\n", m.selectedNISDIdx+1, len(nisds)))
	}

	for i := startIdx; i < endIdx; i++ {
		nisd := nisds[i]
		cursor := "  "
		if i == m.selectedNISDIdx {
			cursor = "▶ "
		}

		// Header line with partition name instead of UUID
		partitionName := m.getNISDPartitionName(nisd)
		headerLine := fmt.Sprintf("%s%d. %s", cursor, i+1, partitionName)
		if nisd.TotalSize > 0 {
			headerLine += fmt.Sprintf(" (%s)", formatBytes(nisd.TotalSize))
		}

		if i == m.selectedNISDIdx {
			s.WriteString(selectedItemStyle.Render(headerLine) + "\n")

			// Detailed view for selected NISD
			s.WriteString(fmt.Sprintf("    UUID: %s\n", nisd.ID))
			s.WriteString(fmt.Sprintf("    Device ID: %s\n", nisd.FailureDomain[ctlplfl.DEVICE_IDX]))
			s.WriteString(fmt.Sprintf("    Hypervisor: %s\n", nisd.FailureDomain[ctlplfl.HV_IDX]))

			// Network information
			if len(nisd.NetInfo) > 0 {
				s.WriteString("    Network Info:\n")
				for j, ni := range nisd.NetInfo {
					s.WriteString(fmt.Sprintf("      %d. IP: %s, Port: %d\n", j+1, ni.IPAddr, ni.Port))
				}
			} else {
				s.WriteString("    Network Info: Not available\n")
			}

			s.WriteString(fmt.Sprintf("    Server Port: %d\n", nisd.PeerPort))
			s.WriteString(fmt.Sprintf("    Total Size: %d bytes", nisd.TotalSize))
			if nisd.TotalSize > 0 {
				s.WriteString(fmt.Sprintf(" (%s)", formatBytes(nisd.TotalSize)))
			}
			s.WriteString("\n")
			s.WriteString(fmt.Sprintf("    Available Size: %d bytes", nisd.AvailableSize))
			if nisd.AvailableSize > 0 {
				s.WriteString(fmt.Sprintf(" (%s)", formatBytes(nisd.AvailableSize)))
			}
			s.WriteString("\n")
		} else {
			s.WriteString(headerLine + "\n")
		}
		s.WriteString("\n")
	}

	s.WriteString("Controls: ↑/↓ navigate • esc back to NISD management")

	return s.String()
}

// Vdev Management Functions

func (m model) updateVdevManagement(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.vdevMgmtCursor > 0 {
				m.vdevMgmtCursor--
			}
		case "down", "j":
			maxItems := 5 // Create Vdev, Edit Vdev, View Vdev, Delete Vdev, Search Vdev
			if m.vdevMgmtCursor < maxItems-1 {
				m.vdevMgmtCursor++
			}
		case "enter", " ":
			switch m.vdevMgmtCursor {
			case 0: // Create Vdev
				m.state = stateVdevForm
				m.message = ""
				// Initialize input fields
				m.vdevNameInput.SetValue("")
				m.vdevReplicaInput.SetValue("1")
				m.vdevCountInput.SetValue("1")
				m.vdevSizeInput.SetValue("")
				m.vdevEntityUUIDInput.SetValue("")
				m.vdevFilterTypeInput.SetValue("any")
				m.vdevFormActiveField = inputVdevName
				m.vdevNameInput.Focus()
				m.vdevReplicaInput.Blur()
				m.vdevCountInput.Blur()
				m.vdevSizeInput.Blur()
				m.vdevEntityUUIDInput.Blur()
				m.vdevFilterTypeInput.Blur()
				// Reset creation tracking
				m.createdVdevs = nil
				m.vdevCreationErrors = nil
				m.vdevCreationProgress = 0
				m.vdevCreationTotal = 0
				return m, textinput.Blink
			case 1: // Edit Vdev
				m.state = stateEditVdev
				m.message = ""
				return m, nil
			case 2: // View Vdev
				m.state = stateViewVdev
				m.message = ""
				m.vdevViewCursor = 0
				return m, nil
			case 3: // Delete Vdev
				m.state = stateDeleteVdev
				m.message = ""
				return m, nil
			case 4: // Search Vdev
				m.state = stateSearchVdev
				m.message = ""
				m.vdevSearchInput.SetValue("")
				m.vdevSearchResult = nil
				m.vdevSearchErr = ""
				m.vdevSearchChunkPage = 0
				m.vdevSearchInput.Focus()
				return m, textinput.Blink
			}
		case "esc":
			m.state = stateMenu
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewVdevManagement() string {
	title := titleStyle.Render("Vdev Management")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	options := []string{
		"Create Vdev",
		"Edit Vdev",
		"View Vdev",
		"Delete Vdev",
		"Search Vdev",
	}

	for i, option := range options {
		cursor := " "
		if m.vdevMgmtCursor == i {
			cursor = ">"
		}
		s.WriteString(fmt.Sprintf("%s %s\n", cursor, option))
	}

	s.WriteString("\n" + helpStyle.Render("↑/↓: navigate • enter: select • esc: back"))

	return s.String()
}

// doVdevSearch is run in a goroutine via tea.Cmd.
// Step 1: GetVdevCfgs (GetAll) to resolve name/UUID → vdevID.
// Step 2: GetVdevsWithChunkInfo with the specific ID to fetch chunk mapping.
func (m model) doVdevSearch(query string) VdevSearchMsg {
	cfgs, err := m.cpClient.GetVdevCfgs(&ctlplfl.GetReq{
		GetAll: true,
		// UserToken: m.userToken(),
	})
	if err != nil {
		return VdevSearchMsg{err: fmt.Sprintf("Failed to query vdevs: %v", err)}
	}

	var matchedID string
	for _, cfg := range cfgs {
		if cfg.ID == query || cfg.Name == query {
			matchedID = cfg.ID
			break
		}
	}
	if matchedID == "" {
		return VdevSearchMsg{err: fmt.Sprintf("No vdev found matching %q", query)}
	}

	// Fetch chunk mapping for the specific vdev (no admin role required).
	vdevs, err := m.cpClient.GetVdevsWithChunkInfo(&ctlplfl.GetReq{
		ID:     matchedID,
		GetAll: false,
	})
	if err != nil {
		return VdevSearchMsg{err: fmt.Sprintf("Failed to get chunk info: %v", err)}
	}
	if len(vdevs) == 0 {
		return VdevSearchMsg{err: fmt.Sprintf("No chunk data found for vdev %q", query)}
	}
	found := vdevs[0]
	return VdevSearchMsg{result: &found}
}

func (m model) updateSearchVdev(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case VdevSearchMsg:
		m.message = ""
		m.vdevSearchResult = msg.result
		m.vdevSearchErr = msg.err
		m.vdevSearchChunkPage = 0
		m.state = stateSearchVdevResult
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = stateVdevManagement
			m.message = ""
			return m, nil
		case "enter":
			query := strings.TrimSpace(m.vdevSearchInput.Value())
			if query == "" {
				m.message = "Please enter a vdev name or UUID"
				return m, nil
			}
			if m.cpClient == nil || !m.cpConnected {
				m.vdevSearchResult = nil
				m.vdevSearchErr = "Control plane not connected"
				m.state = stateSearchVdevResult
				return m, nil
			}
			m.message = "Searching..."
			return m, func() tea.Msg { return m.doVdevSearch(query) }
		}
	}
	m.vdevSearchInput, cmd = m.vdevSearchInput.Update(msg)
	return m, cmd
}

func (m model) viewSearchVdev() string {
	title := titleStyle.Render("Search Vdev")
	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if m.message == "Searching..." {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Search by name or UUID: ")
	s.WriteString(m.vdevSearchInput.View())
	s.WriteString("\n\n")
	s.WriteString(helpStyle.Render("enter: search • esc: back"))
	return s.String()
}

func (m model) updateSearchVdevResult(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = stateSearchVdev
			m.message = ""
			m.vdevSearchInput.Focus()
			return m, textinput.Blink
		case "right", "down", "j", "l":
			if m.vdevSearchResult != nil {
				chunkKeys, _ := buildChunkToNisds(m.vdevSearchResult)
				totalPages := (len(chunkKeys) + chunkPageSize - 1) / chunkPageSize
				if m.vdevSearchChunkPage < totalPages-1 {
					m.vdevSearchChunkPage++
				}
			}
		case "left", "up", "k", "h":
			if m.vdevSearchChunkPage > 0 {
				m.vdevSearchChunkPage--
			}
		}
	}
	return m, nil
}

// buildChunkToNisds inverts NisdToChkMap so we can display per-chunk replica sets.
// Returns a sorted list of chunk indices and a map of chunk → []nisdID (one per replica).
func buildChunkToNisds(vdev *ctlplfl.Vdev) ([]int, map[int][]string) {
	chunkToNisds := make(map[int][]string)
	for _, nc := range vdev.NisdToChkMap {
		for _, chunkIdx := range nc.Chunk {
			chunkToNisds[chunkIdx] = append(chunkToNisds[chunkIdx], nc.Nisd.ID)
		}
	}
	// Collect and sort chunk indices for deterministic display
	keys := make([]int, 0, len(chunkToNisds))
	for k := range chunkToNisds {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys, chunkToNisds
}

const chunkPageSize = 10

func (m model) viewSearchVdevResult() string {
	title := titleStyle.Render("Search Result")
	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.vdevSearchErr != "" {
		s.WriteString(errorStyle.Render(m.vdevSearchErr) + "\n\n")
		s.WriteString(helpStyle.Render("esc: search again"))
		return s.String()
	}

	v := m.vdevSearchResult

	// ── Vdev config (always visible) ─────────────────────────────────────────
	s.WriteString(fmt.Sprintf("  Name:     %s\n", v.Cfg.Name))
	s.WriteString(fmt.Sprintf("  ID:       %s\n", v.Cfg.ID))
	s.WriteString(fmt.Sprintf("  Size:     %s (%d bytes)\n", formatSize(v.Cfg.Size), v.Cfg.Size))
	s.WriteString(fmt.Sprintf("  Chunks:   %d\n", v.Cfg.NumChunks))
	s.WriteString(fmt.Sprintf("  Replicas: %d\n", v.Cfg.NumReplica))
	s.WriteString("\n")

	if len(v.NisdToChkMap) == 0 {
		s.WriteString("  No NISD allocation data available.\n\n")
		s.WriteString(helpStyle.Render("esc: search again"))
		return s.String()
	}

	// ── Chunk → replica mapping (paginated) ──────────────────────────────────
	chunkKeys, chunkToNisds := buildChunkToNisds(v)
	totalChunks := len(chunkKeys)
	totalPages := (totalChunks + chunkPageSize - 1) / chunkPageSize
	page := m.vdevSearchChunkPage
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * chunkPageSize
	end := start + chunkPageSize
	if end > totalChunks {
		end = totalChunks
	}

	s.WriteString(fmt.Sprintf("Chunk → Replica Mapping  (page %d of %d, chunks %d–%d of %d):\n",
		page+1, totalPages, start, end-1, totalChunks))
	s.WriteString("\n")

	for i := start; i < end; i++ {
		chunkIdx := chunkKeys[i]
		nisds := chunkToNisds[chunkIdx]
		sort.Strings(nisds)
		s.WriteString(fmt.Sprintf("  Chunk %3d: ", chunkIdx))
		for r, nisdID := range nisds {
			s.WriteString(fmt.Sprintf("R.%d→%s", r, nisdID))
			if r < len(nisds)-1 {
				s.WriteString("  ")
			}
		}
		s.WriteString("\n")
	}
	s.WriteString("\n")

	s.WriteString(helpStyle.Render("←/→: prev/next page • esc: search again"))
	return s.String()
}

func (m model) updateVdevDeviceSelection(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.deviceCursor > 0 {
				m.deviceCursor--
			}
		case "down", "j":
			// Get devices from control plane for navigation
			var deviceCount int
			if m.cpClient != nil && m.cpConnected {
				req := ctlplfl.GetReq{ID: "", GetAll: true}
				devices, err := m.cpClient.GetDevices(req)
				if err == nil {
					deviceCount = len(devices)
				}
			}
			if m.deviceCursor < deviceCount-1 {
				m.deviceCursor++
			}
		case "r", "R":
			// Toggle device selection with 'R' key
			m.selectedDevicesForVdev[m.deviceCursor] = !m.selectedDevicesForVdev[m.deviceCursor]
		case "enter", " ":
			// Continue to form after device selection
			if len(m.selectedDevicesForVdev) > 0 {
				// Validate that we still have control plane connection
				if m.cpClient == nil || !m.cpConnected {
					m.message = "Control plane connection lost"
					return m, nil
				}

				m.state = stateVdevForm
				// Focus the vdev size input
				m.vdevSizeInput.Focus()
				m.message = ""
			} else {
				m.message = "Please select at least one device using 'R' key"
			}
		case "esc":
			m.state = stateVdevManagement
			m.selectedDevicesForVdev = make(map[int]bool)
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewVdevDeviceSelection() string {
	title := titleStyle.Render("Device Selection Deprecated")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	s.WriteString("This device selection view is no longer used.\n")
	s.WriteString("Create Vdev now uses direct size input only.\n\n")
	s.WriteString(helpStyle.Render("esc: back to Vdev management"))
	return s.String()
}

func (m model) updateVdevForm(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			// Cycle forward: name → replica → count → size → entityUUID → filterType → pfsName → name
			m.vdevNameInput.Blur()
			m.vdevReplicaInput.Blur()
			m.vdevCountInput.Blur()
			m.vdevSizeInput.Blur()
			m.vdevEntityUUIDInput.Blur()
			m.vdevFilterTypeInput.Blur()
			m.vdevPFSNameInput.Blur()
			switch m.vdevFormActiveField {
			case inputVdevName:
				m.vdevFormActiveField = inputVdevReplica
				m.vdevReplicaInput.Focus()
			case inputVdevReplica:
				m.vdevFormActiveField = inputVdevCount
				m.vdevCountInput.Focus()
			case inputVdevCount:
				m.vdevFormActiveField = inputVdevSize
				m.vdevSizeInput.Focus()
			case inputVdevSize:
				m.vdevFormActiveField = inputVdevEntityUUID
				m.vdevEntityUUIDInput.Focus()
			case inputVdevEntityUUID:
				m.vdevFormActiveField = inputVdevFilterType
				m.vdevFilterTypeInput.Focus()
			case inputVdevFilterType:
				m.vdevFormActiveField = inputVdevPFSName
				m.vdevPFSNameInput.Focus()
			default:
				m.vdevFormActiveField = inputVdevName
				m.vdevNameInput.Focus()
			}
			return m, textinput.Blink
		case "shift+tab", "up":
			// Cycle backward
			m.vdevNameInput.Blur()
			m.vdevReplicaInput.Blur()
			m.vdevCountInput.Blur()
			m.vdevSizeInput.Blur()
			m.vdevEntityUUIDInput.Blur()
			m.vdevFilterTypeInput.Blur()
			m.vdevPFSNameInput.Blur()
			switch m.vdevFormActiveField {
			case inputVdevReplica:
				m.vdevFormActiveField = inputVdevName
				m.vdevNameInput.Focus()
			case inputVdevCount:
				m.vdevFormActiveField = inputVdevReplica
				m.vdevReplicaInput.Focus()
			case inputVdevSize:
				m.vdevFormActiveField = inputVdevCount
				m.vdevCountInput.Focus()
			case inputVdevEntityUUID:
				m.vdevFormActiveField = inputVdevSize
				m.vdevSizeInput.Focus()
			case inputVdevFilterType:
				m.vdevFormActiveField = inputVdevEntityUUID
				m.vdevEntityUUIDInput.Focus()
			case inputVdevPFSName:
				m.vdevFormActiveField = inputVdevFilterType
				m.vdevFilterTypeInput.Focus()
			default:
				m.vdevFormActiveField = inputVdevPFSName
				m.vdevPFSNameInput.Focus()
			}
			return m, textinput.Blink
		case "enter":
			// Validate inputs and create Vdevs
			nameStr := strings.TrimSpace(m.vdevNameInput.Value())
			replicaStr := m.vdevReplicaInput.Value()
			countStr := m.vdevCountInput.Value()
			sizeStr := m.vdevSizeInput.Value()

			if nameStr == "" {
				m.message = "Please enter a Vdev name"
				return m, nil
			}
			if !isAlphanumericStr(nameStr) {
				m.message = "Vdev name must contain only letters and digits"
				return m, nil
			}

			if replicaStr == "" {
				replicaStr = "1"
			}
			replica, err := strconv.Atoi(replicaStr)
			if err != nil || replica < 1 {
				m.message = "Replication factor must be a positive integer (e.g. 1, 2, 4)"
				return m, nil
			}

			if countStr == "" {
				m.message = "Please enter number of Vdevs"
				return m, nil
			}
			if sizeStr == "" {
				m.message = "Please enter Vdev size"
				return m, nil
			}

			// Parse count
			count, err := strconv.Atoi(countStr)
			if err != nil || count <= 0 {
				m.message = "Please enter a valid number of Vdevs (positive integer)"
				return m, nil
			}
			if count > 100 {
				m.message = "Maximum 100 Vdevs can be created at once"
				return m, nil
			}

			// Parse size
			size, err := parseSize(sizeStr)
			if err != nil {
				m.message = fmt.Sprintf("Invalid size format: %v", err)
				return m, nil
			}

			// Warn if entity UUID is provided with FD_ANY — the ID would be ignored
			// and could cause a server panic (HR.FD[-1] lookup).
			fdType := parseFDType(m.vdevFilterTypeInput.Value())
			entityUUID := strings.TrimSpace(m.vdevEntityUUIDInput.Value())
			if fdType == ctlplfl.FD_ANY && entityUUID != "" {
				m.message = "Entity UUID is only valid when a specific failure domain type (pdu/rack/hv/device) is set"
				return m, nil
			}

			// Determine whether the PFS input is a UUID (PFSID) or a name.
			pfsInput := strings.TrimSpace(m.vdevPFSNameInput.Value())
			if isUUID(pfsInput) {
				m.vdevCreationPFSID = pfsInput
				m.vdevCreationPFSName = ""
			} else {
				m.vdevCreationPFSID = ""
				m.vdevCreationPFSName = pfsInput
			}

			// Initialize creation tracking
			m.vdevCreationTotal = count
			m.vdevCreationProgress = 0
			m.createdVdevs = make([]ctlplfl.VdevCfg, 0, count)
			m.vdevCreationErrors = make([]string, 0)

			// Start creation process
			m.state = stateVdevCreationProgress
			m.message = ""

			// Start creating Vdevs
			return m, m.createVdevsCommand(size, count, replica)
		case "esc":
			m.state = stateVdevManagement
			m.message = ""
			return m, nil
		}
	}

	// Update the active input field
	switch m.vdevFormActiveField {
	case inputVdevName:
		m.vdevNameInput, cmd = m.vdevNameInput.Update(msg)
	case inputVdevReplica:
		m.vdevReplicaInput, cmd = m.vdevReplicaInput.Update(msg)
	case inputVdevCount:
		m.vdevCountInput, cmd = m.vdevCountInput.Update(msg)
	case inputVdevSize:
		m.vdevSizeInput, cmd = m.vdevSizeInput.Update(msg)
	case inputVdevEntityUUID:
		m.vdevEntityUUIDInput, cmd = m.vdevEntityUUIDInput.Update(msg)
	case inputVdevFilterType:
		m.vdevFilterTypeInput, cmd = m.vdevFilterTypeInput.Update(msg)
	case inputVdevPFSName:
		m.vdevPFSNameInput, cmd = m.vdevPFSNameInput.Update(msg)
	}

	return m, cmd
}

func (m model) viewVdevForm() string {
	title := titleStyle.Render("Create Vdev(s)")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Enter configuration for Vdev creation:\n\n")

	// Name input
	s.WriteString("Vdev Name: ")
	s.WriteString(m.vdevNameInput.View())
	s.WriteString("\n\n")

	// Replica input
	s.WriteString("Replication Factor: ")
	s.WriteString(m.vdevReplicaInput.View())
	s.WriteString("\n\n")

	// Count input
	s.WriteString("Number of Vdevs: ")
	s.WriteString(m.vdevCountInput.View())
	s.WriteString("\n\n")

	// Size input
	s.WriteString("Vdev Size: ")
	s.WriteString(m.vdevSizeInput.View())
	s.WriteString("\n\n")

	// Entity UUID input
	s.WriteString("Entity UUID (optional): ")
	s.WriteString(m.vdevEntityUUIDInput.View())
	s.WriteString("\n\n")

	// Filter type input
	s.WriteString("Failure Domain Type (optional): ")
	s.WriteString(m.vdevFilterTypeInput.View())
	s.WriteString("\n\n")

	// PFS input
	s.WriteString("PFS Name or UUID (optional): ")
	s.WriteString(m.vdevPFSNameInput.View())
	s.WriteString("\n\n")

	s.WriteString("Name: letters and digits only, unique across all Vdevs\n")
	s.WriteString("Replication factor: positive integer (default 1); when count > 1 names get a numeric suffix\n")
	s.WriteString("Examples: 10GB, 1TB, 500MB, 2PB\n")
	s.WriteString("Failure domain types: any, pdu, rack, hv, device, partition\n\n")

	// Show summary if both fields have values
	countStr := m.vdevCountInput.Value()
	sizeStr := m.vdevSizeInput.Value()
	if countStr != "" && sizeStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil && count > 0 {
			if count == 1 {
				s.WriteString(fmt.Sprintf("This will create 1 Vdev of size %s\n\n", sizeStr))
			} else {
				s.WriteString(fmt.Sprintf("This will create %d Vdevs of size %s each\n", count, sizeStr))
				if size, err := parseSize(sizeStr); err == nil {
					totalSize := size * int64(count)
					s.WriteString(fmt.Sprintf("Total storage allocation: %s\n\n", formatSize(totalSize)))
				} else {
					s.WriteString("\n")
				}
			}
		}
	}

	s.WriteString(helpStyle.Render("tab/shift+tab: navigate • enter: create • esc: back"))

	return s.String()
}

func (m model) updateEditVdev(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = stateVdevManagement
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewEditVdev() string {
	title := titleStyle.Render("Edit Vdev")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	s.WriteString("Edit functionality coming soon...\n\n")

	s.WriteString(helpStyle.Render("esc: back"))

	return s.String()
}

func (m model) updateViewVdev(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.vdevViewCursor > 0 {
				m.vdevViewCursor--
			}
		case "down", "j":
			// Get Vdevs from control plane for navigation
			if m.cpClient != nil && m.cpConnected {
				vdevs, err := m.cpClient.GetVdevCfgs(&ctlplfl.GetReq{GetAll: true})
				if err == nil && len(vdevs) > 0 {
					// Sort to ensure consistent ordering for navigation bounds
					sort.Slice(vdevs, func(i, j int) bool {
						return vdevs[i].ID < vdevs[j].ID
					})
					// Ensure cursor stays within bounds
					if m.vdevViewCursor < len(vdevs)-1 {
						m.vdevViewCursor++
					}
				}
			}
		case "esc":
			m.state = stateVdevManagement
			m.message = ""
			m.vdevViewCursor = 0
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewViewVdev() string {
	title := titleStyle.Render("View Vdevs")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	// Query Vdevs from control plane
	if m.cpClient != nil && m.cpConnected {
		// Build a PFSID → name map for display (skip reverse-index entries where ID is not a UUID).
		pfsNameMap := make(map[string]string)
		if pfsList, err := m.cpClient.GetPFS(&ctlplfl.GetReq{GetAll: true}); err == nil {
			for _, p := range pfsList {
				if isUUID(p.ID) {
					pfsNameMap[p.ID] = p.Name
				}
			}
		}

		vdevs, err := m.cpClient.GetVdevCfgs(&ctlplfl.GetReq{GetAll: true})
		if err != nil {
			s.WriteString(errorStyle.Render(fmt.Sprintf("Failed to query Vdevs: %v", err)) + "\n\n")
		} else if len(vdevs) == 0 {
			s.WriteString("No Vdevs found.\n\n")
		} else {
			// Sort vdevs by ID to ensure consistent ordering
			sort.Slice(vdevs, func(i, j int) bool {
				return vdevs[i].ID < vdevs[j].ID
			})

			s.WriteString(fmt.Sprintf("Found %d Vdev(s):\n\n", len(vdevs)))

			// Calculate pagination - show 7 items per page to keep header visible
			itemsPerPage := 7 // Show 7 items per page for optimal display
			totalPages := (len(vdevs) + itemsPerPage - 1) / itemsPerPage

			// Ensure cursor is within bounds
			if m.vdevViewCursor < 0 {
				m.vdevViewCursor = 0
			}
			if m.vdevViewCursor >= len(vdevs) {
				m.vdevViewCursor = len(vdevs) - 1
			}

			// Calculate which page the cursor is on
			currentPage := m.vdevViewCursor / itemsPerPage
			startIdx := currentPage * itemsPerPage
			endIdx := startIdx + itemsPerPage
			if endIdx > len(vdevs) {
				endIdx = len(vdevs)
			}

			// Always show pagination info when there are multiple pages or for debugging
			if totalPages > 1 {
				s.WriteString(fmt.Sprintf("Page %d of %d (showing items %d-%d of %d) | Cursor at position %d\n\n",
					currentPage+1, totalPages, startIdx+1, endIdx, len(vdevs), m.vdevViewCursor+1))
			} else {
				// Show cursor position for debugging even on single page
				s.WriteString(fmt.Sprintf("Cursor at position %d of %d\n\n", m.vdevViewCursor+1, len(vdevs)))
			}

			for i := startIdx; i < endIdx; i++ {
				vdev := vdevs[i]
				cursor := "  "
				if m.vdevViewCursor == i {
					cursor = "▶ "
					s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, vdev.Name)))
					s.WriteString("\n")
					s.WriteString(selectedItemStyle.Render(fmt.Sprintf("   ID: %s", vdev.ID)))
					s.WriteString("\n")
					s.WriteString(selectedItemStyle.Render(fmt.Sprintf("   Size: %d bytes", vdev.Size)))
					s.WriteString("\n")
					s.WriteString(selectedItemStyle.Render(fmt.Sprintf("   Chunks: %d", vdev.NumChunks)))
					s.WriteString("\n")
					s.WriteString(selectedItemStyle.Render(fmt.Sprintf("   Replicas: %d", vdev.NumReplica)))
					s.WriteString("\n")
					if vdev.PFSID != "" {
						pfsDisplay := vdev.PFSID
						if name, ok := pfsNameMap[vdev.PFSID]; ok && name != "" {
							pfsDisplay = fmt.Sprintf("%s (%s)", name, vdev.PFSID)
						}
						s.WriteString(selectedItemStyle.Render(fmt.Sprintf("   PFS: %s", pfsDisplay)))
						s.WriteString("\n")
					}
					s.WriteString("\n")
				} else {
					s.WriteString(fmt.Sprintf("%s%d. %s\n", cursor, i+1, vdev.Name))
					s.WriteString(fmt.Sprintf("   ID: %s\n", vdev.ID))
					s.WriteString(fmt.Sprintf("   Size: %d bytes\n", vdev.Size))
					s.WriteString(fmt.Sprintf("   Chunks: %d\n", vdev.NumChunks))
					s.WriteString(fmt.Sprintf("   Replicas: %d\n", vdev.NumReplica))
					if vdev.PFSID != "" {
						pfsDisplay := vdev.PFSID
						if name, ok := pfsNameMap[vdev.PFSID]; ok && name != "" {
							pfsDisplay = fmt.Sprintf("%s (%s)", name, vdev.PFSID)
						}
						s.WriteString(fmt.Sprintf("   PFS: %s\n", pfsDisplay))
					}
					s.WriteString("\n")
				}
			}
		}
	} else {
		s.WriteString(errorStyle.Render("Control plane not connected") + "\n\n")
	}

	s.WriteString(helpStyle.Render("↑/↓: navigate • esc: back"))

	return s.String()
}

func (m model) updateDeleteVdev(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.vdevDeleteCursor > 0 {
				m.vdevDeleteCursor--
			}
		case "down", "j":
			// Get Vdevs from control plane for navigation
			if m.cpClient != nil && m.cpConnected {
				req := &ctlplfl.GetReq{ID: "", GetAll: true}
				vdevs, err := m.cpClient.GetVdevsWithChunkInfo(req)
				if err == nil {
					// Sort to ensure consistent ordering for navigation bounds
					sort.Slice(vdevs, func(i, j int) bool {
						return vdevs[i].Cfg.ID < vdevs[j].Cfg.ID
					})
					if m.vdevDeleteCursor < len(vdevs)-1 {
						m.vdevDeleteCursor++
					}
				}
			}
		case "enter", " ":
			// Delete selected Vdev
			if m.cpClient != nil && m.cpConnected {
				req := &ctlplfl.GetReq{ID: "", GetAll: true}
				vdevs, err := m.cpClient.GetVdevsWithChunkInfo(req)
				if err != nil {
					m.message = fmt.Sprintf("Failed to query Vdevs: %v", err)
					return m, nil
				}
				if len(vdevs) == 0 {
					m.message = "No Vdevs available to delete"
					return m, nil
				}
				// Sort to ensure consistent ordering for selection
				sort.Slice(vdevs, func(i, j int) bool {
					return vdevs[i].Cfg.ID < vdevs[j].Cfg.ID
				})
				if m.vdevDeleteCursor >= 0 && m.vdevDeleteCursor < len(vdevs) {
					selectedVdev := vdevs[m.vdevDeleteCursor]
					// TODO: Implement actual DeleteVdev function when available
					// For now, show a placeholder message
					m.message = fmt.Sprintf("Delete functionality for Vdev %s is not yet implemented in the backend", selectedVdev.Cfg.ID)
					return m, nil
				}
			} else {
				m.message = "Control plane not connected"
				return m, nil
			}
		case "esc":
			m.state = stateVdevManagement
			m.vdevDeleteCursor = 0
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewDeleteVdev() string {
	title := titleStyle.Render("Delete Vdev")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	// Query Vdevs from control plane
	if m.cpClient != nil && m.cpConnected {
		req := &ctlplfl.GetReq{ID: "", GetAll: true}
		vdevs, err := m.cpClient.GetVdevsWithChunkInfo(req)
		if err != nil {
			s.WriteString(errorStyle.Render(fmt.Sprintf("Failed to query Vdevs: %v", err)) + "\n\n")
		} else if len(vdevs) == 0 {
			s.WriteString("No Vdevs found to delete.\n\n")
		} else {
			// Sort vdevs by ID to ensure consistent ordering
			sort.Slice(vdevs, func(i, j int) bool {
				return vdevs[i].Cfg.ID < vdevs[j].Cfg.ID
			})

			s.WriteString("Select a Vdev to delete:\n\n")
			for i, vdev := range vdevs {
				cursor := "  "
				if m.vdevDeleteCursor == i {
					cursor = "▶ "
					s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%d. ID: %s (Size: %d bytes)", cursor, i+1, vdev.Cfg.ID, vdev.Cfg.Size)))
				} else {
					s.WriteString(fmt.Sprintf("%s%d. ID: %s (Size: %d bytes)", cursor, i+1, vdev.Cfg.ID, vdev.Cfg.Size))
				}
				s.WriteString("\n")
			}
			s.WriteString("\n")
		}
	} else {
		s.WriteString(errorStyle.Render("Control plane not connected") + "\n\n")
	}

	s.WriteString(helpStyle.Render("↑/↓: navigate • enter: delete • esc: back"))

	return s.String()
}

func (m model) updateShowAddedVdev(msg tea.Msg) (model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		m.state = stateVdevManagement
		m.selectedDevicesForVdev = make(map[int]bool)
		m.vdevSizeInput.SetValue("")
		m.vdevEntityUUIDInput.SetValue("")
		m.vdevFilterTypeInput.SetValue("any")
		m.message = ""
		return m, nil
	}
	return m, nil
}

func (m model) viewShowAddedVdev() string {
	title := titleStyle.Render("Vdev Created Successfully")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	s.WriteString("Vdev Details:\n")
	s.WriteString(fmt.Sprintf("Name: %s\n", m.currentVdev.Cfg.Name))
	s.WriteString(fmt.Sprintf("ID: %s\n", m.currentVdev.Cfg.ID))
	s.WriteString(fmt.Sprintf("Size: %d bytes\n", m.currentVdev.Cfg.Size))
	s.WriteString(fmt.Sprintf("Chunks: %d\n", m.currentVdev.Cfg.NumChunks))
	s.WriteString(fmt.Sprintf("Replicas: %d\n", m.currentVdev.Cfg.NumReplica))

	s.WriteString("\n\nPress any key to return to Vdev management")

	return s.String()
}

// updateVdevCreationProgress handles Vdev creation progress
func (m model) updateVdevCreationProgress(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			// Allow canceling during creation (though Vdevs already created won't be rolled back)
			m.state = stateVdevManagement
			m.message = "Vdev creation canceled"
			return m, nil
		}
	case VdevCreationMsg:
		// Handle creation result
		m.vdevCreationProgress++

		if msg.Success {
			m.createdVdevs = append(m.createdVdevs, *msg.Vdev)
		} else {
			m.vdevCreationErrors = append(m.vdevCreationErrors, fmt.Sprintf("Vdev %d: %v", msg.Index+1, msg.Error))
		}

		// Check if we need to create more Vdevs
		if m.vdevCreationProgress < m.vdevCreationTotal {
			// Get the size and create next Vdev
			sizeStr := m.vdevSizeInput.Value()
			replicaStr := m.vdevReplicaInput.Value()
			replica := 1
			if r, err := strconv.Atoi(replicaStr); err == nil && r >= 1 {
				replica = r
			}
			if size, err := parseSize(sizeStr); err == nil {
				progress := m.vdevCreationProgress
				return m, func() tea.Msg {
					return m.createSingleVdev(size, replica, progress)
				}
			} else {
				// Error parsing size, stop creation
				m.vdevCreationErrors = append(m.vdevCreationErrors, fmt.Sprintf("Failed to parse size: %v", err))
				m.state = stateVdevCreationSummary
				return m, nil
			}
		} else {
			// All done, show summary
			m.state = stateVdevCreationSummary
			return m, nil
		}
	}
	return m, nil
}

// viewVdevCreationProgress shows progress of Vdev creation
func (m model) viewVdevCreationProgress() string {
	title := titleStyle.Render("Creating Vdevs")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	countStr := m.vdevCountInput.Value()
	sizeStr := m.vdevSizeInput.Value()
	s.WriteString(fmt.Sprintf("Creating %s Vdevs of size %s each...\n\n", countStr, sizeStr))

	// Show progress for each Vdev
	for i := 0; i < m.vdevCreationTotal; i++ {
		if i < len(m.createdVdevs) {
			s.WriteString(fmt.Sprintf("✓ Vdev %d/%d: ID %s created successfully\n", i+1, m.vdevCreationTotal, m.createdVdevs[i].ID))
		} else if i < m.vdevCreationProgress {
			// This means there was an error for this Vdev
			if i < len(m.vdevCreationErrors) {
				s.WriteString(fmt.Sprintf("✗ Vdev %d/%d: Failed - %s\n", i+1, m.vdevCreationTotal, m.vdevCreationErrors[i]))
			} else {
				s.WriteString(fmt.Sprintf("✗ Vdev %d/%d: Failed - unknown error\n", i+1, m.vdevCreationTotal))
			}
		} else if i == m.vdevCreationProgress {
			s.WriteString(fmt.Sprintf("⟳ Vdev %d/%d: Creating...\n", i+1, m.vdevCreationTotal))
		} else {
			s.WriteString(fmt.Sprintf("  Vdev %d/%d: Waiting...\n", i+1, m.vdevCreationTotal))
		}
	}

	// Progress bar
	s.WriteString("\n")
	progress := float64(m.vdevCreationProgress) / float64(m.vdevCreationTotal)
	barWidth := 40
	filledWidth := int(progress * float64(barWidth))

	s.WriteString("[")
	for i := 0; i < barWidth; i++ {
		if i < filledWidth {
			s.WriteString("█")
		} else {
			s.WriteString("░")
		}
	}
	s.WriteString(fmt.Sprintf("] %.0f%%\n\n", progress*100))

	s.WriteString(helpStyle.Render("esc: cancel (already created Vdevs will remain)"))

	return s.String()
}

// updateVdevCreationSummary handles the summary view
func (m model) updateVdevCreationSummary(msg tea.Msg) (model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		m.state = stateVdevManagement
		m.message = ""
		return m, nil
	}
	return m, nil
}

// viewVdevCreationSummary shows the final summary of Vdev creation
func (m model) viewVdevCreationSummary() string {
	title := titleStyle.Render("Vdev Creation Summary")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	successCount := len(m.createdVdevs)
	errorCount := len(m.vdevCreationErrors)

	if errorCount == 0 {
		s.WriteString(successStyle.Render(fmt.Sprintf("Successfully created %d Vdevs:", successCount)))
	} else {
		s.WriteString(fmt.Sprintf("Creation completed with %d successes and %d failures:\n", successCount, errorCount))
	}
	s.WriteString("\n\n")

	// Show successful Vdevs
	if successCount > 0 {
		s.WriteString("✓ Successfully created:\n")
		for i, vdev := range m.createdVdevs {
			s.WriteString(fmt.Sprintf("%d. Name:   %s\n", i+1, vdev.Name))
			s.WriteString(fmt.Sprintf("   Vdev ID: %s\n", vdev.ID))
			s.WriteString(fmt.Sprintf("   Size:    %s (%d bytes)\n", formatSize(vdev.Size), vdev.Size))
			s.WriteString(fmt.Sprintf("   Chunks:  %d\n", vdev.NumChunks))
			s.WriteString(fmt.Sprintf("   Replicas: %d\n", vdev.NumReplica))
			s.WriteString(fmt.Sprintf("   Status:  Active\n"))
			switch {
			case vdev.PFSName != "" && vdev.PFSID != "":
				s.WriteString(fmt.Sprintf("   PFS:     %s (%s)\n", vdev.PFSName, vdev.PFSID))
			case vdev.PFSName != "":
				s.WriteString(fmt.Sprintf("   PFS:     %s\n", vdev.PFSName))
			case vdev.PFSID != "":
				s.WriteString(fmt.Sprintf("   PFS:     %s\n", vdev.PFSID))
			}
			s.WriteString("\n")
		}
	}

	// Show errors if any
	if errorCount > 0 {
		s.WriteString("✗ Failed to create:\n")
		for _, err := range m.vdevCreationErrors {
			s.WriteString(fmt.Sprintf("   %s\n", err))
		}
		s.WriteString("\n")
	}

	if successCount > 0 {
		sizeStr := m.vdevSizeInput.Value()
		if size, err := parseSize(sizeStr); err == nil {
			totalAllocated := size * int64(successCount)
			s.WriteString(fmt.Sprintf("Total allocated: %s\n\n", formatSize(totalAllocated)))
		}
	}

	s.WriteString(helpStyle.Render("enter: back to Vdev management"))

	return s.String()
}

// Helper function to parse size strings like "10GB", "1TB", "1PB"
// VdevCreationMsg represents a message for Vdev creation progress
type VdevCreationMsg struct {
	Index   int
	Success bool
	Vdev    *ctlplfl.VdevCfg
	Error   error
}

type VdevSearchMsg struct {
	result *ctlplfl.Vdev
	err    string
}

// createVdevsCommand returns a command that creates multiple Vdevs
func (m model) createVdevsCommand(size int64, count int, replica int) tea.Cmd {
	return func() tea.Msg {
		// Create the first Vdev
		return m.createSingleVdev(size, replica, 0)
	}
}

// isUUID returns true if s looks like a UUID (8-4-4-4-12 hex with hyphens).
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// isAlphanumericStr returns true if s contains only ASCII letters and digits.
func isAlphanumericStr(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// parseFDType converts a filter type string to an FD constant.
func parseFDType(s string) ctlplfl.FD {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pdu":
		return ctlplfl.FD_PDU
	case "rack":
		return ctlplfl.FD_RACK
	case "hv", "hypervisor":
		return ctlplfl.FD_HV
	case "device":
		return ctlplfl.FD_DEVICE
	case "partition":
		return ctlplfl.FD_PARTITION
	default:
		return ctlplfl.FD_ANY
	}
}

// createSingleVdev creates a single Vdev and returns a VdevCreationMsg
func (m model) createSingleVdev(size int64, replica int, index int) VdevCreationMsg {
	filter := ctlplfl.Filter{
		Type: parseFDType(m.vdevFilterTypeInput.Value()),
	}
	// Entity ID is only meaningful when a specific FD type is requested.
	// Sending a non-empty ID with FD_ANY would reach GetEntityByID with an
	// FD_ANY type, causing HR.FD[-1] (index out of bounds) on the server.
	if filter.Type != ctlplfl.FD_ANY {
		filter.ID = strings.TrimSpace(m.vdevEntityUUIDInput.Value())
	}
	// When creating multiple Vdevs, append a 1-based numeric suffix to make
	// each name unique (e.g. "myvol1", "myvol2"). Single Vdev uses the name as-is.
	name := strings.TrimSpace(m.vdevNameInput.Value())
	if m.vdevCreationTotal > 1 {
		name = name + strconv.Itoa(index+1)
	}
	vdev := &ctlplfl.VdevReq{
		Vdev: &ctlplfl.VdevCfg{
			Name:       name,
			Size:       size,
			NumReplica: uint8(replica),
			PFSID:      m.vdevCreationPFSID,
			PFSName:    m.vdevCreationPFSName,
		},
		Filter: filter,
	}

	if m.cpClient != nil && m.cpConnected {
		log.Info("Creating Vdev with size: ", vdev.Vdev.Size)
		resp, err := m.cpClient.CreateVdev(vdev)
		if err != nil {
			log.Error("CreateVdev failed: ", err)
			return VdevCreationMsg{
				Index:   index,
				Success: false,
				Error:   err,
			}
		}
		if resp != nil && !resp.Success {
			log.Error("CreateVdev rejected: ", resp.Error)
			return VdevCreationMsg{
				Index:   index,
				Success: false,
				Error:   fmt.Errorf("%s", resp.Error),
			}
		}
		log.Info("Vdev created successfully: ", resp.ID)
		vdev.Vdev.ID = resp.ID
		vdev.Vdev.NumChunks = uint32(ctlplfl.Count8GBChunks(size))
		return VdevCreationMsg{
			Index:   index,
			Success: true,
			Vdev:    vdev.Vdev,
		}
	}

	return VdevCreationMsg{
		Index:   index,
		Success: false,
		Error:   fmt.Errorf("control plane not connected"),
	}
}

func parseSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))

	if len(sizeStr) < 2 {
		return 0, fmt.Errorf("invalid size format")
	}

	// Extract number and unit
	var numStr string
	var unit string

	for i, r := range sizeStr {
		if r >= '0' && r <= '9' || r == '.' {
			numStr += string(r)
		} else {
			unit = sizeStr[i:]
			break
		}
	}

	if numStr == "" {
		return 0, fmt.Errorf("no number found in size")
	}

	// Parse the number (support decimal)
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %v", err)
	}

	// Convert based on unit
	var multiplier int64
	switch unit {
	case "B", "BYTES":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "PB":
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unsupported unit: %s", unit)
	}

	return int64(num * float64(multiplier)), nil
}

// formatSize converts bytes to human readable format
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp+1])
}

// validateDeviceInfo ensures device has required fields populated
func validateDeviceInfo(device *ctlplfl.Device) {
	// Ensure Name is set if it's empty but ID is present
	if device.Name == "" && device.ID != "" {
		device.Name = device.ID
	}

	// Ensure DevicePath is set if it's empty
	if device.DevicePath == "" && device.ID != "" {
		device.DevicePath = "/dev/" + device.ID
	}

	// Ensure SerialNumber has a fallback if empty
	if device.SerialNumber == "" {
		device.SerialNumber = "Unknown"
	}

	// Ensure FailureDomain has a fallback if empty
	if device.FailureDomain == "" {
		device.FailureDomain = "default"
	}
}

/*
// updateInitializeDeviceForm handles the Initialize Device form
func (m model) updateInitializeDeviceForm(msg tea.Msg) (model, tea.Cmd) {
	// Check if control plane is connected
	if m.cpClient == nil || !m.cpConnected {
		m.message = "Control plane not connected. Please connect to control plane first."
		m.state = stateDeviceManagement
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = stateDeviceManagement
			m.message = ""
			return m, nil
		case "enter", " ":
			// Call PutDevice for all devices
			deviceCount := 0
			errorCount := 0

			// Initialize configured devices
			for _, hv := range m.cpHypervisors {
				for _, device := range hv.Dev {
					deviceInfo := ctlplfl.Device{
						ID:            device.ID,
						Name:          device.Name,
						DevicePath:    device.DevicePath,
						SerialNumber:  device.SerialNumber,
						State:         ctlplfl.INITIALIZED, // Default status
						Size:          device.Size,
						HypervisorID:  hv.ID,
						FailureDomain: device.FailureDomain,
					}

					// Validate and fix device info
					validateDeviceInfo(&deviceInfo)

					log.Info("Sending device to CP - ID: ", deviceInfo.ID,
						", Name: ", deviceInfo.Name,
						", Path: ", deviceInfo.DevicePath,
						", Size: ", deviceInfo.Size,
						", Serial: ", deviceInfo.SerialNumber,
						", HV: ", deviceInfo.HypervisorID)

					devResp, err := m.cpClient.PutDevice(&deviceInfo)
					if err != nil {
						log.Warn("Failed to initialize device in control plane: ", err)
						errorCount++
					} else if devResp != nil && !devResp.Success {
						log.Warn("Control plane rejected device init: ", devResp.Error)
						errorCount++
					} else {
						deviceCount++
					}
				}
			}

			// Initialize discovered devices if any
			if len(m.discoveredDevs) > 0 && m.currentHv.ID != "" {
				for _, device := range m.discoveredDevs {
					deviceInfo := ctlplfl.Device{
						ID:            device.ID,
						Name:          device.Name,
						DevicePath:    device.DevicePath,
						SerialNumber:  device.SerialNumber,
						State:         ctlplfl.INITIALIZED, // Default status
						Size:          device.Size,
						HypervisorID:  m.currentHv.ID,
						FailureDomain: device.FailureDomain,
					}

					// Validate and fix device info
					validateDeviceInfo(&deviceInfo)

					log.Info("Sending device to CP - ID: ", deviceInfo.ID,
						", Name: ", deviceInfo.Name,
						", Path: ", deviceInfo.DevicePath,
						", Size: ", deviceInfo.Size,
						", Serial: ", deviceInfo.SerialNumber,
						", HV: ", deviceInfo.HypervisorID)

					devResp, err := m.cpClient.PutDevice(&deviceInfo)
					if err != nil {
						log.Warn("Failed to initialize discovered device in control plane: ", err)
						errorCount++
					} else if devResp != nil && !devResp.Success {
						log.Warn("Control plane rejected discovered device init: ", devResp.Error)
						errorCount++
					} else {
						deviceCount++
					}
				}
			}

			if errorCount > 0 {
				m.message = fmt.Sprintf("Initialized %d devices with %d errors", deviceCount, errorCount)
			} else {
				m.message = fmt.Sprintf("Successfully initialized %d devices in control plane", deviceCount)
			}
			m.refreshCPData()

			m.state = stateDeviceManagement
			return m, nil
		}
	}
	return m, nil
}
*/

/*
// viewInitializeDeviceForm displays the Initialize Device form
func (m model) viewInitializeDeviceForm() string {
	title := titleStyle.Render("Initialize Device")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	// Check if control plane is connected
	if m.cpClient == nil || !m.cpConnected {
		s.WriteString(errorStyle.Render("Control plane not connected. Please connect to control plane first.") + "\n\n")
		s.WriteString(helpStyle.Render("esc: back to Device Management"))
		return s.String()
	}

	// Count devices from both local config and discovered devices
	deviceCount := 0
	for _, hv := range m.cpHypervisors {
		deviceCount += len(hv.Dev)
	}

	// Also check if there are any discovered devices not yet in config
	if len(m.discoveredDevs) > 0 {
		s.WriteString("Note: Found discovered devices that can be initialized.\n\n")
	}

	if deviceCount == 0 && len(m.discoveredDevs) == 0 {
		s.WriteString("This function initializes devices to the Control Plane.\n\n")
		s.WriteString("Options:\n")
		s.WriteString("• Use Hypervisor Management → Device Discovery to find devices\n")
		s.WriteString("• Manually configure devices in hypervisor settings\n\n")
		s.WriteString("Current status:\n")
		s.WriteString(fmt.Sprintf("• Configured devices: %d\n", deviceCount))
		s.WriteString(fmt.Sprintf("• Discovered devices: %d\n", len(m.discoveredDevs)))
		s.WriteString("\n" + helpStyle.Render("esc: back to Device Management"))
		return s.String()
	}

	totalDevices := deviceCount + len(m.discoveredDevs)
	s.WriteString(fmt.Sprintf("This will initialize %d devices to the Control Plane using PutDevice().\n\n", totalDevices))

	// Show device summary
	s.WriteString("Devices to be initialized:\n")

	// Show configured devices
	for _, hv := range m.cpHypervisors {
		if len(hv.Dev) > 0 {
			s.WriteString(fmt.Sprintf("\nHypervisor: %s (%s) - Configured Devices\n", hv.Name, hv.ID))
			for _, device := range hv.Dev {
				status := "✓"
				if device.State != ctlplfl.INITIALIZED {
					status = "○"
				}
				s.WriteString(fmt.Sprintf("  %s %s - %s (Size: %d GB)\n",
					status, device.Name, device.ID, device.Size/(1024*1024*1024)))
			}
		}
	}

	// Show discovered devices
	if len(m.discoveredDevs) > 0 && m.currentHv.ID != "" {
		s.WriteString(fmt.Sprintf("\nHypervisor: %s (%s) - Discovered Devices\n", m.currentHv.Name, m.currentHv.ID))
		for _, device := range m.discoveredDevs {
			s.WriteString(fmt.Sprintf("  ○ %s - %s (Size: %d GB) [Discovered]\n",
				device.Name, device.ID, device.Size/(1024*1024*1024)))
		}
	}

	s.WriteString("\n" + helpStyle.Render("enter: Initialize all devices, esc: back to Device Management"))

	return s.String()
}
*/

// PFS Management Methods

func (m model) updatePFSManagement(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.pfsMgmtCursor > 0 {
				m.pfsMgmtCursor--
			}
		case "down", "j":
			if m.pfsMgmtCursor < 2 {
				m.pfsMgmtCursor++
			}
		case "enter", " ":
			switch m.pfsMgmtCursor {
			case 0: // Add PFS
				m.state = statePFSForm
				m.pfsNameInput.SetValue("")
				m.message = ""
				m.pfsNameInput.Focus()
				return m, textinput.Blink
			case 1: // View PFSs
				m.state = stateViewPFS
				m.pfsViewCursor = 0
				m.message = ""
				return m, nil
			case 2: // Back
				m.state = stateMenu
				m.message = ""
				return m, nil
			}
		case "esc", "q":
			m.state = stateMenu
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) updatePFSForm(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.pfsNameInput.Value())
			if name == "" {
				m.message = "PFS Name is required"
				return m, nil
			}
			pfs := &ctlplfl.PFS{Name: name}
			resp, err := m.cpClient.PutPFS(pfs)
			if err != nil {
				m.message = fmt.Sprintf("Failed to create PFS: %v", err)
				return m, nil
			}
			if resp != nil && !resp.Success {
				m.message = fmt.Sprintf("Failed to create PFS: %s", resp.Error)
				return m, nil
			}
			m.currentPFS = ctlplfl.PFS{ID: resp.ID, Name: name}
			m.state = stateShowAddedPFS
			m.message = ""
			return m, nil
		case "esc":
			m.state = statePFSManagement
			m.message = ""
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.pfsNameInput, cmd = m.pfsNameInput.Update(msg)
	return m, cmd
}

func (m model) updateShowAddedPFS(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ", "esc":
			m.state = statePFSManagement
			return m, nil
		}
	}
	return m, nil
}

// PFS Management Views

func (m model) viewPFSManagement() string {
	title := titleStyle.Render("PFS Management")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		if strings.Contains(m.message, "Failed") || strings.Contains(m.message, "Error") {
			s.WriteString(errorStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(successStyle.Render(m.message) + "\n\n")
		}
	}

	s.WriteString("Select an action:\n\n")

	items := []string{
		"Add PFS  - Create a new Parallel File System",
		"View PFS - List all PFSs and their Vdevs",
		"Back     - Return to main menu",
	}

	for i, item := range items {
		cursor := "  "
		if i == m.pfsMgmtCursor {
			cursor = "▶ "
			s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, item)))
		} else {
			s.WriteString(fmt.Sprintf("%s%d. %s", cursor, i+1, item))
		}
		s.WriteString("\n")
	}

	s.WriteString("\nControls: ↑/↓ navigate, enter select, esc back to main menu")
	return s.String()
}

func (m model) viewPFSForm() string {
	title := titleStyle.Render("Create New PFS")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.message != "" {
		s.WriteString(errorStyle.Render(m.message) + "\n\n")
	}

	s.WriteString("Name:\n")
	s.WriteString(focusedStyle.Render(m.pfsNameInput.View()) + "\n\n")
	s.WriteString("Controls: enter submit, esc back")
	return s.String()
}

func (m model) viewShowAddedPFS() string {
	title := titleStyle.Render("PFS Created Successfully")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	s.WriteString(lipgloss.NewStyle().Bold(true).Render("PFS Details:") + "\n\n")
	s.WriteString(fmt.Sprintf("Name: %s\n", m.currentPFS.Name))
	if m.currentPFS.ID != "" {
		s.WriteString(fmt.Sprintf("UUID: %s\n", m.currentPFS.ID))
	}

	s.WriteString("\nPress enter or esc to continue")
	return s.String()
}

func (m model) updateViewPFS(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.pfsViewCursor > 0 {
				m.pfsViewCursor--
			}
		case "down", "j":
			if m.cpClient != nil && m.cpConnected {
				pfsList, err := m.cpClient.GetPFS(&ctlplfl.GetReq{GetAll: true})
				if err == nil && m.pfsViewCursor < len(pfsList)-1 {
					m.pfsViewCursor++
				}
			}
		case "esc", "q":
			m.state = statePFSManagement
			m.message = ""
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewViewPFS() string {
	title := titleStyle.Render("View PFSs")

	var s strings.Builder
	s.WriteString(title + "\n\n")

	if m.cpClient == nil || !m.cpConnected {
		s.WriteString(errorStyle.Render("Control plane not connected") + "\n\n")
		s.WriteString(helpStyle.Render("esc: back"))
		return s.String()
	}

	pfsList, err := m.cpClient.GetPFS(&ctlplfl.GetReq{GetAll: true})
	if err != nil {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Failed to query PFSs: %v", err)) + "\n\n")
		s.WriteString(helpStyle.Render("esc: back"))
		return s.String()
	}

	// Filter out reverse-index entries (ID is the PFS name, not a UUID).
	realPFSs := pfsList[:0]
	for _, p := range pfsList {
		if isUUID(p.ID) {
			realPFSs = append(realPFSs, p)
		}
	}
	pfsList = realPFSs

	if len(pfsList) == 0 {
		s.WriteString("No PFSs found.\n\n")
		s.WriteString(helpStyle.Render("esc: back"))
		return s.String()
	}

	// Sort PFSs by name for consistent ordering.
	sort.Slice(pfsList, func(i, j int) bool {
		return pfsList[i].Name < pfsList[j].Name
	})

	// Build vdev ID → name map for resolving vdev names under each PFS.
	vdevNameMap := make(map[string]string)
	if vdevs, err := m.cpClient.GetVdevCfgs(&ctlplfl.GetReq{GetAll: true}); err == nil {
		for _, v := range vdevs {
			vdevNameMap[v.ID] = v.Name
		}
	}

	s.WriteString(fmt.Sprintf("Found %d PFS(s):\n\n", len(pfsList)))

	for i, pfs := range pfsList {
		cursor := "  "
		if i == m.pfsViewCursor {
			cursor = "▶ "
			s.WriteString(selectedItemStyle.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, pfs.Name)))
			s.WriteString("\n")
			s.WriteString(selectedItemStyle.Render(fmt.Sprintf("   ID: %s", pfs.ID)))
			s.WriteString("\n")
			s.WriteString(selectedItemStyle.Render(fmt.Sprintf("   Vdevs: %d", len(pfs.VdevIDs))))
			s.WriteString("\n")
			for _, vdevID := range pfs.VdevIDs {
				vdevLabel := vdevID
				if name, ok := vdevNameMap[vdevID]; ok && name != "" {
					vdevLabel = fmt.Sprintf("%s (%s)", name, vdevID)
				}
				s.WriteString(selectedItemStyle.Render(fmt.Sprintf("     - %s", vdevLabel)))
				s.WriteString("\n")
			}
			s.WriteString("\n")
		} else {
			s.WriteString(fmt.Sprintf("%s%d. %s\n", cursor, i+1, pfs.Name))
			s.WriteString(fmt.Sprintf("   ID: %s\n", pfs.ID))
			s.WriteString(fmt.Sprintf("   Vdevs: %d\n", len(pfs.VdevIDs)))
			for _, vdevID := range pfs.VdevIDs {
				vdevLabel := vdevID
				if name, ok := vdevNameMap[vdevID]; ok && name != "" {
					vdevLabel = fmt.Sprintf("%s (%s)", name, vdevID)
				}
				s.WriteString(fmt.Sprintf("     - %s\n", vdevLabel))
			}
			s.WriteString("\n")
		}
	}

	s.WriteString(helpStyle.Render("↑/↓: navigate • esc: back"))
	return s.String()
}

func main() {
	// Define command line flags
	var (
		cpEnabled    = flag.Bool("cp", false, "Enable control plane integration")
		cpRaftUUID   = flag.String("raft-uuid", "", "Control plane raft UUID")
		cpGossipPath = flag.String("gossip-path", "", "Control plane gossip configuration path")
		logFile      = flag.String("log-file", "", "Path to log file (if not specified, logs to stderr)")
		showHelp     = flag.Bool("help", false, "Show help information")
	)

	flag.Parse()

	// Configure logging
	if *logFile != "" {
		file, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("Failed to open log file %s: %v", *logFile, err)
		}
		log.SetOutput(file)
		log.Info("Logging configured to file: ", *logFile)
	}

	if *showHelp {
		fmt.Println("Niova Backend Configuration Tool")
		fmt.Println("Usage: niova-ctl [OPTIONS]")
		fmt.Println("\nThis tool helps configure Niova backend by:")
		fmt.Println("1. Managing Power Distribution Units (PDUs) and server racks")
		fmt.Println("2. Collecting hypervisor details (IP, name, port range)")
		fmt.Println("3. Discovering storage devices via SSH")
		fmt.Println("4. Allowing device selection and partitioning for NISD")
		fmt.Println("5. Discovering and partitioning devices for NISD")
		fmt.Println("\nControl Plane Options:")
		fmt.Println("  -cp                  Enable control plane integration")
		fmt.Println("  -raft-uuid string    Control plane raft UUID")
		fmt.Println("  -gossip-path string  Control plane gossip configuration path")
		fmt.Println("  -log-file string     Path to log file (if not specified, logs to stderr)")
		fmt.Println("  -help               Show this help information")
		return
	}

	defer func() {
		if userClientTeardownFn != nil {
			userClientTeardownFn()
		}
	}()

	p := tea.NewProgram(
		initialModel(*cpEnabled, *cpRaftUUID, *cpGossipPath, *logFile),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}

package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/pages"
)

// resourceSavedMsg is returned by the async command that persists a
// newly-created or edited resource. FailPrefix/SuccessMsg carry each
// resource's status wording since the verb differs by resource.
type resourceSavedMsg struct {
	FailPrefix string
	SuccessMsg string
	Tab        int
	Err        error
}

// pfsSaveRequest is the ConfirmDialog payload for "save_pfs". Carries
// IsEdit explicitly rather than inferring it from PFS.ID, since only some
// backends implement domain.PFSEditor.
type pfsSaveRequest struct {
	PFS    ctlplfl.PFS
	IsEdit bool
}

// requireCPClient reports whether a.CPClient is connected, setting the
// standard "not connected" status if not — the shared guard every
// handleSubmitXForm below opens with, since none can proceed without it.
func (a *App) requireCPClient() bool {
	if a.CPClient != nil {
		return true
	}
	a.StatusMsg = "Error: Control plane client is not connected"
	a.StatusType = components.StatusError
	return false
}

// idOrNew trims id and mints a fresh UUID if that leaves it empty — the
// "reuse on edit, mint on create" idiom every resource's ID field follows;
// PutX upserts by ID, so reusing it on edit is what makes it an update
// rather than an unwanted new record.
func idOrNew(id string) string {
	if id = strings.TrimSpace(id); id != "" {
		return id
	}
	return uuid.NewString()
}

// saveResourceCmd wraps a single-call resource save (SetToken, call put,
// wrap the result in resourceSavedMsg) — the shape shared by every
// executeSaveX that isn't a batch or backend-capability-gated save (those
// stay inline, where the extra branching is easier to follow written out).
func (a *App) saveResourceCmd(failPrefix, successMsg string, tab int, put func() error) tea.Cmd {
	return func() tea.Msg {
		a.CPClient.SetToken(a.UserToken())
		return resourceSavedMsg{FailPrefix: failPrefix, SuccessMsg: successMsg, Tab: tab, Err: put()}
	}
}

func (a *App) handleSubmitPDUForm(msg pages.SubmitPDUFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	pdu := ctlplfl.PDU{
		ID:            idOrNew(msg.PDUID),
		Name:          msg.Name,
		Specification: msg.Specification,
		Location:      msg.Location,
		PowerCapacity: msg.PowerCapacity,
	}
	title := "Confirm PDU Creation"
	prompt := fmt.Sprintf("Save PDU \"%s\"?", pdu.Name)
	if msg.IsEdit {
		title = "Confirm PDU Edit"
		prompt = fmt.Sprintf("Update PDU \"%s\"?", pdu.Name)
	}
	details := []string{
		fmt.Sprintf("PDU ID: %s", pdu.ID),
		fmt.Sprintf("Name: %s", pdu.Name),
		fmt.Sprintf("Location: %s", pdu.Location),
		fmt.Sprintf("Power Capacity: %s", pdu.PowerCapacity),
	}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeWarning, "save_pdu", pdu)
	return a, nil
}

// executeSavePDU persists a PDU confirmed via handleSubmitPDUForm's dialog.
func (a *App) executeSavePDU(pdu ctlplfl.PDU) tea.Cmd {
	return a.saveResourceCmd("Failed to save PDU: ", "PDU saved successfully!", 1, func() error { return a.CPClient.PutPDU(&pdu) })
}

func (a *App) handleSubmitRackForm(msg pages.SubmitRackFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	pduID := strings.TrimSpace(msg.PDUID)
	if pduID != "" {
		for _, p := range a.PDUs {
			if strings.EqualFold(p.ID, pduID) || strings.EqualFold(p.Name, pduID) {
				pduID = p.ID
				break
			}
		}
	}
	rack := ctlplfl.Rack{
		ID:            idOrNew(msg.RackID),
		PDUID:         pduID,
		Name:          msg.Name,
		Specification: msg.Specification,
		Location:      msg.Location,
	}
	title := "Confirm Rack Creation"
	prompt := fmt.Sprintf("Save Rack \"%s\"?", rack.Name)
	if msg.IsEdit {
		title = "Confirm Rack Edit"
		prompt = fmt.Sprintf("Update Rack \"%s\"?", rack.Name)
	}
	details := []string{
		fmt.Sprintf("Name: %s", rack.Name),
		fmt.Sprintf("Parent PDU ID: %s", rack.PDUID),
		fmt.Sprintf("Location: %s", rack.Location),
	}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeWarning, "save_rack", rack)
	return a, nil
}

// executeSaveRack persists a Rack confirmed via handleSubmitRackForm's dialog.
func (a *App) executeSaveRack(rack ctlplfl.Rack) tea.Cmd {
	return a.saveResourceCmd("Failed to save Rack: ", "Rack created successfully!", 2, func() error { return a.CPClient.PutRack(&rack) })
}

func (a *App) handleSubmitHypervisorForm(msg pages.SubmitHypervisorFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	var ipAddrs []string
	if msg.PrimaryIP != "" {
		ipAddrs = append(ipAddrs, msg.PrimaryIP)
	}
	if msg.AdditionalIPs != "" {
		for _, ip := range strings.Split(msg.AdditionalIPs, ",") {
			trimmed := strings.TrimSpace(ip)
			if trimmed != "" {
				ipAddrs = append(ipAddrs, trimmed)
			}
		}
	}
	rackID := strings.TrimSpace(msg.RackID)
	if rackID != "" {
		for _, r := range a.Racks {
			if strings.EqualFold(r.ID, rackID) || strings.EqualFold(r.Name, rackID) {
				rackID = r.ID
				break
			}
		}
	}
	// idOrNew reuses the original ID on edit (see HypervisorFormPage.InitialID)
	// — required, or PutHypervisor inserts a brand-new record instead.
	hv := ctlplfl.Hypervisor{
		ID:        idOrNew(msg.HypervisorID),
		RackID:    rackID,
		Name:      msg.Name,
		IPAddrs:   ipAddrs,
		SSHPort:   msg.SSHPort,
		PortRange: msg.PortRange,
	}
	title := "Confirm Hypervisor Creation"
	prompt := fmt.Sprintf("Save Hypervisor \"%s\"?", hv.Name)
	if msg.IsEdit {
		title = "Confirm Hypervisor Edit"
		prompt = fmt.Sprintf("Update Hypervisor \"%s\"?", hv.Name)
	}
	details := []string{
		fmt.Sprintf("Name: %s", hv.Name),
		fmt.Sprintf("Parent Rack ID: %s", hv.RackID),
		fmt.Sprintf("Primary IP: %s", msg.PrimaryIP),
	}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeWarning, "save_hypervisor", hv)
	return a, nil
}

// executeSaveHypervisor persists a Hypervisor confirmed via
// handleSubmitHypervisorForm's dialog.
func (a *App) executeSaveHypervisor(hv ctlplfl.Hypervisor) tea.Cmd {
	return a.saveResourceCmd("Failed to save Hypervisor: ", "Hypervisor created successfully!", 3, func() error { return a.CPClient.PutHypervisor(&hv) })
}

func (a *App) handleSubmitDeviceForm(msg pages.SubmitDeviceFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	sizeGB, _ := strconv.ParseInt(msg.SizeGB, 10, 64)
	dev := ctlplfl.Device{
		ID:           idOrNew(msg.DeviceID),
		HypervisorID: msg.HypervisorID,
		Name:         msg.Name,
		DevicePath:   msg.DevicePath,
		SerialNumber: msg.SerialNumber,
		Size:         sizeGB * 1024 * 1024 * 1024,
	}
	title := "Confirm Device Initialization"
	prompt := fmt.Sprintf("Initialize Device \"%s\"?", dev.Name)
	if msg.IsEdit {
		title = "Confirm Device Edit"
		prompt = fmt.Sprintf("Update Device \"%s\"?", dev.Name)
	}
	details := []string{
		fmt.Sprintf("Name: %s", dev.Name),
		fmt.Sprintf("Path: %s", dev.DevicePath),
		fmt.Sprintf("Size: %s GB", msg.SizeGB),
	}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeWarning, "save_device", dev)
	return a, nil
}

// executeSaveDevice persists a Device confirmed via handleSubmitDeviceForm's
// dialog.
func (a *App) executeSaveDevice(dev ctlplfl.Device) tea.Cmd {
	return a.saveResourceCmd("Failed to save Device: ", "Device initialized successfully!", 4, func() error { return a.CPClient.PutDevice(&dev) })
}

// partitionDiskRequest is the ConfirmDialog payload for "partition_disk".
type partitionDiskRequest struct {
	Device     ctlplfl.Device
	Hypervisor ctlplfl.Hypervisor
	Count      int
}

// partitionDiskDoneMsg reports the outcome of an SSH partitioning run
// (executePartitionDisk). Err set means the whole operation failed and
// nothing on screen should change beyond the status bar; otherwise Partitions
// is what's now physically on the device, for PartitionDiskResultPage.
type partitionDiskDoneMsg struct {
	DevicePath string
	Partitions []domain.DevicePartitionInfo
	Err        error
}

// handleSubmitPartitionDiskForm validates the partition count and resolves
// the device's hypervisor before handing off to a confirm dialog — this
// runs `parted` over SSH and (re)creates the device's partition table.
func (a *App) handleSubmitPartitionDiskForm(msg pages.SubmitPartitionDiskFormMsg) (tea.Model, tea.Cmd) {
	count, err := strconv.Atoi(strings.TrimSpace(msg.Count))
	if err != nil || count <= 0 {
		a.StatusMsg = "Error: number of partitions must be a positive number"
		a.StatusType = components.StatusError
		return a, nil
	}
	if count > 20 {
		a.StatusMsg = "Error: too many partitions requested (max 20)"
		a.StatusType = components.StatusError
		return a, nil
	}

	var hv domain.Hypervisor
	found := false
	for _, h := range a.AllHypervisors() {
		if h.ID == msg.HypervisorID {
			hv = h
			found = true
			break
		}
	}
	if !found {
		a.StatusMsg = "Error: hypervisor not found for this device"
		a.StatusType = components.StatusError
		return a, nil
	}

	req := partitionDiskRequest{
		Device:     ctlplfl.Device{ID: msg.DeviceID, HypervisorID: msg.HypervisorID, DevicePath: msg.DevicePath},
		Hypervisor: hv,
		Count:      count,
	}
	title := "Confirm Disk Partitioning"
	prompt := fmt.Sprintf("Create %d equal partitions on %s (%s)?", count, msg.DevicePath, hv.Name)
	details := []string{
		"This runs parted over SSH and (re)creates the device's partition table —",
		"any data currently on the device will be lost.",
		"Refuses to run if the device already has partitions.",
	}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeDanger, "partition_disk", req)
	return a, nil
}

// executePartitionDisk performs the actual SSH partitioning confirmed via
// handleSubmitPartitionDiskForm's dialog. Refuses to touch a device that
// already has partitions rather than silently wiping them.
func (a *App) executePartitionDisk(req partitionDiskRequest) tea.Cmd {
	return func() tea.Msg {
		deviceName := strings.TrimPrefix(req.Device.DevicePath, "/dev/")

		existing, err := domain.GetDevicePartitionInfo(req.Hypervisor, deviceName)
		if err == nil && len(existing) > 0 {
			return partitionDiskDoneMsg{Err: fmt.Errorf("%s already has %d partition(s) — refusing to overwrite", req.Device.DevicePath, len(existing))}
		}

		if err := domain.DeleteAllPartitionsFromDevice(req.Hypervisor, deviceName); err != nil {
			return partitionDiskDoneMsg{Err: fmt.Errorf("failed to prepare partition table: %w", err)}
		}
		if err := domain.CreateMultipleEqualPartitions(req.Hypervisor, deviceName, req.Count); err != nil {
			return partitionDiskDoneMsg{Err: fmt.Errorf("failed to create partitions: %w", err)}
		}

		result, err := domain.GetDevicePartitionInfo(req.Hypervisor, deviceName)
		if err != nil {
			return partitionDiskDoneMsg{Err: fmt.Errorf("partitions were created but could not be listed back: %w", err)}
		}
		return partitionDiskDoneMsg{DevicePath: req.Device.DevicePath, Partitions: result}
	}
}

// discoveredDevicesSaveRequest is the ConfirmDialog payload for
// "save_discovered_devices" — the checked subset of an SSH device scan,
// written straight via PutDevice with no per-device edit stop (mirrors
// main.go's old updateDeviceSelection batch-confirm flow).
type discoveredDevicesSaveRequest struct {
	HypervisorID string
	Devices      []ctlplfl.Device
}

func (a *App) handleSubmitDiscoveredDevices(msg pages.SubmitDiscoveredDevicesMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	if len(msg.Devices) == 0 {
		a.StatusMsg = "Error: no devices selected"
		a.StatusType = components.StatusError
		return a, nil
	}
	devs := make([]ctlplfl.Device, len(msg.Devices))
	details := make([]string, 0, len(msg.Devices))
	for i, d := range msg.Devices {
		// idOrNew reuses the device's own by-id-derived ID instead of
		// minting a fresh UUID — PutDevice upserts by ID, so this is what
		// makes re-discovering the same disk idempotent.
		devs[i] = ctlplfl.Device{
			ID:           idOrNew(d.ID),
			HypervisorID: msg.HypervisorID,
			Name:         d.Name,
			DevicePath:   d.DevicePath,
			SerialNumber: d.SerialNumber,
			Size:         d.Size,
		}
		details = append(details, fmt.Sprintf("%s (%s)", d.DevicePath, d.SerialNumber))
	}
	req := discoveredDevicesSaveRequest{HypervisorID: msg.HypervisorID, Devices: devs}
	prompt := fmt.Sprintf("Initialize %d discovered device(s)?", len(devs))
	a.ConfirmDialog = components.NewConfirmDialog("Confirm Device Initialization", prompt, details, components.ConfirmTypeWarning, "save_discovered_devices", req)
	return a, nil
}

// executeSaveDiscoveredDevices persists every device confirmed via
// handleSubmitDiscoveredDevices' dialog, one PutDevice call each — same
// aggregate-error-count pattern main.go used for its "add hypervisor with
// N selected devices" step, adapted to the standalone Add Disk flow.
func (a *App) executeSaveDiscoveredDevices(req discoveredDevicesSaveRequest) tea.Cmd {
	return func() tea.Msg {
		a.CPClient.SetToken(a.UserToken())
		errCount := 0
		var lastErr error
		for _, dev := range req.Devices {
			d := dev
			if err := a.CPClient.PutDevice(&d); err != nil {
				errCount++
				lastErr = err
			}
		}
		okCount := len(req.Devices) - errCount
		msg := resourceSavedMsg{Tab: 4}
		if errCount == 0 {
			msg.SuccessMsg = fmt.Sprintf("Initialized %d device(s) successfully!", okCount)
		} else if okCount == 0 {
			msg.FailPrefix = "Failed to initialize devices: "
			msg.Err = lastErr
		} else {
			msg.FailPrefix = fmt.Sprintf("Initialized %d/%d device(s), %d failed: ", okCount, len(req.Devices), errCount)
			msg.Err = lastErr
		}
		return msg
	}
}

func (a *App) handleSubmitPartitionForm(msg pages.SubmitPartitionFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	sizeGB, _ := strconv.ParseInt(msg.SizeGB, 10, 64)
	part := ctlplfl.DevicePartition{
		PartitionID:   idOrNew(msg.PartitionID),
		DevID:         msg.DeviceID,
		PartitionPath: msg.PartitionPath,
		Size:          sizeGB * 1024 * 1024 * 1024,
		NISDUUID:      msg.NISDUUID,
	}
	title := "Confirm Partition Creation"
	prompt := fmt.Sprintf("Create Partition \"%s\"?", part.PartitionPath)
	if msg.IsEdit {
		title = "Confirm Partition Edit"
		prompt = fmt.Sprintf("Update Partition \"%s\"?", part.PartitionPath)
	}
	details := []string{
		fmt.Sprintf("Partition Path: %s", part.PartitionPath),
		fmt.Sprintf("Size: %s GB", msg.SizeGB),
		fmt.Sprintf("Parent Device ID: %s", part.DevID),
	}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeWarning, "save_partition", part)
	return a, nil
}

// executeSavePartition persists a DevicePartition confirmed via
// handleSubmitPartitionForm's dialog. Reached only via Edit — Create routes
// through PartitionDiscoveryPage/handleSubmitDiscoveredPartitions instead.
func (a *App) executeSavePartition(part ctlplfl.DevicePartition) tea.Cmd {
	return func() tea.Msg {
		pw, ok := a.partitionWriter()
		if !ok {
			return resourceSavedMsg{FailPrefix: "Failed to create Partition: ", Tab: 5, Err: fmt.Errorf("partition management isn't supported against this backend")}
		}
		a.CPClient.SetToken(a.UserToken())
		err := pw.PutPartition(&part)
		return resourceSavedMsg{FailPrefix: "Failed to create Partition: ", SuccessMsg: "Partition created successfully!", Tab: 5, Err: err}
	}
}

// discoveredPartitionsSaveRequest is the ConfirmDialog payload for
// "save_discovered_partitions" — the checked subset of an SSH partition
// scan (PartitionDiscoveryPage).
type discoveredPartitionsSaveRequest struct {
	Partitions []ctlplfl.DevicePartition
}

func (a *App) handleSubmitDiscoveredPartitions(msg pages.SubmitDiscoveredPartitionsMsg) (tea.Model, tea.Cmd) {
	if !a.supportsPartitionWrite() {
		a.StatusMsg = "Error: partition management isn't supported against this backend"
		a.StatusType = components.StatusError
		return a, nil
	}
	if len(msg.Partitions) == 0 {
		a.StatusMsg = "Error: no partitions selected"
		a.StatusType = components.StatusError
		return a, nil
	}
	parts := make([]ctlplfl.DevicePartition, len(msg.Partitions))
	details := make([]string, 0, len(msg.Partitions))
	for i, info := range msg.Partitions {
		parts[i] = ctlplfl.DevicePartition{
			PartitionID:   info.Name,
			PartitionPath: info.Path,
			// NISDUUID left blank: a NISD doesn't exist for this partition
			// yet, and mdsvc-tidb's device_partition.nisd_uuid is a real
			// foreign key to nisd.id — pre-assigning a random UUID here
			// (as this used to do) inserts a reference to a NISD that
			// doesn't exist, which mdsvc-tidb correctly rejects (niova
			// -mdsvc has no such constraint, so this went unnoticed there).
			// buildCreateNISD mints the real ID once a NISD is actually
			// created against this partition.
			Size:  info.Size,
			DevID: msg.DeviceID,
		}
		details = append(details, fmt.Sprintf("%s (%.2f GB)", info.Path, float64(info.Size)/(1024*1024*1024)))
	}
	req := discoveredPartitionsSaveRequest{Partitions: parts}
	prompt := fmt.Sprintf("Register %d partition(s) found on disk?", len(parts))
	a.ConfirmDialog = components.NewConfirmDialog("Confirm Partition Registration", prompt, details, components.ConfirmTypeWarning, "save_discovered_partitions", req)
	return a, nil
}

// executeSaveDiscoveredPartitions persists every partition confirmed via
// handleSubmitDiscoveredPartitions' dialog, one PutPartition call each —
// same aggregate-error-count pattern executeSaveDiscoveredDevices uses.
func (a *App) executeSaveDiscoveredPartitions(req discoveredPartitionsSaveRequest) tea.Cmd {
	return func() tea.Msg {
		pw, ok := a.partitionWriter()
		if !ok {
			return resourceSavedMsg{FailPrefix: "Failed to register partitions: ", Tab: 5, Err: fmt.Errorf("partition management isn't supported against this backend")}
		}
		a.CPClient.SetToken(a.UserToken())
		errCount := 0
		var lastErr error
		for _, part := range req.Partitions {
			p := part
			if err := pw.PutPartition(&p); err != nil {
				errCount++
				lastErr = err
			}
		}
		okCount := len(req.Partitions) - errCount
		msg := resourceSavedMsg{Tab: 5}
		switch {
		case errCount == 0:
			msg.SuccessMsg = fmt.Sprintf("Registered %d partition(s) successfully!", okCount)
		case okCount == 0:
			msg.FailPrefix = "Failed to register partitions: "
			msg.Err = lastErr
		default:
			msg.FailPrefix = fmt.Sprintf("Registered %d/%d partition(s), %d failed: ", okCount, len(req.Partitions), errCount)
			msg.Err = lastErr
		}
		return msg
	}
}

// wholeDevicePartitionRequest is the ConfirmDialog payload for
// "save_whole_device_partition".
type wholeDevicePartitionRequest struct {
	Device     ctlplfl.Device
	Hypervisor ctlplfl.Hypervisor
}

// handleSubmitWholeDevicePartition resolves the device (and its
// hypervisor, needed to SSH in and read its by-id name) before handing off
// to a confirm dialog — reached when SSH scan found nothing on the device.
func (a *App) handleSubmitWholeDevicePartition(msg pages.SubmitWholeDevicePartitionMsg) (tea.Model, tea.Cmd) {
	if !a.supportsPartitionWrite() {
		a.StatusMsg = "Error: partition management isn't supported against this backend"
		a.StatusType = components.StatusError
		return a, nil
	}
	var device ctlplfl.Device
	var hv domain.Hypervisor
	found := false
	for _, h := range a.AllHypervisors() {
		for _, d := range h.Dev {
			if d.ID == msg.DeviceID {
				device, hv, found = d, h, true
			}
		}
	}
	if !found {
		a.StatusMsg = "Error: device not found"
		a.StatusType = components.StatusError
		return a, nil
	}
	req := wholeDevicePartitionRequest{Device: device, Hypervisor: hv}
	prompt := fmt.Sprintf("Register the entire device %s as a single partition?", device.DevicePath)
	details := []string{fmt.Sprintf("Size: %.2f GB", float64(device.Size)/(1024*1024*1024))}
	a.ConfirmDialog = components.NewConfirmDialog("Confirm Whole-Device Partition", prompt, details, components.ConfirmTypeWarning, "save_whole_device_partition", req)
	return a, nil
}

// executeSaveWholeDevicePartition persists the whole-device partition
// confirmed via handleSubmitWholeDevicePartition's dialog. Resolves the
// device's /dev/disk/by-id name over SSH first — that becomes the PartitionID.
func (a *App) executeSaveWholeDevicePartition(req wholeDevicePartitionRequest) tea.Cmd {
	return func() tea.Msg {
		deviceName := strings.TrimPrefix(req.Device.DevicePath, "/dev/")
		byIdName, err := domain.GetDeviceByIdName(req.Hypervisor, deviceName)
		if err != nil {
			return resourceSavedMsg{FailPrefix: "Failed to resolve device identity: ", Tab: 5, Err: err}
		}
		pw, ok := a.partitionWriter()
		if !ok {
			return resourceSavedMsg{FailPrefix: "Failed to register partition: ", Tab: 5, Err: fmt.Errorf("partition management isn't supported against this backend")}
		}
		part := ctlplfl.DevicePartition{
			PartitionID: byIdName,
			// No separate partition device node exists on this disk, so
			// the "path" is the device path itself, not a /dev/<dev>N
			// that doesn't exist.
			PartitionPath: req.Device.DevicePath,
			// NISDUUID left blank — see the identical note in
			// handleSubmitDiscoveredPartitions.
			Size:  req.Device.Size,
			DevID: req.Device.ID,
		}
		a.CPClient.SetToken(a.UserToken())
		err = pw.PutPartition(&part)
		return resourceSavedMsg{FailPrefix: "Failed to register whole-device partition: ", SuccessMsg: "Registered whole device as a single partition!", Tab: 5, Err: err}
	}
}

// findPartitionByID walks every known hypervisor's devices for the
// DevicePartition matching id, so a NISD-creation follow-up can update it
// without guessing at fields (PartitionPath in particular — PutPartition
// upserts the whole row, so sending a blank one would wipe the real value).
func (a *App) findPartitionByID(id string) (ctlplfl.DevicePartition, bool) {
	for _, hv := range a.AllHypervisors() {
		for _, dev := range hv.Dev {
			for _, part := range dev.Partitions {
				if part.PartitionID == id {
					return part, true
				}
			}
		}
	}
	return ctlplfl.DevicePartition{}, false
}

// linkPartitionToNISD writes nisdID back onto the partition target.TargetID
// resolves to, if any (a no-op for a whole-device target, which has no
// partition to update). Best-effort and silent on failure — the NISD
// itself is already created and usable either way; this is bookkeeping so
// "already has a NISD" can be detected next time (see
// domain.UsedNISDTargetIDs) and so the Partitions tab's "Assigned NISD
// UUID" column stops being permanently blank.
func (a *App) linkPartitionToNISD(target domain.NISDTarget, nisdID string) {
	part, found := a.findPartitionByID(target.TargetID)
	if !found {
		return
	}
	pw, ok := a.partitionWriter()
	if !ok {
		return
	}
	part.NISDUUID = nisdID
	_ = pw.PutPartition(&part)
}

// buildCreateNISD derives a fresh NISD from a single target — NISD UUID
// reuses the target's pre-assigned UUID when there is one (set when its
// partition was created), otherwise mints a fresh one; the port is
// allocated against the target's owning hypervisor, and Socket Path is
// derived from it so concurrently-created NISDs on the same host never
// collide, the same way their allocated ports don't. Shared by the single
// -target create path (handleSubmitNISDForm) and the batch multi-select
// path (executeSaveBatchNISDs) so both stay in lockstep.
func (a *App) buildCreateNISD(target domain.NISDTarget) (ctlplfl.Nisd, error) {
	var hv domain.Hypervisor
	found := false
	for _, h := range a.AllHypervisors() {
		if h.ID == target.HypervisorID {
			hv, found = h, true
			break
		}
	}
	if !found {
		return ctlplfl.Nisd{}, fmt.Errorf("could not find the hypervisor owning target %s", target.TargetID)
	}
	clientPort, serverPort, err := domain.AllocatePortPair(hv.ID, hv.PortRange, a.UserToken(), a.CPClient)
	if err != nil {
		return ctlplfl.Nisd{}, fmt.Errorf("failed to allocate a port for target %s: %w (set a Port Range on the Hypervisor first)", target.TargetID, err)
	}
	netInfos := make([]ctlplfl.NetworkInfo, 0, len(hv.IPAddrs))
	for _, ip := range hv.IPAddrs {
		netInfos = append(netInfos, ctlplfl.NetworkInfo{IPAddr: ip, Port: uint16(clientPort)})
	}
	return ctlplfl.Nisd{
		ID:            idOrNew(target.NISDUUID),
		SocketPath:    fmt.Sprintf("/var/run/nisd/nisd-%d.sock", serverPort),
		PeerPort:      uint16(serverPort),
		FailureDomain: target.FailureDomain,
		TotalSize:     target.Size,
		AvailableSize: target.Size,
		NetInfo:       netInfos,
		NetInfoCnt:    len(netInfos),
	}, nil
}

func (a *App) handleSubmitNISDForm(msg pages.SubmitNISDFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	// FailureDomain is what actually ties this NISD to a Device (mdsvc-tidb)
	// or Device+Partition (niova-mdsvc) — an empty target means nothing was
	// selected (or nothing was available to select), and PutNisd would
	// otherwise silently succeed with a NISD attached to nothing.
	if len(msg.FailureDomain) == 0 {
		a.StatusMsg = "Error: no target device/partition selected"
		a.StatusType = components.StatusError
		return a, nil
	}
	var nisd ctlplfl.Nisd
	if msg.IsEdit {
		// Editing an already-registered NISD: NISD UUID/Peer Port/Socket
		// Path are real values that may legitimately need hand-correction
		// (e.g. a port conflict), so the form still collects them directly
		// here. TotalSize/AvailableSize/NetInfo aren't editable fields —
		// carried over unchanged from msg.Initial, or PutNisd (which
		// overwrites the whole record, not a merge) would silently zero
		// them out on every edit.
		port, _ := strconv.ParseUint(msg.PeerPort, 10, 16)
		nisd = ctlplfl.Nisd{
			ID:            idOrNew(msg.NISDUUID),
			PeerPort:      uint16(port),
			SocketPath:    msg.SocketPath,
			FailureDomain: msg.FailureDomain,
			TotalSize:     msg.Initial.TotalSize,
			AvailableSize: msg.Initial.AvailableSize,
			NetInfo:       msg.Initial.NetInfo,
			NetInfoCnt:    msg.Initial.NetInfoCnt,
		}
	} else {
		// Create: NISD UUID/Peer Port/Socket Path are never typed by the
		// operator, all three are derived — see buildCreateNISD, shared with
		// the batch multi-select flow (executeSaveBatchNISDs).
		target := domain.NISDTarget{
			TargetID:      msg.TargetID,
			FailureDomain: msg.FailureDomain,
			NISDUUID:      msg.TargetNISDUUID,
			HypervisorID:  msg.TargetHypervisorID,
			Size:          msg.TargetSize,
		}
		var err error
		nisd, err = a.buildCreateNISD(target)
		if err != nil {
			a.StatusMsg = "Error: " + err.Error()
			a.StatusType = components.StatusError
			return a, nil
		}
	}
	details := []string{
		fmt.Sprintf("NISD UUID: %s", nisd.ID),
		fmt.Sprintf("Peer Port: %d", nisd.PeerPort),
		fmt.Sprintf("Socket Path: %s", nisd.SocketPath),
	}
	a.ConfirmDialog = components.NewConfirmDialog("Confirm NISD Initialization", fmt.Sprintf("Initialize NISD \"%s\"?", nisd.ID), details, components.ConfirmTypeWarning, "save_nisd", nisd)
	return a, nil
}

// executeSaveNISD persists a NISD confirmed via handleSubmitNISDForm's dialog.
func (a *App) executeSaveNISD(nisd ctlplfl.Nisd) tea.Cmd {
	return a.saveResourceCmd("Failed to initialize NISD: ", "NISD initialized successfully!", 6, func() error { return a.CPClient.PutNisd(&nisd) })
}

// handleSubmitNISDBatch confirms the checked targets from
// NISDBatchCreatePage before handing off to executeSaveBatchNISDs — no
// per-field editing here, same as the single-target create's confirm dialog.
func (a *App) handleSubmitNISDBatch(msg pages.SubmitNISDBatchMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	if len(msg.Targets) == 0 {
		a.StatusMsg = "Error: no targets selected"
		a.StatusType = components.StatusError
		return a, nil
	}
	title := "Confirm NISD Initialization"
	prompt := fmt.Sprintf("Initialize %d NISD(s)?", len(msg.Targets))
	details := make([]string, 0, len(msg.Targets))
	for _, t := range msg.Targets {
		details = append(details, t.Label)
	}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeWarning, "save_nisd_batch", msg.Targets)
	return a, nil
}

// executeSaveBatchNISDs creates one NISD per target confirmed via
// handleSubmitNISDBatch's dialog — same loop-and-aggregate pattern as
// executeSaveDiscoveredDevices/executeSaveBatchNISDs's mdsvc-tidb-only
// predecessor, restored from main.go's old initializeSelectedNISDs and now
// shared by both backends via buildCreateNISD. Ports are allocated and each
// NISD is persisted one at a time, in sequence: domain.AllocatePortPair
// picks a free port by querying the control plane for NISDs already
// registered on this hypervisor, so the next allocation only sees this one
// as taken once it's actually been PutNisd'd — allocating all of them up
// front could hand out the same port pair twice.
func (a *App) executeSaveBatchNISDs(targets []domain.NISDTarget) tea.Cmd {
	return func() tea.Msg {
		a.CPClient.SetToken(a.UserToken())
		errCount := 0
		var lastErr error
		for _, target := range targets {
			nisd, err := a.buildCreateNISD(target)
			if err != nil {
				errCount++
				lastErr = err
				continue
			}
			if err := a.CPClient.PutNisd(&nisd); err != nil {
				errCount++
				lastErr = fmt.Errorf("%s: %w", target.TargetID, err)
				continue
			}
			a.linkPartitionToNISD(target, nisd.ID)
		}
		okCount := len(targets) - errCount
		msg := resourceSavedMsg{Tab: 6}
		switch {
		case errCount == 0:
			msg.SuccessMsg = fmt.Sprintf("Initialized %d NISD(s) successfully!", okCount)
		case okCount == 0:
			msg.FailPrefix = "Failed to initialize NISDs: "
			msg.Err = lastErr
		default:
			msg.FailPrefix = fmt.Sprintf("Initialized %d/%d NISD(s), %d failed: ", okCount, len(targets), errCount)
			msg.Err = lastErr
		}
		return msg
	}
}

func (a *App) handleSubmitVdevForm(msg pages.SubmitVdevFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	sizeGB, _ := strconv.ParseInt(msg.SizeGB, 10, 64)
	dataCnt, _ := strconv.Atoi(msg.DataBlkCnt)
	parityCnt, _ := strconv.Atoi(msg.ParityBlkCnt)

	redMode := ctlplfl.RMReplica
	isEC := strings.EqualFold(msg.Redundancy, "ec")
	if isEC {
		redMode = ctlplfl.RMEC32K
	}

	// Mirrors mdsvc-tidb's own validation (internal/handlers/placement.go)
	// so a bad value is caught here with a clear message instead of a raw
	// "POST /api/vdev: http 400: {...}" dump from the server — every field
	// below is exactly what CreateVDev checks before provisioning.
	if sizeGB <= 0 {
		a.StatusMsg = "Error: Pool Size (GB) must be greater than 0"
		a.StatusType = components.StatusError
		return a, nil
	}
	if dataCnt < 1 {
		a.StatusMsg = "Error: Data Block Count must be at least 1"
		a.StatusType = components.StatusError
		return a, nil
	}
	if isEC && parityCnt < 1 {
		a.StatusMsg = "Error: Parity Block Count must be at least 1 for Erasure Coding"
		a.StatusType = components.StatusError
		return a, nil
	}
	if !isEC && parityCnt != 0 {
		a.StatusMsg = "Error: Parity Block Count must be 0 for Replica mode"
		a.StatusType = components.StatusError
		return a, nil
	}

	pfsID := msg.PFS
	var pfsName string
	if msg.PFS != "" {
		for _, p := range a.PFSs {
			if strings.EqualFold(p.ID, msg.PFS) || strings.EqualFold(p.Name, msg.PFS) {
				pfsID = p.ID
				pfsName = p.Name
				break
			}
		}
		if pfsName == "" {
			pfsName = msg.PFS
		}
	}

	vdev := ctlplfl.VdevConfig{
		ID:           uuid.NewString(),
		Name:         msg.Name,
		Size:         sizeGB * 1024 * 1024 * 1024,
		Redundancy:   redMode,
		DataBlkCnt:   uint8(dataCnt),
		ParityBlkCnt: uint8(parityCnt),
		FilterType:   msg.FilterType,
		PFSID:        pfsID,
		PFSName:      pfsName,
	}
	details := []string{
		fmt.Sprintf("Name: %s", vdev.Name),
		fmt.Sprintf("Redundancy Mode: %s", msg.Redundancy),
		fmt.Sprintf("Data/Parity: %dK / %dM", vdev.DataBlkCnt, vdev.ParityBlkCnt),
	}
	if pfsName != "" {
		details = append(details, fmt.Sprintf("PFS: %s", pfsName))
	}
	a.ConfirmDialog = components.NewConfirmDialog("Confirm Vdev Creation", fmt.Sprintf("Create Vdev Pool \"%s\"?", vdev.Name), details, components.ConfirmTypeWarning, "save_vdev", vdev)
	return a, nil
}

// executeSaveVdev persists a VdevConfig confirmed via handleSubmitVdevForm's
// dialog.
func (a *App) executeSaveVdev(vdev ctlplfl.VdevConfig) tea.Cmd {
	return a.saveResourceCmd("Failed to create Vdev: ", "Vdev pool created successfully!", 7, func() error { return a.CPClient.CreateVdev(&vdev) })
}

func (a *App) handleSubmitPFSForm(msg pages.SubmitPFSFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	// PFS.ID is server-assigned on creation, unlike other resources — leave
	// it empty for a new PFS; on edit, thread through the original ID.
	pfs := ctlplfl.PFS{Name: msg.Name}
	if msg.IsEdit {
		pfs.ID = msg.ID
	}
	title := "Confirm PFS Creation"
	prompt := fmt.Sprintf("Create File System \"%s\"?", pfs.Name)
	if msg.IsEdit {
		title = "Confirm PFS Edit"
		prompt = fmt.Sprintf("Update File System \"%s\"?", pfs.Name)
	}
	details := []string{
		fmt.Sprintf("Name: %s", pfs.Name),
	}
	req := pfsSaveRequest{PFS: pfs, IsEdit: msg.IsEdit}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeWarning, "save_pfs", req)
	return a, nil
}

// executeSavePFS persists a PFS confirmed via handleSubmitPFSForm's dialog.
// Editing requires the optional domain.PFSEditor capability (niova-mdsvc only).
func (a *App) executeSavePFS(req pfsSaveRequest) tea.Cmd {
	return func() tea.Msg {
		a.CPClient.SetToken(a.UserToken())
		pfs := req.PFS
		var err error
		if req.IsEdit {
			if editor, ok := a.CPClient.(domain.PFSEditor); ok {
				err = editor.EditPFS(&pfs)
			} else {
				err = fmt.Errorf("editing a PFS isn't supported against this backend")
			}
		} else {
			err = a.CPClient.CreatePFS(&pfs)
		}
		return resourceSavedMsg{FailPrefix: "Failed to save PFS: ", SuccessMsg: "Parallel File System saved successfully!", Tab: 8, Err: err}
	}
}

// executeDeleteVdev deletes a vdev pool confirmed via a "delete_vdev"
// ConfirmDialog (built in app.go's vdev list view). Requires
// domain.VdevDeleter (mdsvc-tidb only today — niova-mdsvc has a DeleteVdev
// endpoint but it isn't wired into ControlPlaneClient yet).
func (a *App) executeDeleteVdev(vdevID string) tea.Cmd {
	vd, canDelete := a.CPClient.(domain.VdevDeleter)
	if canDelete {
		a.CPClient.SetToken(a.UserToken())
	}
	return func() tea.Msg {
		if !canDelete {
			return vdevDeletedMsg{Err: fmt.Errorf("vdev management is only available against the mdsvc-tidb backend")}
		}
		err := vd.DeleteVdev(vdevID)
		return vdevDeletedMsg{Err: err}
	}
}

// importInfraRequest is the ConfirmDialog payload for "confirm_import_infra".
type importInfraRequest struct {
	FilePath string
	Request  *domain.ImportInfraRequest
}

// importInfraDoneMsg reports the outcome of an ImportInfra run. Unlike
// resourceSavedMsg's single Err, a bulk import can partially succeed — Result
// carries the full per-node report regardless, for ImportInfraResultPage.
type importInfraDoneMsg struct {
	FilePath string
	Result   *domain.ImportResult
}

// handleSubmitImportInfraForm shows a confirmation summarizing how many of
// each node type the file contains (ImportInfraFormPage has already parsed
// it, so this is just formatting) before anything is actually created.
func (a *App) handleSubmitImportInfraForm(msg pages.SubmitImportInfraFormMsg) (tea.Model, tea.Cmd) {
	if !a.requireCPClient() {
		return a, nil
	}
	pdus, racks, hvs, devices, nisds := msg.Request.Counts()
	title := "Confirm Infra Import"
	prompt := fmt.Sprintf("Import from %s?", msg.FilePath)
	details := []string{
		fmt.Sprintf("PDUs: %d", pdus),
		fmt.Sprintf("Racks: %d", racks),
		fmt.Sprintf("Hypervisors: %d", hvs),
		fmt.Sprintf("Devices: %d", devices),
		fmt.Sprintf("NISDs: %d", nisds),
		"",
		"This only creates — nothing existing is modified or removed.",
		"A node that fails partway through is skipped along with its children; everything already created stays created (no rollback).",
	}
	a.ConfirmDialog = components.NewConfirmDialog(title, prompt, details, components.ConfirmTypeWarning,
		"confirm_import_infra", importInfraRequest{FilePath: msg.FilePath, Request: msg.Request})
	return a, nil
}

// executeImportInfra runs the confirmed import via domain.ImportInfra.
func (a *App) executeImportInfra(req importInfraRequest) tea.Cmd {
	a.CPClient.SetToken(a.UserToken())
	return func() tea.Msg {
		result := domain.ImportInfra(a.CPClient, req.Request)
		return importInfraDoneMsg{FilePath: req.FilePath, Result: result}
	}
}

// executeDeleteTenant deletes a tenant confirmed via a "delete_tenant"
// ConfirmDialog (built in app.go's tenant list view). Requires
// domain.TenantManager (mdsvc-tidb only) plus an active tenant-admin session.
func (a *App) executeDeleteTenant(tenantUUID string) tea.Cmd {
	tm, isTenantCapable := a.CPClient.(domain.TenantManager)
	token := a.TenantAdmin.Token
	return func() tea.Msg {
		if !isTenantCapable {
			return pages.TenantDeletedMsg{Err: fmt.Errorf("tenant management is only available against the mdsvc-tidb backend")}
		}
		err := tm.DeleteTenant(token, tenantUUID)
		return pages.TenantDeletedMsg{Err: err}
	}
}

// executeDeleteAuthzPolicy deletes an RBAC or ABAC policy confirmed via a
// "delete_authz_policy" ConfirmDialog (built in app.go's authz policy view).
// Requires domain.AuthzManager (mdsvc-tidb only).
func (a *App) executeDeleteAuthzPolicy(target pages.AuthzDeleteTarget) tea.Cmd {
	am, isAuthzCapable := a.CPClient.(domain.AuthzManager)
	if isAuthzCapable {
		a.CPClient.SetToken(a.UserToken())
	}
	return func() tea.Msg {
		if !isAuthzCapable {
			return pages.AuthzPolicyDeletedMsg{Err: fmt.Errorf("authorization policy management is only available against the mdsvc-tidb backend")}
		}
		var err error
		if target.IsRBAC {
			err = am.DeleteRBACPolicy(target.RBAC)
		} else {
			err = am.DeleteABACPolicy(target.ABAC)
		}
		return pages.AuthzPolicyDeletedMsg{Err: err}
	}
}

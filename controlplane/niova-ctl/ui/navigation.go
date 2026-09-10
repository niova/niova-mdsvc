package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/pages"
)

// SwitchToTab is for *explicit* navigation (number/arrow key, :command)
// where an "Authentication required" error pre-login is correct; "return to
// wherever the user was" sites should call cancelForm() instead. Tabs
// 0/10/11 fetch over the network — callers must return the tea.Cmd (nil for
// every other, cache-only tab), or those three silently never finish loading.
func (a *App) SwitchToTab(tab int) tea.Cmd {
	if tab >= 1 && tab <= 9 && a.AuthEnabled && !a.IsLoggedIn() {
		a.StatusMsg = "Error: Authentication required to view storage resources"
		a.StatusType = components.StatusError
		return nil
	}

	a.ActiveTab = tab

	isAdmin := a.isAdmin()
	// Partitions are additionally blocked on backends that don't implement
	// domain.PartitionWriter (mdsvc-tidb has no partition endpoint at all) —
	// a capability check rather than a backend-enum check, so this stays
	// correct without editing if a future backend adds partition support.
	canWriteParts := isAdmin && a.supportsPartitionWrite()

	var cmd tea.Cmd
	switch tab {
	case 0:
		cmd = a.loadUsersView()
	case 1:
		a.ActivePage = pages.NewPDUViewPage(a.PDUs, a.TermWidth, a.TermHeight, isAdmin)
	case 2:
		a.ActivePage = pages.NewRackViewPage(a.Racks, a.PDUs, a.TermWidth, a.TermHeight, isAdmin)
	case 3:
		a.ActivePage = pages.NewHypervisorViewPage(a.PDUs, a.StandaloneHVs, a.TermWidth, a.TermHeight, isAdmin)
	case 4:
		a.ActivePage = pages.NewDeviceViewPage(a.AllHypervisors(), a.TermWidth, a.TermHeight, isAdmin)
	case 5:
		a.ActivePage = pages.NewPartitionViewPage(a.AllHypervisors(), a.TermWidth, a.TermHeight, canWriteParts)
	case 6:
		// Uses the cached copy — refreshCPDataCmd() keeps it current. Tab
		// switches must never block on network I/O.
		a.ActivePage = pages.NewNISDViewPage(a.NISDs, a.TermWidth, a.TermHeight, isAdmin)
	case 7:
		a.ActivePage = pages.NewVdevViewPage(a.Vdevs, a.TermWidth, a.TermHeight)
	case 8:
		a.ActivePage = pages.NewPFSViewPage(a.PFSs, a.TermWidth, a.TermHeight, isAdmin)
	case 9:
		a.ActivePage = pages.NewConfigViewPage(a.PDUs, a.StandaloneHVs, a.TermWidth, a.TermHeight)
	case 10:
		cmd = a.loadTenantsView()
	case 11:
		cmd = a.loadAuthzView()
	}
	a.ResizeActivePageTable()
	return cmd
}

// cancelForm backs out of the current form (Esc) to its tab, like
// SwitchToTab(a.ActiveTab) but never emitting its auth error. Safety net for
// any pages.FormPage not explicitly handled in the Esc key case (update.go).
func (a *App) cancelForm() tea.Cmd {
	if !a.IsLoggedIn() {
		return nil
	}
	return a.SwitchToTab(a.ActiveTab)
}

func (a *App) GetAvailableTabSequence() []int {
	var seq []int
	if a.IsLoggedIn() {
		seq = append(seq, 1, 2, 3, 4, 5, 6, 7, 8, 9)
		if a.isAdmin() {
			seq = append(seq, 0)
		}
	}
	if a.TenantAdmin.Token != "" {
		seq = append(seq, 10)
	}
	if a.IsLoggedIn() && a.hasAuthzAccess() {
		seq = append(seq, 11)
	}
	if len(seq) == 0 {
		seq = []int{0}
	}
	return seq
}

func (a *App) NextTab() tea.Cmd {
	tabSequence := a.GetAvailableTabSequence()
	currentIdx := 0
	for i, t := range tabSequence {
		if t == a.ActiveTab {
			currentIdx = i
			break
		}
	}
	nextIdx := (currentIdx + 1) % len(tabSequence)
	return a.SwitchToTab(tabSequence[nextIdx])
}

func (a *App) PrevTab() tea.Cmd {
	tabSequence := a.GetAvailableTabSequence()
	currentIdx := 0
	for i, t := range tabSequence {
		if t == a.ActiveTab {
			currentIdx = i
			break
		}
	}
	prevIdx := (currentIdx - 1 + len(tabSequence)) % len(tabSequence)
	return a.SwitchToTab(tabSequence[prevIdx])
}

func (a *App) IsInspecting() bool {
	if ip, ok := a.ActivePage.(pages.Inspecting); ok {
		return ip.IsInspecting()
	}
	return false
}

// IsFiltering reports whether the active page has a live search box open
// (see pages.Filtering) — checked alongside IsFormState()/IsInspecting()
// by update.go's global key dispatch.
func (a *App) IsFiltering() bool {
	if fp, ok := a.ActivePage.(pages.Filtering); ok {
		return fp.IsFiltering()
	}
	return false
}

// globalShortcutsBlocked reports whether a modal, form, or a page's own
// search box currently owns keyboard input, so update.go's single-key
// global shortcuts (tab switches, add/edit, refresh, ...) must not fire.
// requireNotInspecting additionally blocks shortcuts that would conflict
// with an open inspect-detail overlay.
func (a *App) globalShortcutsBlocked(requireNotInspecting bool) bool {
	if a.ConfirmDialog.Active || a.CommandBar.Active || a.HelpModal.Active || a.IsFormState() || a.IsFiltering() {
		return true
	}
	return requireNotInspecting && a.IsInspecting()
}

// FilterView returns the active page's search box, if any — see
// pages.FilterViewer. Rendered by view.go in a fixed slot below the
// header, independent of the active page's own height.
func (a *App) FilterView() string {
	if fv, ok := a.ActivePage.(pages.FilterViewer); ok {
		return fv.FilterView()
	}
	return ""
}

func (a *App) ResizeActivePageTable() {
	if a.TermHeight <= 0 {
		return
	}

	// The header grows by a row per two keybindings a page exposes, so
	// content space below it must grow too, or taller headers get pushed
	// off-screen.
	var keyBindings []key.Binding
	if a.ActivePage != nil {
		keyBindings = a.ActivePage.KeyBindings()
	}
	headerHeight := components.HeaderHeight(keyBindings, false, false)

	// tableOverhead covers the status line, search line, blank separator,
	// footer, outer panel border, and a table's own title/column-header
	// decoration — calibrated so a page with a "typical" 5-row header (e.g.
	// NISDs) fills the terminal exactly. Both status and search lines are
	// permanently-reserved (view.go), so this never needs conditional tuning.
	const tableOverhead = 12
	tableHeight := a.TermHeight - headerHeight - tableOverhead
	if tableHeight < 4 {
		tableHeight = 4
	}

	// viewportOverhead is smaller than tableOverhead: ConfigViewPage's own
	// title lives inside its scrollable content rather than as external
	// chrome, so it only needs the status/search/blank/footer/border overhead.
	const viewportOverhead = 6
	viewportHeight := a.TermHeight - headerHeight - viewportOverhead
	if viewportHeight < 4 {
		viewportHeight = 4
	}

	if r, ok := a.ActivePage.(pages.TableHeightSetter); ok {
		r.SetTableHeight(tableHeight)
	}
	if r, ok := a.ActivePage.(pages.ViewportHeightSetter); ok {
		r.SetViewportHeight(viewportHeight)
	}
	// Column widths are sized against terminal width at construction time
	// (autoSizeColumns) but bubbles/table has no live resize/scroll of its
	// own, so without this a table built at one width would stay stuck at
	// it — overflowing its Card on a narrower resize, or just leaving
	// unused space on a wider one.
	if r, ok := a.ActivePage.(pages.TableWidthSetter); ok {
		r.SetTableWidth(a.TermWidth)
	}
}

// TriggerAddForm builds the Add form for the active tab. Most forms need no
// follow-up I/O, so the returned tea.Cmd is nil for them — tabs 4/5 return
// one, to kick off SSH discovery via a Page with no Init() hook of its own.
func (a *App) TriggerAddForm() tea.Cmd {
	if a.AuthEnabled && !a.IsLoggedIn() {
		a.StatusMsg = "Error: Authentication required to add resources"
		a.StatusType = components.StatusError
		return nil
	}
	if a.ActiveTab == 5 && !a.supportsPartitionWrite() {
		a.StatusMsg = "Error: this backend has no partition resource — use 's' on the Disks tab to partition a device via SSH instead"
		a.StatusType = components.StatusError
		return nil
	}
	if tabRequiresAdmin(a.ActiveTab) && !a.isAdmin() {
		a.StatusMsg = "Error: admin role required for this action"
		a.StatusType = components.StatusError
		return nil
	}

	switch a.ActiveTab {
	case 0:
		a.ActivePage = pages.NewUserCreatePage(false)
	case 1:
		a.ActivePage = pages.NewPDUFormPage(false, domain.PDU{})
	case 2:
		a.ActivePage = pages.NewRackFormPage(false, domain.Rack{}, a.PDUs)
	case 3:
		a.ActivePage = pages.NewHypervisorFormPage(false, domain.Hypervisor{}, a.Racks)
	case 4:
		// Offer SSH device discovery before the manual form when there's a
		// hypervisor to scan; falls through to the plain form otherwise.
		// Uses AllHypervisors(), not StandaloneHVs, so rack-attached
		// hypervisors are reachable over SSH too.
		allHVs := a.AllHypervisors()
		if len(allHVs) > 0 {
			page, cmd := pages.NewDeviceDiscoveryPage(allHVs)
			a.ActivePage = page
			return cmd
		}
		a.ActivePage = pages.NewDeviceFormPage(false, domain.Device{}, allHVs)
	case 5:
		// Discover-then-select, not a blank manual form — registers a key
		// over a partition (or whole device) found via SSH. Edit (below)
		// is untouched, since it edits an already-registered record directly.
		devices := domain.BuildPartitionableDevices(a.PDUs, a.StandaloneHVs)
		page, cmd := pages.NewPartitionDiscoveryPage(devices)
		a.ActivePage = page
		return cmd
	case 6:
		usePartitions := a.supportsPartitionWrite()
		targets := domain.BuildNISDTargets(a.PDUs, a.StandaloneHVs, usePartitions)
		usedTargetIDs := domain.UsedNISDTargetIDs(a.NISDs)
		existingNISDIDs := domain.NISDIDSet(a.NISDs)
		a.ActivePage = pages.NewNISDBatchCreatePage(targets, usedTargetIDs, existingNISDIDs, usePartitions)
	case 7:
		a.ActivePage = pages.NewVdevFormPage(false, domain.VdevConfig{}, a.PFSs)
	case 8:
		a.ActivePage = pages.NewPFSFormPage(false, domain.PFS{})
	case 9:
		a.ActivePage = pages.NewImportInfraFormPage()
	}
	return nil
}

func (a *App) TriggerEditForm() {
	if a.AuthEnabled && !a.IsLoggedIn() {
		a.StatusMsg = "Error: Authentication required to edit resources"
		a.StatusType = components.StatusError
		return
	}
	if a.ActiveTab == 5 && !a.supportsPartitionWrite() {
		a.StatusMsg = "Error: this backend has no partition resource — use 's' on the Disks tab to partition a device via SSH instead"
		a.StatusType = components.StatusError
		return
	}
	if tabRequiresAdmin(a.ActiveTab) && !a.isAdmin() {
		a.StatusMsg = "Error: admin role required for this action"
		a.StatusType = components.StatusError
		return
	}

	es, ok := a.ActivePage.(pages.EditSource)
	if !ok {
		return
	}
	item, ok := es.SelectedForEdit()
	if !ok {
		return
	}

	switch v := item.(type) {
	case domain.PDU:
		a.ActivePage = pages.NewPDUFormPage(true, v)
	case domain.Rack:
		a.ActivePage = pages.NewRackFormPage(true, v, a.PDUs)
	case domain.Hypervisor:
		a.ActivePage = pages.NewHypervisorFormPage(true, v, a.Racks)
	case domain.Device:
		a.ActivePage = pages.NewDeviceFormPage(true, v, a.AllHypervisors())
	case domain.DevicePartition:
		var allDevs []domain.Device
		for _, hv := range a.AllHypervisors() {
			allDevs = append(allDevs, hv.Dev...)
		}
		a.ActivePage = pages.NewPartitionFormPage(true, v, allDevs)
	case domain.Nisd:
		usePartitions := a.supportsPartitionWrite()
		targets := domain.BuildNISDTargets(a.PDUs, a.StandaloneHVs, usePartitions)
		a.ActivePage = pages.NewNISDFormPage(true, v, targets, usePartitions)
	case domain.VdevConfig:
		a.ActivePage = pages.NewVdevFormPage(true, v, a.PFSs)
	case domain.PFS:
		a.ActivePage = pages.NewPFSFormPage(true, v)
	}
}

// TriggerPartitionDiskForm opens PartitionDiskFormPage for the device under
// the cursor on the Disks tab — an SSH utility action, not a control-plane
// resource, so it's never gated on domain.PartitionWriter.
func (a *App) TriggerPartitionDiskForm() {
	if a.AuthEnabled && !a.IsLoggedIn() {
		a.StatusMsg = "Error: Authentication required to partition disks"
		a.StatusType = components.StatusError
		return
	}
	if a.ActiveTab != 4 {
		return
	}
	if !a.isAdmin() {
		a.StatusMsg = "Error: admin role required for this action"
		a.StatusType = components.StatusError
		return
	}

	es, ok := a.ActivePage.(pages.EditSource)
	if !ok {
		return
	}
	item, ok := es.SelectedForEdit()
	if !ok {
		return
	}
	dev, ok := item.(domain.Device)
	if !ok {
		return
	}
	a.ActivePage = pages.NewPartitionDiskFormPage(dev)
}

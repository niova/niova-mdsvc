package pages

// Inspecting is implemented by view pages with an inline detail overlay
// toggled by enter and closed by esc.
type Inspecting interface{ IsInspecting() bool }

// TableHeightSetter is implemented by pages backed by a bubbles/table.Model
// (or, for AuthzPolicyPage, two of them) whose row count must track the
// terminal size.
type TableHeightSetter interface{ SetTableHeight(h int) }

// ViewportHeightSetter is implemented by pages backed by a
// bubbles/viewport.Model. Distinct method name from TableHeightSetter so a
// page with both fields can't structurally collide on either interface.
type ViewportHeightSetter interface{ SetViewportHeight(h int) }

// TableWidthSetter is implemented by pages backed by a bubbles/table.Model
// (or two, for AuthzPolicyPage) whose column widths must track terminal
// size — bubbles/table has no native resize of its own, so this is what
// makes columns reflow instead of staying fixed at construction-time width.
type TableWidthSetter interface{ SetTableWidth(w int) }

// EditSource is implemented by view pages that support editing the
// currently-selected row via 'e'. It returns the domain value under the
// cursor (e.g. domain.PDU), or ok=false if there's no valid selection.
type EditSource interface{ SelectedForEdit() (any, bool) }

// Filtering is implemented by table pages with a live name/UUID search
// (TablePage). The app's global key dispatch checks this so letter/digit
// shortcuts (tab-switch, add/edit, ...) pass through to the filter box
// instead of firing while the operator is typing a search term.
type Filtering interface{ IsFiltering() bool }

// FilterViewer is implemented by table pages with a live search box.
// FilterView returns its rendered "/ term" line, or "" when there's nothing
// to show — rendered in a permanently-reserved line (view.go) so opening or
// closing the search box never changes the layout's total height.
type FilterViewer interface{ FilterView() string }

// FormPage is implemented by input-form/auth pages, not table views —
// IsFormState()/the Esc case type-assert against this. isFormPage is
// unexported so only this package's types can implement it (sealed).
type FormPage interface {
	Page
	isFormPage()
}

// EscTargetKind is which list view EscTarget.EscAction wants Esc to reload.
type EscTargetKind int

const (
	EscTargetNone EscTargetKind = iota
	EscTargetTenants
	EscTargetUsers
	EscTargetAuthz
)

// EscTarget is implemented by pages reached outside the normal tab flow (via
// a command like :tenants/:authz, with no ActiveTab slot of their own) whose
// Esc should reload that same list view rather than fall through to generic
// form-cancel handling. Lets update.go dispatch on the returned kind instead
// of growing another page-type assertion each time one of these is added.
type EscTarget interface{ EscAction() EscTargetKind }

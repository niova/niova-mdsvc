package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/pages"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

func (a *App) View() string {
	if a.Quitting {
		return styles.TextMuted.Render("Goodbye!\n")
	}

	username := ""
	role := ""
	if a.Auth.User != nil {
		username = a.Auth.User.Username
		role = a.Auth.User.UserRole
	} else if a.TenantAdmin.Token != "" {
		username = a.TenantAdmin.Username
		role = "tenant-admin"
	}

	pageTitle := ""
	var keyBindings []key.Binding

	if a.ActivePage != nil {
		pageTitle = a.ActivePage.Title()
		keyBindings = a.ActivePage.KeyBindings()
	}

	metrics := a.Metrics

	// The tenant-admin login form always renders full-page, whether reached
	// pre-login via the chooser or via :tenant-login from an existing
	// session — resource nav has nothing to do with authenticating.
	_, isTenantAdminLoginPage := a.ActivePage.(*pages.TenantAdminLoginPage)
	_, isLoginChoicePage := a.ActivePage.(*pages.LoginChoicePage)
	_, isUserLoginPage := a.ActivePage.(*pages.UserLoginPage)
	_, isUserCreatePage := a.ActivePage.(*pages.UserCreatePage)
	isAuthPage := isLoginChoicePage || isUserLoginPage || isUserCreatePage || isTenantAdminLoginPage
	isFormState := a.IsFormState()
	isNonTabPage := false

	headerStr := components.RenderHeader(components.HeaderProps{
		Title:               "Niova Backend Configuration Tool",
		CPConnected:         a.CPConnected,
		AuthEnabled:         a.AuthEnabled,
		LoggedInUser:        username,
		Role:                role,
		Backend:             a.backendLabel(),
		Tenant:              a.tenantLabel(),
		Breadcrumbs:         pageTitle,
		Metrics:             metrics,
		ShowHotkeys:         true,
		HideResourceActions: isAuthPage || isFormState || isNonTabPage,
		HideGlobalShortcuts: isFormState,
		KeyBindings:         keyBindings,
	})

	pageView := ""
	if a.ConfirmDialog.Active {
		pageView = a.ConfirmDialog.View()
	} else if a.HelpModal.Active {
		pageView = a.HelpModal.View(pageTitle, keyBindings)
	} else if isAuthPage {
		pageView = a.ActivePage.View()
	} else {
		pageView = components.RenderDashboard(components.DashboardProps{
			ActiveTab:         a.ActiveTab,
			Metrics:           metrics,
			MainContentView:   a.ActivePage.View(),
			Width:             a.TermWidth,
			Height:            a.TermHeight,
			IsAdmin:           a.isAdmin(),
			HasResourceAccess: a.hasResourceAccess(),
			HasTenantAdmin:    a.TenantAdmin.Token != "",
			HasAuthzAccess:    a.hasAuthzAccess(),
		})
	}

	// statusStr and inputStr are each a permanently-reserved single line —
	// blank when unused — so status messages, search box, and command mode
	// never change the total layout height. ResizeActivePageTable's overhead
	// budgets assume exactly these two fixed lines; keep both in sync.
	statusStr := ""
	if a.StatusMsg != "" {
		msg := a.StatusMsg
		if a.Loading {
			msg = a.Spinner.View() + " " + msg
		}
		statusStr = components.RenderStatusBar(msg, a.StatusType)
	} else if a.Loading {
		statusStr = components.RenderStatusBar(a.Spinner.View()+" Loading...", components.StatusInfo)
	}

	// inputStr shares one line between command mode and a page's own search
	// box — the two never overlap (opening ':' is blocked while a page's
	// search box is focused, see update.go's ":" case), so there's no
	// ambiguity in always preferring CommandBar when it's active.
	inputStr := a.CommandBar.View()
	if inputStr == "" {
		inputStr = a.FilterView()
	}

	// ActiveTab defaults to 1 ("pdus") pre-login, so hide the footer pill on
	// auth screens rather than showing a misleading resource context.
	footerStr := ""
	if !isAuthPage {
		footerStr = components.RenderFooter(a.ActiveTab)
	}

	frame := lipgloss.JoinVertical(lipgloss.Left,
		headerStr,
		statusStr,
		inputStr,
		pageView,
		"",
		footerStr,
	)

	// Hard boundary: pages.availableTableWidth measures the real chrome, but
	// this is still the last-resort backstop if that's ever off by even one
	// character. MaxWidth truncates (ANSI-aware) any line that would
	// otherwise run past the right edge, regardless of what computed it.
	if a.TermWidth > 0 {
		frame = lipgloss.NewStyle().MaxWidth(a.TermWidth).Render(frame)
	}
	return frame
}

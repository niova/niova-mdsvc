package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

type FieldType int

const (
	FieldTypeText FieldType = iota
	FieldTypeSelect
)

type SelectOption struct {
	Label string
	Value string
}

// FieldDefinition defines a form input or dropdown selection field
type FieldDefinition struct {
	Type        FieldType
	Label       string
	Placeholder string
	CharLimit   int
	Value       string
	Options     []SelectOption

	// Validate, if set (FieldTypeText only), reports whether the field's
	// value is acceptable. A non-nil error shows an inline message and
	// blocks submission (FormModel.Valid()), but doesn't itself stop
	// invalid characters from being typed — pair with BlockInvalidChars.
	Validate func(string) error

	// BlockInvalidChars rejects a keystroke outright when it would make
	// Validate fail. Only safe when Validate is lenient toward incomplete
	// input (digits-only range checks) — one requiring a complete format
	// would reject every in-progress keystroke; use AllowedChars for those.
	BlockInvalidChars bool

	// AllowedChars, if set (FieldTypeText only), rejects a keystroke outright
	// for any rune it returns false for — checked per-rune, so unlike
	// BlockInvalidChars it can't reject a legitimate in-progress prefix
	// (e.g. "30" on the way to "30kW"). Validate's own error still only
	// shows once blurred, same as always.
	AllowedChars func(rune) bool

	// Suggestions, if set (FieldTypeText only), enables ghost-text
	// autocomplete against this fixed list. Bound to Right rather than
	// bubbles' default Tab, since Tab is FormModel's own next-field key.
	Suggestions []string
}

// FormModel encapsulates a group of text inputs & select dropdowns with keyboard navigation
type FormModel struct {
	Fields        []FieldDefinition
	Inputs        []textinput.Model
	SelectIndices []int // Current option index for FieldTypeSelect
	FocusedIndex  int
}

// NewForm constructs a FormModel from field definitions
func NewForm(fields []FieldDefinition) FormModel {
	inputs := make([]textinput.Model, len(fields))
	selectIndices := make([]int, len(fields))

	for i, f := range fields {
		selectIndices[i] = 0

		if f.Type == FieldTypeSelect {
			// Find initial selected option index if Value is provided
			if len(f.Options) > 0 {
				for optIdx, opt := range f.Options {
					if opt.Value == f.Value || opt.Label == f.Value {
						selectIndices[i] = optIdx
						break
					}
				}
			}
		} else {
			t := textinput.New()
			t.Placeholder = f.Placeholder
			t.Width = 44
			t.Prompt = "  "
			t.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
			t.PlaceholderStyle = lipgloss.NewStyle().Foreground(styles.ColorPlaceholder)
			if f.CharLimit > 0 {
				t.CharLimit = f.CharLimit
			} else {
				t.CharLimit = 512
			}
			if f.Validate != nil {
				t.Validate = textinput.ValidateFunc(f.Validate)
			}
			if len(f.Suggestions) > 0 {
				t.ShowSuggestions = true
				t.SetSuggestions(f.Suggestions)
				t.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("right"))
			}
			if f.Value != "" {
				t.SetValue(f.Value)
			} else if f.Validate != nil {
				// SetValue normally runs Validate; run it here too so a
				// required field is flagged from the start, not just after
				// its first edit.
				t.Err = f.Validate("")
			}
			lblLower := strings.ToLower(f.Label)
			if strings.Contains(lblLower, "secret") || strings.Contains(lblLower, "password") {
				t.EchoMode = textinput.EchoPassword
				t.EchoCharacter = '•'
			}
			if i == 0 {
				t.Focus()
			}
			inputs[i] = t
		}
	}

	// Focus first input or select
	if len(fields) > 0 && fields[0].Type == FieldTypeText {
		inputs[0].Focus()
	}

	return FormModel{
		Fields:        fields,
		Inputs:        inputs,
		SelectIndices: selectIndices,
		FocusedIndex:  0,
	}
}

// Update handles focus navigation (Tab, Shift+Tab, Up, Down) and option cycling (Left, Right, Space)
func (f *FormModel) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "tab", "down":
			f.NextField()
			return nil
		case "shift+tab", "up":
			f.PrevField()
			return nil
		case "left", "h":
			if f.CurrentField().Type == FieldTypeSelect {
				f.PrevOption()
				return nil
			}
		case "right", "l", "space":
			if f.CurrentField().Type == FieldTypeSelect {
				f.NextOption()
				return nil
			}
		}
	}

	for i := range f.Inputs {
		if f.Fields[i].Type != FieldTypeText || !f.Inputs[i].Focused() {
			continue
		}

		// AllowedChars: reject a keystroke whose runes aren't all accepted.
		// Both gates only apply to character insertion (tea.KeyRunes);
		// backspace/delete/arrows always go through untouched.
		if f.Fields[i].AllowedChars != nil {
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyRunes {
				allowed := true
				for _, r := range km.Runes {
					if !f.Fields[i].AllowedChars(r) {
						allowed = false
						break
					}
				}
				if !allowed {
					continue // reject: leave f.Inputs[i] untouched
				}
			}
		}

		// BlockInvalidChars: try the update on a copy first, and only keep
		// it if the field still validates.
		if f.Fields[i].BlockInvalidChars {
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyRunes {
				trial := f.Inputs[i]
				trial, cmd := trial.Update(msg)
				if trial.Err != nil {
					continue // reject: leave f.Inputs[i] untouched
				}
				f.Inputs[i] = trial
				cmds = append(cmds, cmd)
				continue
			}
		}

		var cmd tea.Cmd
		f.Inputs[i], cmd = f.Inputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

// Valid reports whether every text field's value satisfies its Validate
// function (fields without one always count as valid). Call before
// submitting, since bubbles' Validate only records Model.Err on its own.
func (f FormModel) Valid() bool {
	for i, field := range f.Fields {
		if field.Type == FieldTypeText && field.Validate != nil && f.Inputs[i].Err != nil {
			return false
		}
	}
	return true
}

func (f *FormModel) CurrentField() FieldDefinition {
	if f.FocusedIndex >= 0 && f.FocusedIndex < len(f.Fields) {
		return f.Fields[f.FocusedIndex]
	}
	return FieldDefinition{}
}

// NextField moves focus to next field
func (f *FormModel) NextField() {
	if len(f.Fields) == 0 {
		return
	}
	if f.Fields[f.FocusedIndex].Type == FieldTypeText {
		f.Inputs[f.FocusedIndex].Blur()
	}
	f.FocusedIndex = (f.FocusedIndex + 1) % len(f.Fields)
	if f.Fields[f.FocusedIndex].Type == FieldTypeText {
		f.Inputs[f.FocusedIndex].Focus()
	}
}

// PrevField moves focus to previous field
func (f *FormModel) PrevField() {
	if len(f.Fields) == 0 {
		return
	}
	if f.Fields[f.FocusedIndex].Type == FieldTypeText {
		f.Inputs[f.FocusedIndex].Blur()
	}
	f.FocusedIndex = (f.FocusedIndex - 1 + len(f.Fields)) % len(f.Fields)
	if f.Fields[f.FocusedIndex].Type == FieldTypeText {
		f.Inputs[f.FocusedIndex].Focus()
	}
}

// NextOption cycles to next select option
func (f *FormModel) NextOption() {
	idx := f.FocusedIndex
	opts := f.Fields[idx].Options
	if len(opts) > 0 {
		f.SelectIndices[idx] = (f.SelectIndices[idx] + 1) % len(opts)
	}
}

// PrevOption cycles to previous select option
func (f *FormModel) PrevOption() {
	idx := f.FocusedIndex
	opts := f.Fields[idx].Options
	if len(opts) > 0 {
		f.SelectIndices[idx] = (f.SelectIndices[idx] - 1 + len(opts)) % len(opts)
	}
}

// Value returns value of field at index
func (f FormModel) Value(index int) string {
	if index < 0 || index >= len(f.Fields) {
		return ""
	}
	field := f.Fields[index]
	if field.Type == FieldTypeSelect {
		opts := field.Options
		if len(opts) == 0 {
			return ""
		}
		optIdx := f.SelectIndices[index]
		if optIdx >= 0 && optIdx < len(opts) {
			return opts[optIdx].Value
		}
		return ""
	}
	return strings.TrimSpace(f.Inputs[index].Value())
}

// View renders all fields (text inputs & select dropdowns) inside a styled card
func (f FormModel) View() string {
	var elements []string
	for i, field := range f.Fields {
		label := field.Label
		isFocused := i == f.FocusedIndex

		if isFocused {
			label = styles.SelectedItem.Render("▶ " + label)
		} else {
			label = styles.UnselectedItem.Render("  " + label)
		}

		var renderedField string
		if field.Type == FieldTypeSelect {
			var optLabel string
			if len(field.Options) == 0 {
				optLabel = "No options available"
			} else {
				optIdx := f.SelectIndices[i]
				if optIdx >= 0 && optIdx < len(field.Options) {
					optLabel = field.Options[optIdx].Label
				}
			}
			displayStr := "◄  " + optLabel + "  ►"
			if isFocused {
				renderedField = styles.InputFocused.Render(displayStr) + " " + styles.TextMuted.Render("(◄/► or Space to select)")
			} else {
				renderedField = styles.InputBlurred.Render(displayStr)
			}
		} else {
			input := f.Inputs[i]
			// Error styling only shows once blurred — an in-progress value
			// like "900" is a valid prefix of "9000-9100" and shouldn't
			// look broken mid-edit. Valid() still gates on the real Err.
			switch {
			case input.Err != nil && !isFocused:
				renderedField = styles.InputError.Render(input.View())
			case isFocused:
				renderedField = styles.InputFocused.Render(input.View())
			default:
				renderedField = styles.InputBlurred.Render(input.View())
			}
		}

		element := lipgloss.JoinVertical(lipgloss.Left, label, renderedField)
		if field.Type == FieldTypeText && !isFocused && f.Inputs[i].Err != nil {
			element = lipgloss.JoinVertical(lipgloss.Left, element, styles.TextError.Render("  "+f.Inputs[i].Err.Error()))
		}
		elements = append(elements, element)
	}

	content := strings.Join(elements, "\n")
	return styles.Card.Render(content)
}

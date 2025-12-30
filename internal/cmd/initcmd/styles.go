package initcmd

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Theme colors - using CertWatch brand colors
var (
	colorPrimary   = lipgloss.Color("#0EA5E9") // Sky blue
	colorSuccess   = lipgloss.Color("#22C55E") // Green
	colorWarning   = lipgloss.Color("#F59E0B") // Amber
	colorError     = lipgloss.Color("#EF4444") // Red
	colorMuted     = lipgloss.Color("#6B7280") // Gray
	colorHighlight = lipgloss.Color("#A855F7") // Purple
	colorDark      = lipgloss.Color("#1F2937") // Dark gray
	colorLight     = lipgloss.Color("#F9FAFB") // Light gray
)

// Component styles
var (
	// TitleStyle for section headers
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	// SuccessStyle for success messages
	SuccessStyle = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	// ErrorStyle for error messages
	ErrorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	// WarningStyle for warning messages
	WarningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	// MutedStyle for secondary text
	MutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	// CodeStyle for command/code display
	CodeStyle = lipgloss.NewStyle().
			Background(colorDark).
			Foreground(colorLight).
			Padding(0, 1)

	// BoxStyle for summary sections
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			MarginTop(1)

	// HeaderStyle for the main header
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Background(lipgloss.Color("#1E3A5F")).
			Padding(0, 2).
			MarginBottom(1)

	// SectionStyle for section dividers
	SectionStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1).
			MarginBottom(1)
)

// Prefixes for messages
const (
	SuccessPrefix = "✓ "
	ErrorPrefix   = "✗ "
	WarningPrefix = "! "
	InfoPrefix    = "→ "
)

// CreateTheme returns a custom huh theme matching CertWatch branding.
func CreateTheme() *huh.Theme {
	t := huh.ThemeBase()

	// Customize focused state
	t.Focused.Title = t.Focused.Title.Foreground(colorPrimary)
	t.Focused.Description = t.Focused.Description.Foreground(colorMuted)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(colorHighlight)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(colorPrimary)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(colorPrimary)

	// Customize blurred state
	t.Blurred.Title = t.Blurred.Title.Foreground(colorMuted)

	return t
}

// RenderHeader renders the main wizard header.
func RenderHeader() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorLight).
		Background(colorPrimary).
		Padding(0, 2).
		Render(" CertWatch Agent Setup ")

	return header
}

// RenderSection renders a section divider.
func RenderSection(title string) string {
	line := "─────────────────────────────────────────"
	return SectionStyle.Render("─── " + title + " " + line[:40-len(title)])
}

// RenderSuccess renders a success message.
func RenderSuccess(msg string) string {
	return SuccessStyle.Render(SuccessPrefix + msg)
}

// RenderError renders an error message.
func RenderError(msg string) string {
	return ErrorStyle.Render(ErrorPrefix + msg)
}

// RenderWarning renders a warning message.
func RenderWarning(msg string) string {
	return WarningStyle.Render(WarningPrefix + msg)
}

// RenderInfo renders an info message.
func RenderInfo(msg string) string {
	return MutedStyle.Render(InfoPrefix + msg)
}

// RenderCode renders a code/command.
func RenderCode(code string) string {
	return CodeStyle.Render(code)
}

package initcmd

import (
	"fmt"

	"github.com/charmbracelet/huh"

	"github.com/certwatch-app/cw-agent/internal/ui"
)

// NewWelcomeForm creates the welcome and file configuration form.
func NewWelcomeForm(state *WizardState) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Welcome to CertWatch Agent Setup!").
				Description("This wizard will help you create a configuration file for the CertWatch Agent.\n\n"+
					"You'll need:\n"+
					"  • Your CertWatch API key (from https://certwatch.app/settings/api-keys)\n"+
					"  • Hostnames of certificates you want to monitor"),

			huh.NewInput().
				Title("Config file path").
				Description("Where to save the configuration file").
				Placeholder("./certwatch.yaml").
				Value(&state.ConfigPath).
				Validate(ValidateConfigPath),
		),
	).WithTheme(ui.CreateTheme())
}

// NewAPIForm creates the API configuration form.
func NewAPIForm(state *WizardState) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("API Configuration").
				Description("Connect to your CertWatch account"),

			huh.NewInput().
				Title("CertWatch API Key").
				Description("Your API key with 'cloud:sync' scope").
				Placeholder("cw_xxxxxxxx_xxxxxxxxxxxx").
				Value(&state.APIKey).
				EchoMode(huh.EchoModePassword).
				Validate(ValidateAPIKey),
		),
	).WithTheme(ui.CreateTheme())
}

// NewAgentForm creates the agent configuration form.
func NewAgentForm(state *WizardState) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Agent Configuration").
				Description("Configure agent behavior"),

			huh.NewInput().
				Title("Agent Name").
				Description("A unique name to identify this agent (e.g., production-azure-monitor)").
				Placeholder("my-agent").
				Value(&state.AgentName).
				Validate(ValidateAgentName),

			huh.NewSelect[string]().
				Title("Sync Interval").
				Description("How often to sync with CertWatch cloud").
				Options(
					huh.NewOption("1 minute", "1m"),
					huh.NewOption("5 minutes (recommended)", "5m"),
					huh.NewOption("15 minutes", "15m"),
					huh.NewOption("30 minutes", "30m"),
				).
				Value(&state.SyncInterval),

			huh.NewSelect[string]().
				Title("Scan Interval").
				Description("How often to scan certificates locally").
				Options(
					huh.NewOption("30 seconds", "30s"),
					huh.NewOption("1 minute (recommended)", "1m"),
					huh.NewOption("5 minutes", "5m"),
				).
				Value(&state.ScanInterval),

			huh.NewSelect[string]().
				Title("Log Level").
				Description("Logging verbosity").
				Options(
					huh.NewOption("Debug (verbose)", "debug"),
					huh.NewOption("Info (recommended)", "info"),
					huh.NewOption("Warn", "warn"),
					huh.NewOption("Error (quiet)", "error"),
				).
				Value(&state.LogLevel),
		),
	).WithTheme(ui.CreateTheme())
}

// NewAdvancedForm creates the advanced configuration form for observability.
func NewAdvancedForm(state *WizardState) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Observability Settings").
				Description("Configure metrics and health monitoring (optional)"),

			huh.NewSelect[string]().
				Title("Metrics Server Port").
				Description("Port for Prometheus metrics endpoint (/metrics). Set to 0 to disable.").
				Options(
					huh.NewOption("8080 (default)", "8080"),
					huh.NewOption("9090", "9090"),
					huh.NewOption("3000", "3000"),
					huh.NewOption("Disabled", "0"),
				).
				Value(&state.MetricsPort),

			huh.NewSelect[string]().
				Title("Heartbeat Interval").
				Description("How often to send heartbeats for downtime alerts. Set to 0 to disable.").
				Options(
					huh.NewOption("30 seconds (recommended)", "30s"),
					huh.NewOption("1 minute", "1m"),
					huh.NewOption("5 minutes", "5m"),
					huh.NewOption("Disabled", "0"),
				).
				Value(&state.HeartbeatInterval),
		),
	).WithTheme(ui.CreateTheme())
}

// NewCertificateForm creates a certificate entry form.
func NewCertificateForm(state *WizardState, certNum int) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("Certificate #%d", certNum)).
				Description("Add a certificate endpoint to monitor"),

			huh.NewInput().
				Title("Hostname").
				Description("The hostname to check (e.g., api.example.com)").
				Placeholder("api.example.com").
				Value(&state.CurrentCert.Hostname).
				Validate(ValidateHostname),

			huh.NewInput().
				Title("Port").
				Description("TLS port (default: 443)").
				Placeholder("443").
				Value(&state.CurrentCert.PortStr).
				Validate(ValidatePort),

			huh.NewInput().
				Title("Tags (comma-separated)").
				Description("Optional tags for organization").
				Placeholder("production, api").
				Value(&state.CurrentCert.Tags).
				Validate(ValidateTags),

			huh.NewInput().
				Title("Notes").
				Description("Optional notes about this certificate").
				Placeholder("Main API endpoint").
				Value(&state.CurrentCert.Notes).
				Validate(ValidateNotes),

			huh.NewConfirm().
				Title("Add another certificate?").
				Value(&state.AddAnother).
				Affirmative("Yes").
				Negative("No"),
		),
	).WithTheme(ui.CreateTheme())
}

// NewOverwriteConfirmForm creates a form to confirm file overwrite.
func NewOverwriteConfirmForm(state *WizardState, path string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("File '%s' already exists. Overwrite?", path)).
				Description("The existing file will be replaced with the new configuration.").
				Value(&state.OverwriteFile).
				Affirmative("Yes, overwrite").
				Negative("No, cancel"),
		),
	).WithTheme(ui.CreateTheme())
}

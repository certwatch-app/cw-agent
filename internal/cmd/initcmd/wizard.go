package initcmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/charmbracelet/huh"
)

// Wizard manages the interactive configuration wizard.
type Wizard struct {
	state      *WizardState
	outputPath string
}

// NewWizard creates a new wizard instance.
func NewWizard() *Wizard {
	return &Wizard{
		state: NewWizardState(),
	}
}

// SetOutputPath sets the output path (from command line flag).
func (w *Wizard) SetOutputPath(path string) {
	w.outputPath = path
	if path != "" {
		w.state.ConfigPath = path
	}
}

// Run executes the wizard flow.
func (w *Wizard) Run() error {
	// Setup signal handling for graceful Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		fmt.Println()
		fmt.Println(RenderWarning("Setup canceled by user"))
		os.Exit(0)
	}()

	// Print header
	fmt.Println()
	fmt.Println(RenderHeader())
	fmt.Println()

	// Step 1: Welcome and file configuration
	if err := w.runWelcomeForm(); err != nil {
		return w.handleError(err)
	}

	// Step 2: Check for existing file
	if err := w.handleExistingFile(); err != nil {
		return err
	}

	// Step 3: API configuration
	fmt.Println(RenderSection("API Configuration"))
	if err := w.runAPIForm(); err != nil {
		return w.handleError(err)
	}

	// Step 4: Agent configuration
	fmt.Println(RenderSection("Agent Configuration"))
	if err := w.runAgentForm(); err != nil {
		return w.handleError(err)
	}

	// Step 5: Certificate configuration (loop)
	fmt.Println(RenderSection("Certificates to Monitor"))
	if err := w.runCertificateForms(); err != nil {
		return w.handleError(err)
	}

	// Step 6: Generate and validate config
	cfg, err := w.state.ToConfig()
	if err != nil {
		return w.handleError(fmt.Errorf("failed to create configuration: %w", err))
	}

	if err := cfg.Validate(); err != nil {
		return w.handleValidationError(err)
	}

	// Step 7: Write config file
	fmt.Println()
	if err := WriteConfig(cfg, w.state.ConfigPath); err != nil {
		return w.handleError(err)
	}

	// Step 8: Show success and next steps
	w.showSuccess()

	return nil
}

func (w *Wizard) runWelcomeForm() error {
	form := NewWelcomeForm(w.state)
	return form.Run()
}

func (w *Wizard) runAPIForm() error {
	form := NewAPIForm(w.state)
	return form.Run()
}

func (w *Wizard) runAgentForm() error {
	form := NewAgentForm(w.state)
	return form.Run()
}

func (w *Wizard) runCertificateForms() error {
	certNum := 1

	for {
		// Reset current cert for new entry
		w.state.ResetCurrentCert()

		// Run certificate form
		form := NewCertificateForm(w.state, certNum)
		if err := form.Run(); err != nil {
			return err
		}

		// Save the certificate
		w.state.SaveCurrentCert()

		// Check if user wants to add more
		if !w.state.AddAnother {
			break
		}

		certNum++
	}

	// Validate we have at least one certificate
	if len(w.state.Certificates) == 0 {
		return fmt.Errorf("at least one certificate is required")
	}

	return nil
}

func (w *Wizard) handleExistingFile() error {
	if !FileExists(w.state.ConfigPath) {
		return nil
	}

	form := NewOverwriteConfirmForm(w.state, w.state.ConfigPath)
	if err := form.Run(); err != nil {
		return w.handleError(err)
	}

	if !w.state.OverwriteFile {
		fmt.Println(RenderWarning("Setup canceled: file already exists"))
		os.Exit(0)
	}

	return nil
}

func (w *Wizard) handleError(err error) error {
	if err == huh.ErrUserAborted {
		fmt.Println()
		fmt.Println(RenderWarning("Setup canceled"))
		os.Exit(0)
	}
	fmt.Println()
	fmt.Println(RenderError(err.Error()))
	return err
}

func (w *Wizard) handleValidationError(err error) error {
	fmt.Println()
	fmt.Println(RenderError("Configuration validation failed:"))
	fmt.Println(RenderError("  " + err.Error()))
	fmt.Println()
	fmt.Println(RenderInfo("Please run 'cw-agent init' again with corrected values."))
	return err
}

func (w *Wizard) showSuccess() {
	fmt.Println()
	fmt.Println(RenderSuccess("Config written to " + w.state.ConfigPath))
	fmt.Println(RenderSuccess("Validated successfully"))
	fmt.Println()

	// Show summary
	fmt.Println(TitleStyle.Render("Configuration Summary:"))
	fmt.Println(MutedStyle.Render("  Agent:        ") + w.state.AgentName)
	fmt.Println(MutedStyle.Render("  Certificates: ") + fmt.Sprintf("%d", len(w.state.Certificates)))
	fmt.Println(MutedStyle.Render("  Sync:         ") + w.state.SyncInterval)
	fmt.Println(MutedStyle.Render("  Scan:         ") + w.state.ScanInterval)
	fmt.Println()

	fmt.Println(TitleStyle.Render("Next steps:"))
	fmt.Println()
	fmt.Println("  To validate your config:")
	fmt.Println("    " + RenderCode("cw-agent validate -c "+w.state.ConfigPath))
	fmt.Println()
	fmt.Println("  To start monitoring:")
	fmt.Println("    " + RenderCode("cw-agent start -c "+w.state.ConfigPath))
	fmt.Println()
}

// RunNonInteractive runs the wizard in non-interactive mode using environment variables.
func RunNonInteractive(outputPath string) error {
	state := NewWizardState()
	state.ConfigPath = outputPath

	// Read from environment variables
	state.APIKey = os.Getenv("CW_API_KEY")
	if state.APIKey == "" {
		return fmt.Errorf("CW_API_KEY environment variable is required in non-interactive mode")
	}

	if endpoint := os.Getenv("CW_API_ENDPOINT"); endpoint != "" {
		state.APIEndpoint = endpoint
	}

	if name := os.Getenv("CW_AGENT_NAME"); name != "" {
		state.AgentName = name
	} else {
		state.AgentName = "default-agent"
	}

	if interval := os.Getenv("CW_SYNC_INTERVAL"); interval != "" {
		state.SyncInterval = interval
	}

	if interval := os.Getenv("CW_SCAN_INTERVAL"); interval != "" {
		state.ScanInterval = interval
	}

	if level := os.Getenv("CW_LOG_LEVEL"); level != "" {
		state.LogLevel = level
	}

	// Parse certificates from CW_CERTIFICATES (comma-separated hostnames)
	certsEnv := os.Getenv("CW_CERTIFICATES")
	if certsEnv != "" {
		for _, hostname := range strings.Split(certsEnv, ",") {
			hostname = strings.TrimSpace(hostname)
			if hostname != "" {
				state.Certificates = append(state.Certificates, CertificateInput{
					Hostname: hostname,
					PortStr:  "443",
				})
			}
		}
	}

	// Validate we have certificates
	if len(state.Certificates) == 0 {
		return fmt.Errorf("CW_CERTIFICATES environment variable is required (comma-separated hostnames)")
	}

	// Convert and validate
	cfg, err := state.ToConfig()
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Write config
	if err := WriteConfig(cfg, state.ConfigPath); err != nil {
		return err
	}

	fmt.Println(RenderSuccess("Config written to " + state.ConfigPath))
	return nil
}

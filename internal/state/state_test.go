package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	configPath := "/etc/certwatch/certwatch.yaml"
	m := NewManager(configPath)

	expectedStatePath := "/etc/certwatch/.certwatch-state.json"
	if m.filePath != expectedStatePath {
		t.Errorf("expected filePath %q, got %q", expectedStatePath, m.filePath)
	}

	if m.state == nil {
		t.Error("expected state to be initialized")
	}
}

func TestLoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "certwatch.yaml")

	m := NewManager(configPath)
	err := m.Load()

	if err != nil {
		t.Errorf("expected no error for non-existent file, got %v", err)
	}

	if m.GetAgentID() != "" {
		t.Error("expected empty agent ID for first run")
	}

	if m.GetAgentName() != "" {
		t.Error("expected empty agent name for first run")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "certwatch.yaml")

	// Create and save state
	m1 := NewManager(configPath)
	m1.SetAgentID("test-agent-id-123")
	m1.SetAgentName("production-monitor")
	m1.SetLastSyncAt(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))

	if err := m1.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Verify file was created
	statePath := filepath.Join(tmpDir, stateFileName)
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("state file was not created")
	}

	// Check file permissions (Unix only)
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("failed to stat state file: %v", err)
	}
	mode := info.Mode().Perm()
	// On Windows, permissions work differently, so we just check it's not world-readable
	if mode&0077 != 0 && os.Getenv("OS") != "Windows_NT" {
		t.Errorf("expected restricted permissions, got %o", mode)
	}

	// Load state in new manager
	m2 := NewManager(configPath)
	if err := m2.Load(); err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if m2.GetAgentID() != "test-agent-id-123" {
		t.Errorf("expected agent ID %q, got %q", "test-agent-id-123", m2.GetAgentID())
	}

	if m2.GetAgentName() != "production-monitor" {
		t.Errorf("expected agent name %q, got %q", "production-monitor", m2.GetAgentName())
	}

	expectedSync := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	if !m2.GetLastSyncAt().Equal(expectedSync) {
		t.Errorf("expected last sync at %v, got %v", expectedSync, m2.GetLastSyncAt())
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "certwatch.yaml")
	statePath := filepath.Join(tmpDir, stateFileName)

	// Write corrupted JSON
	if err := os.WriteFile(statePath, []byte("not valid json"), 0600); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	m := NewManager(configPath)
	err := m.Load()

	// Should return error for corrupted file
	if err == nil {
		t.Error("expected error for corrupted file")
	}

	// But state should be initialized (treating as first run)
	if m.GetAgentID() != "" {
		t.Error("expected empty agent ID after corrupted file")
	}
}

func TestHasNameChanged(t *testing.T) {
	tests := []struct {
		name        string
		storedName  string
		configName  string
		wantChanged bool
	}{
		{
			name:        "first run - no stored name",
			storedName:  "",
			configName:  "new-agent",
			wantChanged: false,
		},
		{
			name:        "same name",
			storedName:  "production-monitor",
			configName:  "production-monitor",
			wantChanged: false,
		},
		{
			name:        "name changed",
			storedName:  "production-monitor",
			configName:  "new-monitor",
			wantChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{
				state: &State{AgentName: tt.storedName},
			}

			got := m.HasNameChanged(tt.configName)
			if got != tt.wantChanged {
				t.Errorf("HasNameChanged() = %v, want %v", got, tt.wantChanged)
			}
		})
	}
}

func TestHasState(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		agentNam string
		want     bool
	}{
		{
			name:     "no state",
			agentID:  "",
			agentNam: "",
			want:     false,
		},
		{
			name:     "has agent ID",
			agentID:  "some-id",
			agentNam: "",
			want:     true,
		},
		{
			name:     "has agent name",
			agentID:  "",
			agentNam: "some-name",
			want:     true,
		},
		{
			name:     "has both",
			agentID:  "some-id",
			agentNam: "some-name",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{
				state: &State{
					AgentID:   tt.agentID,
					AgentName: tt.agentNam,
				},
			}

			got := m.HasState()
			if got != tt.want {
				t.Errorf("HasState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPreviousAgentID(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "certwatch.yaml")

	m := NewManager(configPath)

	// Initially empty
	if m.GetPreviousAgentID() != "" {
		t.Error("expected empty previous agent ID initially")
	}

	// Set previous agent ID
	m.SetPreviousAgentID("old-agent-id-456")
	if m.GetPreviousAgentID() != "old-agent-id-456" {
		t.Errorf("expected previous agent ID %q, got %q", "old-agent-id-456", m.GetPreviousAgentID())
	}

	// Save and reload
	if err := m.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	m2 := NewManager(configPath)
	if err := m2.Load(); err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if m2.GetPreviousAgentID() != "old-agent-id-456" {
		t.Errorf("expected previous agent ID to persist, got %q", m2.GetPreviousAgentID())
	}

	// Clear previous agent ID
	m2.ClearPreviousAgentID()
	if m2.GetPreviousAgentID() != "" {
		t.Error("expected empty previous agent ID after clear")
	}
}

func TestReset(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "certwatch.yaml")

	m := NewManager(configPath)
	m.SetAgentID("test-id")
	m.SetAgentName("test-name")
	m.SetPreviousAgentID("prev-id")

	if err := m.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Verify file exists
	statePath := filepath.Join(tmpDir, stateFileName)
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("state file should exist before reset")
	}

	// Reset
	if err := m.Reset(); err != nil {
		t.Fatalf("failed to reset: %v", err)
	}

	// Verify state is cleared
	if m.GetAgentID() != "" {
		t.Error("expected empty agent ID after reset")
	}
	if m.GetAgentName() != "" {
		t.Error("expected empty agent name after reset")
	}
	if m.GetPreviousAgentID() != "" {
		t.Error("expected empty previous agent ID after reset")
	}

	// Verify file is removed
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("state file should be removed after reset")
	}
}

func TestFilePath(t *testing.T) {
	configPath := "/home/user/config/certwatch.yaml"
	m := NewManager(configPath)

	expected := "/home/user/config/.certwatch-state.json"
	if m.FilePath() != expected {
		t.Errorf("expected FilePath() = %q, got %q", expected, m.FilePath())
	}
}

func TestStateFileFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "certwatch.yaml")

	m := NewManager(configPath)
	m.SetAgentID("uuid-1234")
	m.SetAgentName("prod-agent")
	m.SetPreviousAgentID("old-uuid-5678")
	m.SetLastSyncAt(time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC))

	if err := m.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Read raw file and verify JSON structure
	statePath := filepath.Join(tmpDir, stateFileName)
	//nolint:gosec // G304: Test file reading from test temp directory, path is safe
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse state file as JSON: %v", err)
	}

	// Verify expected fields
	if parsed["agent_id"] != "uuid-1234" {
		t.Errorf("expected agent_id in JSON, got %v", parsed["agent_id"])
	}
	if parsed["agent_name"] != "prod-agent" {
		t.Errorf("expected agent_name in JSON, got %v", parsed["agent_name"])
	}
	if parsed["previous_agent_id"] != "old-uuid-5678" {
		t.Errorf("expected previous_agent_id in JSON, got %v", parsed["previous_agent_id"])
	}
	if _, ok := parsed["last_updated"]; !ok {
		t.Error("expected last_updated in JSON")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "certwatch.yaml")

	m := NewManager(configPath)

	// Test concurrent reads and writes
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			m.SetAgentID("id")
			m.SetAgentName("name")
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = m.GetAgentID()
			_ = m.GetAgentName()
			_ = m.HasNameChanged("test")
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done

	// If we get here without deadlock or race, the test passes
}

// Package state handles agent state persistence to disk.
// This allows agent_id to survive restarts and enables detection of agent name changes.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State holds persisted agent state
type State struct {
	AgentID         string    `json:"agent_id"`
	AgentName       string    `json:"agent_name"`
	PreviousAgentID string    `json:"previous_agent_id,omitempty"` // For migration
	LastSyncAt      time.Time `json:"last_sync_at,omitempty"`
	LastUpdated     time.Time `json:"last_updated"`
}

// Manager handles state persistence
type Manager struct {
	filePath string
	state    *State
	mu       sync.RWMutex
}

// stateFileName is the name of the state file stored alongside config
const stateFileName = ".certwatch-state.json"

// DefaultStateDir is the default directory for state storage in containers
const DefaultStateDir = "/var/lib/certwatch"

// NewManager creates a state manager for the given config file path
// The state file will be stored in the same directory as the config file
func NewManager(configPath string) *Manager {
	dir := filepath.Dir(configPath)
	statePath := filepath.Join(dir, stateFileName)

	return &Manager{
		filePath: statePath,
		state:    &State{},
	}
}

// NewManagerWithStateDir creates a state manager with an explicit state directory
// This is useful when the config is in a read-only location (e.g., ConfigMap mount)
func NewManagerWithStateDir(stateDir string) *Manager {
	statePath := filepath.Join(stateDir, stateFileName)

	return &Manager{
		filePath: statePath,
		state:    &State{},
	}
}

// Load reads state from disk
// Returns nil if file doesn't exist (first run)
// Returns error if file exists but cannot be read/parsed
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// First run, no state file yet
			m.state = &State{}
			return nil
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	state := &State{}
	if err := json.Unmarshal(data, state); err != nil {
		// State file corrupted, treat as first run but log warning
		m.state = &State{}
		return fmt.Errorf("failed to parse state file (treating as first run): %w", err)
	}

	m.state = state
	return nil
}

// Save writes state to disk with secure permissions (0600)
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.LastUpdated = time.Now().UTC()

	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write with secure permissions (owner read/write only)
	if err := os.WriteFile(m.filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// GetAgentID returns the persisted agent ID
func (m *Manager) GetAgentID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.AgentID
}

// SetAgentID sets the agent ID (call Save() to persist)
func (m *Manager) SetAgentID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.AgentID = id
}

// ClearAgentID removes the agent ID (used when agent is deleted from server)
func (m *Manager) ClearAgentID() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.AgentID = ""
}

// GetAgentName returns the persisted agent name
func (m *Manager) GetAgentName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.AgentName
}

// SetAgentName sets the agent name (call Save() to persist)
func (m *Manager) SetAgentName(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.AgentName = name
}

// GetPreviousAgentID returns the previous agent ID (for migration)
func (m *Manager) GetPreviousAgentID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.PreviousAgentID
}

// SetPreviousAgentID sets the previous agent ID for migration purposes
func (m *Manager) SetPreviousAgentID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.PreviousAgentID = id
}

// ClearPreviousAgentID clears the previous agent ID after successful migration
func (m *Manager) ClearPreviousAgentID() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.PreviousAgentID = ""
}

// GetLastSyncAt returns the last sync timestamp
func (m *Manager) GetLastSyncAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.LastSyncAt
}

// SetLastSyncAt sets the last sync timestamp (call Save() to persist)
func (m *Manager) SetLastSyncAt(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.LastSyncAt = t
}

// HasNameChanged checks if the config name differs from the persisted name
// Returns false if no previous name is stored (first run)
func (m *Manager) HasNameChanged(configName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// First run - no previous name stored
	if m.state.AgentName == "" {
		return false
	}

	return m.state.AgentName != configName
}

// HasState returns true if there is persisted state (not first run)
func (m *Manager) HasState() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.AgentID != "" || m.state.AgentName != ""
}

// Reset clears all state
func (m *Manager) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state = &State{}

	// Remove state file if it exists
	if err := os.Remove(m.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state file: %w", err)
	}

	return nil
}

// FilePath returns the path to the state file
func (m *Manager) FilePath() string {
	return m.filePath
}

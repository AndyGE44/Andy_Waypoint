package checkpoint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func NewManager(baseDir string) *Manager {
	overlayDir := filepath.Join(baseDir, "overlays")
	criuDir := filepath.Join(baseDir, "criu")
	metadataDir := filepath.Join(baseDir, "metadata")
	workOverlay := filepath.Join(baseDir, "work")

	// Create directories
	os.MkdirAll(overlayDir, 0755)
	os.MkdirAll(criuDir, 0755)
	os.MkdirAll(metadataDir, 0755)
	os.MkdirAll(workOverlay, 0755)

	return &Manager{
		baseDir:     baseDir,
		overlayDir:  overlayDir,
		criuDir:     criuDir,
		metadataDir: metadataDir,
		workOverlay: workOverlay,
	}
}

// CreateCheckpoint creates both filesystem and memory checkpoint
func (m *Manager) CreateCheckpoint(pid int, checkpointID string) error {
	// Validate checkpoint ID
	if checkpointID == "" || checkpointID == "current" {
		return fmt.Errorf("invalid checkpoint ID: %s", checkpointID)
	}

	// Check if process exists
	if !m.processExists(pid) {
		return fmt.Errorf("process %d does not exist", pid)
	}

	// Create checkpoint directories
	overlayCkptPath := filepath.Join(m.overlayDir, checkpointID)
	criuCkptPath := filepath.Join(m.criuDir, checkpointID)

	os.MkdirAll(overlayCkptPath, 0755)
	os.MkdirAll(criuCkptPath, 0755)

	// 1. Create a memory checkpoint
	if err := m.createMemoryCheckpoint(pid, criuCkptPath); err != nil {
		return fmt.Errorf("memory checkpoint failed: %w", err)
	}

	// 2. Create a filesystem checkpoint
	if err := m.createFilesystemCheckpoint(overlayCkptPath); err != nil {
		return fmt.Errorf("filesystem checkpoint failed: %w", err)
	}

	// 3. Save metadata
	metadata := Metadata{
		ID:          checkpointID,
		PID:         pid,
		OverlayPath: overlayCkptPath,
		CriuPath:    criuCkptPath,
		Timestamp:   time.Now().Unix(),
		OriginalDir: m.originalDir,
		SessionID:   m.sessionID,
	}

	return m.saveMetadata(checkpointID, metadata)
}

// RestoreCheckpoint restores both filesystem and memory state
func (m *Manager) RestoreCheckpoint(checkpointID string) (int, error) {
	// Load metadata
	metadata, err := m.loadMetadata(checkpointID)
	if err != nil {
		return 0, fmt.Errorf("failed to load checkpoint metadata: %w", err)
	}

	// 1. Restore filesystem state
	if err := m.restoreFilesystemState(checkpointID); err != nil {
		return 0, fmt.Errorf("filesystem restore failed: %w", err)
	}

	// 2. Restore memory state using CRIU
	newPID, err := m.restoreMemoryState(metadata.PID, metadata.CriuPath)
	if err != nil {
		return 0, fmt.Errorf("memory restore failed: %w", err)
	}

	return newPID, nil
}

// ListCheckpoints returns list of available checkpoints
func (m *Manager) ListCheckpoints() ([]string, error) {
	files, err := os.ReadDir(m.metadataDir)
	if err != nil {
		return nil, err
	}

	var checkpoints []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") && file.Name() != "environment.json" {
			checkpointID := strings.TrimSuffix(file.Name(), ".json")
			checkpoints = append(checkpoints, checkpointID)
		}
	}

	return checkpoints, nil
}

// Cleanup removes all files and unmounts overlay for this session
func (m *Manager) Cleanup() error {
	// Unmount overlay
	if m.workOverlay != "" {
		cmd := exec.Command("umount", m.workOverlay)
		cmd.Run() // Ignore errors - might already be unmounted
	}

	// Remove session directory
	if err := os.RemoveAll(m.baseDir); err != nil {
		return fmt.Errorf("failed to remove session directory: %w", err)
	}

	// Remove global session info
	return removeSessionInfo(m.sessionID)
}

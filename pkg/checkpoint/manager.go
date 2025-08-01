package checkpoint

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Manager struct {
	baseDir     string
	overlayDir  string
	criuDir     string
	metadataDir string
	workOverlay string // Current working overlay mount point
	originalDir string // Original directory being managed
	sessionID   string // Unique session identifier
}

type Metadata struct {
	ID          string `json:"id"`
	PID         int    `json:"pid"`
	OverlayPath string `json:"overlay_path"`
	CriuPath    string `json:"criu_path"`
	Timestamp   int64  `json:"timestamp"`
	OriginalDir string `json:"original_dir"`
	SessionID   string `json:"session_id"`
}

type SessionInfo struct {
	SessionID   string `json:"session_id"`
	BaseDir     string `json:"base_dir"`
	OriginalDir string `json:"original_dir"`
	WorkOverlay string `json:"work_overlay"`
	CreatedAt   int64  `json:"created_at"`
}

// Generate a random session ID
func generateSessionID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// /tmp/
// ├── checkpoint-sessions/           # Individual session directories
// │   	├── a1b2c3d4e5f6g7h8/         # App A's session
// │   	│  	├── overlays/
// │   	│   ├── criu/
// │   	│   ├── metadata/
// │   	│   └── work/                  # App A works here
// │   	└── x9y8z7w6v5u4t3s2/         # App B's session
// │       	├── overlays/
// │       	├── criu/
// │       	├── metadata/
// │       	└── work/                  # App B works here
// └── checkpoint-sessions-info/      # Global session registry
// 		├── a1b2c3d4e5f6g7h8.json
// 		└── x9y8z7w6v5u4t3s2.json

// NewManagerWithSession creates a new manager with a random session ID
func NewManagerWithSession() (*Manager, string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Use /tmp/checkpoint-sessions as the root directory
	baseDir := filepath.Join("/tmp", "checkpoint-sessions", sessionID)
	manager := NewManager(baseDir)
	manager.sessionID = sessionID

	// Save session info globally
	if err := saveSessionInfo(sessionID, manager); err != nil {
		return nil, "", fmt.Errorf("failed to save session info: %w", err)
	}

	return manager, sessionID, nil
}

// LoadManager loads an existing manager by session ID
func LoadManager(sessionID string) (*Manager, error) {
	sessionInfo, err := loadSessionInfo(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	manager := NewManager(sessionInfo.BaseDir)
	manager.sessionID = sessionID
	manager.originalDir = sessionInfo.OriginalDir
	manager.workOverlay = sessionInfo.WorkOverlay

	return manager, nil
}

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

// InitEnvironment sets up OverlayFS for the given directory
func (m *Manager) InitEnvironment(originalDir string) (string, error) {
	// Convert to absolute path
	absDir, err := filepath.Abs(originalDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if the user-specified directory exists
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		return "", fmt.Errorf("directory does not exist: %s", absDir)
	}

	m.originalDir = absDir

	// Create overlay structure
	upperDir := filepath.Join(m.overlayDir, "current", "upper")
	workDir := filepath.Join(m.overlayDir, "current", "work")

	os.MkdirAll(upperDir, 0755)
	os.MkdirAll(workDir, 0755)

	// Mount overlay
	err = m.mountOverlay(absDir, upperDir, workDir, m.workOverlay)
	if err != nil {
		return "", fmt.Errorf("failed to mount overlay: %w", err)
	}

	// Update session info with environment details
	if err := updateSessionEnvironment(m.sessionID, absDir, m.workOverlay); err != nil {
		return "", fmt.Errorf("failed to update session info: %w", err)
	}

	return m.workOverlay, nil
}

// CreateCheckpoint creates both filesystem and memory checkpoint
func (m *Manager) CreateCheckpoint(pid int, checkpointID string) error {
	// Check if process exists
	if !m.processExists(pid) {
		return fmt.Errorf("process %d does not exist", pid)
	}

	// Create checkpoint directories
	overlayPath := filepath.Join(m.overlayDir, checkpointID)
	criuPath := filepath.Join(m.criuDir, checkpointID)

	os.MkdirAll(overlayPath, 0755)
	os.MkdirAll(criuPath, 0755)

	// 1. Create filesystem checkpoint (snapshot current overlay state)
	if err := m.createFilesystemCheckpoint(checkpointID, overlayPath); err != nil {
		return fmt.Errorf("filesystem checkpoint failed: %w", err)
	}

	// 2. Create memory checkpoint using CRIU
	if err := m.createMemoryCheckpoint(pid, criuPath); err != nil {
		return fmt.Errorf("memory checkpoint failed: %w", err)
	}

	// 3. Save metadata
	metadata := Metadata{
		ID:          checkpointID,
		PID:         pid,
		OverlayPath: overlayPath,
		CriuPath:    criuPath,
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

// Helper methods

func saveSessionInfo(sessionID string, manager *Manager) error {
	sessionsDir := "/tmp/checkpoint-sessions-info"
	os.MkdirAll(sessionsDir, 0755)

	sessionInfo := SessionInfo{
		SessionID: sessionID,
		BaseDir:   manager.baseDir,
		CreatedAt: time.Now().Unix(),
	}

	data, err := json.MarshalIndent(sessionInfo, "", "  ")
	if err != nil {
		return err
	}

	sessionFile := filepath.Join(sessionsDir, sessionID+".json")
	return os.WriteFile(sessionFile, data, 0644)
}

func loadSessionInfo(sessionID string) (*SessionInfo, error) {
	sessionsDir := "/tmp/checkpoint-sessions-info"
	sessionFile := filepath.Join(sessionsDir, sessionID+".json")

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	var sessionInfo SessionInfo
	err = json.Unmarshal(data, &sessionInfo)
	return &sessionInfo, err
}

func updateSessionEnvironment(sessionID, originalDir, workOverlay string) error {
	sessionInfo, err := loadSessionInfo(sessionID)
	if err != nil {
		return err
	}

	sessionInfo.OriginalDir = originalDir
	sessionInfo.WorkOverlay = workOverlay

	data, err := json.MarshalIndent(sessionInfo, "", "  ")
	if err != nil {
		return err
	}

	sessionsDir := "/tmp/checkpoint-sessions-info"
	sessionFile := filepath.Join(sessionsDir, sessionID+".json")
	return os.WriteFile(sessionFile, data, 0644)
}

func removeSessionInfo(sessionID string) error {
	sessionsDir := "/tmp/checkpoint-sessions-info"
	sessionFile := filepath.Join(sessionsDir, sessionID+".json")
	return os.Remove(sessionFile)
}

func (m *Manager) mountOverlay(lowerDir, upperDir, workDir, mountPoint string) error {
	// Unmount if already mounted
	exec.Command("umount", mountPoint).Run() // Ignore errors

	// Mount overlay
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	cmd := exec.Command("mount", "-t", "overlay", "overlay", "-o", options, mountPoint)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mount command failed: %w", err)
	}

	return nil
}

func (m *Manager) createFilesystemCheckpoint(checkpointID, overlayPath string) error {
	// Copy current upper directory to checkpoint
	currentUpper := filepath.Join(m.overlayDir, "current", "upper")
	checkpointUpper := filepath.Join(overlayPath, "upper")

	// TODO: Search about OverlayFS, we might need to copy both upper and work directories???
	checkpointWork := filepath.Join(overlayPath, "work")

	os.MkdirAll(checkpointUpper, 0755)
	os.MkdirAll(checkpointWork, 0755)

	// Use rsync to copy the upper directory
	cmd := exec.Command("rsync", "-a", currentUpper+"/", checkpointUpper+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy filesystem state: %w", err)
	}

	return nil
}

func (m *Manager) createMemoryCheckpoint(pid int, criuPath string) error {
	// Use CRIU to dump the process
	cmd := exec.Command("criu", "dump",
		"-t", fmt.Sprintf("%d", pid),
		"-D", criuPath,
		"--shell-job",
		"--tcp-established", // Include TCP connections
		"--leave-running")   // Keep process running after checkpoint

	// CRIU might need root privileges
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: 0, // This might need adjustment based on your setup
			Gid: 0,
		},
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("CRIU dump failed: %w, output: %s", err, string(output))
	}

	return nil
}

func (m *Manager) restoreFilesystemState(checkpointID string) error {
	// Restore filesystem by replacing current upper with checkpoint upper
	currentUpper := filepath.Join(m.overlayDir, "current", "upper")
	checkpointUpper := filepath.Join(m.overlayDir, checkpointID, "upper")

	// TODO: Search about OverlayFS, we might need to copy both upper and work directories, and possibly re-mount the overlay

	// Backup current state
	backupUpper := filepath.Join(m.overlayDir, "current", "upper.backup")
	os.RemoveAll(backupUpper)
	os.Rename(currentUpper, backupUpper)

	// Copy checkpoint state to current
	cmd := exec.Command("rsync", "-a", checkpointUpper+"/", currentUpper+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restore filesystem state: %w", err)
	}

	return nil
}

func (m *Manager) restoreMemoryState(pid int, criuPath string) (int, error) {
	// Kill the original process if it exists
	err := m.killProcess(pid)
	if err != nil {
		return -1, fmt.Errorf("failed to kill original process %d: %w", pid, err)
	}

	// Use CRIU to restore the process
	cmd := exec.Command("criu", "restore",
		"-D", criuPath,
		"--shell-job",
		"--tcp-established")

	// Set root privileges
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: 0,
			Gid: 0,
		},
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return -1, fmt.Errorf("CRIU restore failed: %w, output: %s", err, string(output))
	}

	return pid, nil
}

func (m *Manager) killProcess(pid int) error {
	// Mimic my "__kill_original_process"'s soft and hard kill behavior
	process, err := os.FindProcess(pid)
	if err != nil {
		// Process does not exist, probably already terminated
		return nil
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		// If graceful termination fails, try SIGKILL
		if err := process.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("failed to kill process %d: %w", pid, err)
		}
	}

	// Wait for process to terminate (up to 1 second)
	for i := 0; i < 10; i++ {
		if !m.processExists(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// If still running, force kill
	return process.Signal(syscall.SIGKILL)
}

func (m *Manager) processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func (m *Manager) saveMetadata(checkpointID string, metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	metadataPath := filepath.Join(m.metadataDir, checkpointID+".json")
	return os.WriteFile(metadataPath, data, 0644)
}

func (m *Manager) loadMetadata(checkpointID string) (*Metadata, error) {
	metadataPath := filepath.Join(m.metadataDir, checkpointID+".json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata Metadata
	err = json.Unmarshal(data, &metadata)
	return &metadata, err
}

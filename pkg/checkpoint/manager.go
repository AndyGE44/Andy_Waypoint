package checkpoint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func NewManager(baseDir string) *Manager {
	metadataDir := filepath.Join(baseDir, "metadata")
	workOverlay := filepath.Join(baseDir, "work")

	// Create directories
	os.MkdirAll(metadataDir, 0755)
	os.MkdirAll(workOverlay, 0755)

	return &Manager{
		baseDir:     baseDir,
		metadataDir: metadataDir,
		workOverlay: workOverlay,
	}
}

// WorkOverlay returns the work overlay directory for this manager
func (m *Manager) WorkOverlay() string {
	return m.workOverlay
}

// SandboxMode returns whether sandbox mode is enabled for this manager
func (m *Manager) SandboxMode() bool {
	return m.sandboxMode
}

// ExecuteCommand executes a command in the checkpoint environment.
// If sandbox mode is enabled, the command runs in an isolated sandbox.
// Otherwise, it runs directly in the work overlay directory.
func (m *Manager) ExecuteCommand(command string, args ...string) (*exec.Cmd, error) {
	if m.SandboxMode() {
		// Use sandbox isolation - pass originalDir so commands start there
		return ExecuteInSandbox(m.workOverlay, m.originalDir, command, args...)
	} else {
		// Execute directly in work overlay (which contains the original directory's content)
		cmd := exec.Command(command, args...)
		cmd.Dir = m.workOverlay
		return cmd, nil
	}
}

// CreateCheckpoint creates both the filesystem and the memory checkpoint
// Deprecated: Since version 0.2.0, use CreateCheckpointParallel instead

// CreateCheckpointParallel creates both the filesystem and memory checkpoints in parallel
// This function uses goroutines to speed up the checkpoint creation process (x0.65)
// func (m *Manager) CreateCheckpointParallel(pid int, checkpointID string) error {
// 	// Validate checkpoint ID
// 	if checkpointID == "" || checkpointID == "current" {
// 		return fmt.Errorf("invalid checkpoint ID: %s", checkpointID)
// 	}

// 	// Check if process exists
// 	if pid == SkipMemoryCheckpoint {
// 		fmt.Println("Skipping memory checkpoint as per user request")
// 	} else if !m.processExists(pid) {
// 		return fmt.Errorf("process %d does not exist", pid)
// 	}

// 	// Create checkpoint directories
// 	overlayCkptPath := filepath.Join(m.overlayDir, checkpointID)
// 	criuCkptPath := filepath.Join(m.criuDir, checkpointID)

// 	os.MkdirAll(overlayCkptPath, 0755)
// 	os.MkdirAll(criuCkptPath, 0755)

// 	var wg sync.WaitGroup
// 	var filesystemErr, memoryErr error

// 	wg.Add(2)

// 	// 1. Create a memory checkpoint
// 	go func() {
// 		defer wg.Done()
// 		if pid == SkipMemoryCheckpoint {
// 			memoryErr = nil
// 			return
// 		}
// 		memoryErr = m.createMemoryCheckpoint(pid, criuCkptPath)
// 	}()

// 	// 2. Create a filesystem checkpoint
// 	go func() {
// 		defer wg.Done()
// 		filesystemErr = m.createFilesystemCheckpoint(overlayCkptPath)
// 	}()

// 	// Wait for both goroutines to finish
// 	wg.Wait()

// 	// Check for errors
// 	if memoryErr != nil {
// 		return fmt.Errorf("memory checkpoint failed: %w", memoryErr)
// 	}
// 	if filesystemErr != nil {
// 		return fmt.Errorf("filesystem checkpoint failed: %w", filesystemErr)
// 	}

// 	// 3. Save metadata
// 	metadata := Metadata{
// 		ID:          checkpointID,
// 		PID:         pid,
// 		OverlayPath: overlayCkptPath,
// 		CriuPath:    criuCkptPath,
// 		Timestamp:   time.Now().Unix(),
// 		OriginalDir: m.originalDir,
// 		SessionID:   m.sessionID,
// 	}

// 	return m.saveMetadata(checkpointID, metadata)
// }

func (m *Manager) CreateCheckpointNew(pid int, checkpointID string) error {
	// Validate checkpoint ID
	if checkpointID == "" || checkpointID == "current" {
		return fmt.Errorf("invalid checkpoint ID: %s", checkpointID)
	}

	// Create a memory checkpoint to "~/current/criu/*.img"
	if pid == SkipMemoryCheckpoint {
		fmt.Println("Skipping memory checkpoint as per user request")
	} else if !m.processExists(pid) {
		return fmt.Errorf("process %d does not exist", pid)
	} else {
		currentCriuDir := filepath.Join(m.baseDir, "current", "criu")
		memoryErr := m.createMemoryCheckpoint(pid, currentCriuDir)
		if memoryErr != nil {
			return fmt.Errorf("memory checkpoint failed: %w", memoryErr)
		}
	}

	// Unmount current overlay to ensure filesystem consistency
	exec.Command("umount", m.workOverlay).Run()

	// Rename "~/current/" to "~/<checkpointID>/"
	currentDir := filepath.Join(m.baseDir, "current")
	ckptDir := filepath.Join(m.baseDir, checkpointID)
	if err := os.Rename(currentDir, ckptDir); err != nil {
		return fmt.Errorf("failed to rename current directory: %w", err)
	}

	// Recreate a new empty "current" overlay for continued use
	os.MkdirAll(currentDir, 0755)
	upperDir := filepath.Join(m.baseDir, "current", "upper")
	workDir := filepath.Join(m.baseDir, "current", "work")
	os.MkdirAll(upperDir, 0755)
	os.MkdirAll(workDir, 0755)

	// Update current parent list to include this new checkpoint
	parentList := m.currentParent
	parentList = append(parentList, checkpointID)
	m.currentParent = parentList

	// Remount the new "current" overlay with mutliple lowerdirs
	lowerDirs := []string{m.originalDir}
	for _, parentID := range parentList {
		parentOverlay := filepath.Join(m.baseDir, parentID, "upper")
		lowerDirs = append(lowerDirs, parentOverlay)
	}
	err := m.mountOverlay(lowerDirs, upperDir, workDir, m.workOverlay)
	if err != nil {
		return fmt.Errorf("failed to remount new current overlay: %w", err)
	}

	// Save metadata
	metadata := Metadata{
		ID:          checkpointID,
		PID:         pid,
		Timestamp:   time.Now().Unix(),
		OriginalDir: m.originalDir,
		SessionID:   m.sessionID,
		ParentList:  parentList,
	}

	return m.saveMetadata(checkpointID, metadata)
}

// RestoreCheckpoint restores both the filesystem and memory state
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
	if metadata.PID == SkipMemoryCheckpoint {
		fmt.Println("Skipping memory restore as per user request")
		return SkipMemoryCheckpoint, nil
	}
	newPID, err := m.restoreMemoryState(metadata.PID, metadata.CriuPath)
	if err != nil {
		return 0, fmt.Errorf("memory restore failed: %w", err)
	}

	return newPID, nil
}

// ListCheckpoints returns a list of available checkpoints
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

// Cleanup removes all files and unmounts the overlay for this session
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

// CleanupForce removes all files and unmounts the overlay for this session
func (m *Manager) CleanupForce() error {
	fmt.Printf("Starting forceful cleanup for session %s...\n", m.sessionID)

	// Step 1: Unmount overlay filesystems
	fmt.Println("Unmounting overlay filesystems...")
	if err := m.forceUnmountOverlays(); err != nil {
		fmt.Printf("Warning: Failed to unmount overlays: %v\n", err)
	}

	// Step 2: Kill processes using files in this directory
	fmt.Println("Killing processes using session directory...")
	if err := m.killProcessesUsingDirectory(); err != nil {
		fmt.Printf("Warning: Failed to kill some processes: %v\n", err)
	}

	// Step 3: Close file handles
	fmt.Println("Closing file handles...")
	if err := m.closeFileHandles(); err != nil {
		fmt.Printf("Warning: Failed to close some file handles: %v\n", err)
	}

	// Step 4: Force unmount any remaining mounts
	fmt.Println("Force unmounting all mounts in session directory...")
	if err := m.forceUnmountAll(); err != nil {
		fmt.Printf("Warning: Failed to force unmount: %v\n", err)
	}

	// Step 5: Try removing the directory multiple times with a backoff
	fmt.Println("Removing session directory...")
	if err := m.removeDirectoryWithRetry(); err != nil {
		return fmt.Errorf("failed to remove session directory after multiple attempts: %w", err)
	}

	// Step 6: Remove global session info
	fmt.Println("Removing session info...")
	if err := removeSessionInfo(m.sessionID); err != nil {
		fmt.Printf("Warning: Failed to remove session info: %v\n", err)
	}

	return nil
}

// CleanupInteractive cleanup with user interaction
func (m *Manager) CleanupInteractive() error {
	// Try automatic cleanup first
	err := m.Cleanup()
	if err == nil {
		return nil
	}

	fmt.Printf("Automatic cleanup failed: %v\n", err)
	fmt.Println("This usually happens when processes are still using files in the session directory.")
	fmt.Println("\nTroubleshooting hints:")

	// Show processes using the directory
	fmt.Printf("Processes using session directory:\n")
	pids, _ := m.findProcessesUsingDirectory()
	if len(pids) > 0 {
		for _, pid := range pids {
			cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "pid,ppid,cmd")
			output, _ := cmd.Output()
			fmt.Print(string(output))
		}
	} else {
		fmt.Print("  No processes found")
	}
	fmt.Println()

	// Show mount points
	fmt.Printf("Active mount points:\n")
	mounts, _ := m.findMountsInDirectory()
	if len(mounts) > 0 {
		for _, mount := range mounts {
			fmt.Printf("  %s\n", mount)
		}
	} else {
		fmt.Print("  No mounts found")
	}
	fmt.Println()

	fmt.Println("\nRecommended actions:")
	fmt.Println("1. Close any terminals/editors in the session directory")
	fmt.Println("2. Deactivate Python virtual environments, Docker containers, etc.")
	fmt.Println("3. Stop any processes listed above")
	fmt.Println("4. Unmount any mounts listed above (e.g., using 'sudo umount <mountpoint>')")

	return fmt.Errorf("manual intervention required")
}

package checkpoint

// All filesystem-related operations

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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

func (m *Manager) mountOverlay(lowerDir, upperDir, workDir, mountPoint string) error {
	// Unmount if already mounted
	exec.Command("umount", mountPoint).Run()

	// Mount overlay
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	cmd := exec.Command("mount", "-t", "overlay", "overlay", "-o", options, mountPoint)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mount command failed: %w", err)
	}

	return nil
}

func (m *Manager) createFilesystemCheckpoint(overlayCkptPath string) error {
	currentUpper := filepath.Join(m.overlayDir, "current", "upper")
	currentWork := filepath.Join(m.overlayDir, "current", "work")

	// Create checkpoint directories
	checkpointUpper := filepath.Join(overlayCkptPath, "upper")
	checkpointWork := filepath.Join(overlayCkptPath, "work")
	os.MkdirAll(checkpointUpper, 0755)
	os.MkdirAll(checkpointWork, 0755)

	// Copy current upper and work directories to checkpoint
	// Use rsync to preserve permissions and attributes
	cmd := exec.Command("rsync", "-a", currentUpper+"/", checkpointUpper+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy filesystem state: %w", err)
	}

	cmd = exec.Command("rsync", "-a", currentWork+"/", checkpointWork+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy work directory: %w", err)
	}

	return nil
}

// forceUnmountOverlays unmounts all overlay filesystems in the session
func (m *Manager) forceUnmountOverlays() error {
	// Unmount the main work overlay
	if m.workOverlay != "" {
		if err := m.forceUnmount(m.workOverlay); err != nil {
			return fmt.Errorf("failed to unmount work overlay: %w", err)
		}
	}

	// Find and unmount any other overlay mounts in our directory
	mounts, err := m.findMountsInDirectory()
	if err != nil {
		return err
	}

	for _, mount := range mounts {
		if err := m.forceUnmount(mount); err != nil {
			fmt.Printf("Warning: Failed to unmount %s: %v\n", mount, err)
		}
	}

	return nil
}

// forceUnmount attempts to unmount with increasing force
func (m *Manager) forceUnmount(mountPoint string) error {
	// Try normal unmount first
	cmd := exec.Command("umount", mountPoint)
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Try lazy unmount
	cmd = exec.Command("umount", "-l", mountPoint)
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Try force unmount
	cmd = exec.Command("umount", "-f", mountPoint)
	return cmd.Run()
}

// findMountsInDirectory finds all mount points within our session directory
func (m *Manager) findMountsInDirectory() ([]string, error) {
	var mounts []string

	file, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			mountPoint := fields[1]
			if strings.HasPrefix(mountPoint, m.baseDir) {
				mounts = append(mounts, mountPoint)
			}
		}
	}

	return mounts, scanner.Err()
}

// forceUnmountAll uses umount to unmount everything in our directory tree
func (m *Manager) forceUnmountAll() error {
	// Find all mount points and force unmount them
	cmd := exec.Command("findmnt", "-n", "-o", "TARGET", "-M", m.baseDir)
	output, err := cmd.Output()
	if err != nil {
		return nil // No mounts found
	}

	mounts := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, mount := range mounts {
		if mount != "" {
			exec.Command("umount", "-f", "-l", mount).Run()
		}
	}

	return nil
}

// removeDirectoryWithRetry attempts to remove the base directory with exponential backoff
func (m *Manager) removeDirectoryWithRetry() error {
	maxAttempts := 5
	baseDelay := 500 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := os.RemoveAll(m.baseDir)
		if err == nil {
			return nil
		}

		if attempt == maxAttempts {
			return fmt.Errorf("final attempt failed: %w", err)
		}

		fmt.Printf("Attempt %d failed (%v), retrying in %v...\n",
			attempt, err, baseDelay)

		time.Sleep(baseDelay)
		baseDelay *= 2 // Exponential backoff
	}

	return nil
}

func (m *Manager) restoreFilesystemState(checkpointID string) error {
	// Unmount current overlay
	exec.Command("umount", m.workOverlay).Run()

	// Restore filesystem by replacing the current layers with the checkpoint layers
	currentUpper := filepath.Join(m.overlayDir, "current", "upper")
	checkpointUpper := filepath.Join(m.overlayDir, checkpointID, "upper")
	currentWork := filepath.Join(m.overlayDir, "current", "work")
	checkpointWork := filepath.Join(m.overlayDir, checkpointID, "work")

	// Backup current state
	backupUpper := filepath.Join(m.overlayDir, "current", "upper.backup")
	os.RemoveAll(backupUpper)
	os.Rename(currentUpper, backupUpper)
	backupWork := filepath.Join(m.overlayDir, "current", "work.backup")
	os.RemoveAll(backupWork)
	os.Rename(currentWork, backupWork)

	// Copy checkpoint state to current
	cmd := exec.Command("rsync", "-a", checkpointUpper+"/", currentUpper+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restore filesystem state: %w", err)
	}
	cmd = exec.Command("rsync", "-a", checkpointWork+"/", currentWork+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restore work directory: %w", err)
	}

	// Remount overlay with restored state
	if err := m.mountOverlay(m.originalDir, currentUpper, currentWork, m.workOverlay); err != nil {
		// Restore backup if mount fails
		os.Rename(backupUpper, currentUpper)
		os.Rename(backupWork, currentWork)
		return fmt.Errorf("failed to remount overlay after restore: %w", err)
	}

	return nil
}

// restoreFilesystemStateQuick restores the filesystem state quickly by replacing the current layers with the checkpoint layers
// This is an experimental method and might be unsafe if the current state is not clean
// ... seems buggy, maybe not needed
func (m *Manager) restoreFilesystemStateQuick(checkpointID string) error {
	// Restore filesystem by replacing the current layers with the checkpoint layers
	currentUpper := filepath.Join(m.overlayDir, "current", "upper")
	checkpointUpper := filepath.Join(m.overlayDir, checkpointID, "upper")
	currentWork := filepath.Join(m.overlayDir, "current", "work")
	checkpointWork := filepath.Join(m.overlayDir, checkpointID, "work")

	// Backup current state
	backupUpper := filepath.Join(m.overlayDir, "current", "upper.backup")
	os.RemoveAll(backupUpper)
	os.Rename(currentUpper, backupUpper)
	backupWork := filepath.Join(m.overlayDir, "current", "work.backup")
	os.RemoveAll(backupWork)
	os.Rename(currentWork, backupWork)

	// Copy checkpoint state to current
	cmd := exec.Command("rsync", "-a", checkpointUpper+"/", currentUpper+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restore filesystem state: %w", err)
	}
	cmd = exec.Command("rsync", "-a", checkpointWork+"/", currentWork+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restore work directory: %w", err)
	}

	return nil
}

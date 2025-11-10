package checkpoint

// All CRIU-related operations

import (
	"fmt"
	"os/exec"
	"syscall"
)

func (m *Manager) createMemoryCheckpoint(pid int, criuPath string) error {
	// Use CRIU to dump the process
	cmd := exec.Command("criu", "dump",
		"-t", fmt.Sprintf("%d", pid),
		"-D", criuPath,
		"--shell-job",
		"--tcp-established", // Include TCP connections
		"--leave-running")   // Keep process running after checkpoint

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: 0,
			Gid: 0,
		},
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create memory checkpoint: %w", err)
	}

	return nil
}

func (m *Manager) restoreMemoryState(pid int, criuPath string) (int, error) {
	// Kill the original process if it exists
	err := m.killProcess(pid)
	if err != nil {
		return -1, fmt.Errorf("failed to kill original process %d: %w", pid, err)
	}

	var cmd *exec.Cmd

	// Check if sandboxing is enabled
	if m.SandboxMode() {
		cmd, err = RestoreInSandbox(criuPath, m.workOverlay, nil)
		if err != nil {
			return -1, fmt.Errorf("failed to setup sandbox for restore: %w", err)
		}
	} else {
		// Default behavior: no sandboxing
		criuCmd := fmt.Sprintf(
			"criu restore --images-dir '%s' --shell-job --tcp-established",
			criuPath,
		)

		cmd = exec.Command("script", "-q", "-c", criuCmd, "/dev/null")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: 0,
				Gid: 0,
			},
		}
	}

	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("failed to restore memory state: %w", err)
	}

	return pid, nil
}

package checkpoint

// Process management utilities

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (m *Manager) processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func (m *Manager) killProcess(pid int) error {
	// Mimic my "__kill_original_process"'s soft and hard kill behavior
	if !m.processExists(pid) {
		// Process does not exist, probably already terminated
		return nil
	}

	// Retrieve the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to retrieve process %d: %w", pid, err)
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

// killProcessesUsingDirectory kills processes that have files open in our directory
func (m *Manager) killProcessesUsingDirectory() error {
	pids, err := m.findProcessesUsingDirectory()
	if err != nil {
		return err
	}

	if len(pids) == 0 {
		return nil
	}

	fmt.Printf("Found %d processes using directory, attempting to terminate...\n", len(pids))

	// Try SIGTERM first
	for _, pid := range pids {
		if err := m.killProcess(pid); err != nil {
			fmt.Printf("Warning: Failed to kill process %d: %v\n", pid, err)
		}
	}

	// Wait a moment for processes to terminate
	time.Sleep(2 * time.Second)

	// Check if any processes are still alive and force kill them
	remainingPids, _ := m.findProcessesUsingDirectory()
	for _, pid := range remainingPids {
		process, err := os.FindProcess(pid)
		if err == nil {
			process.Signal(syscall.SIGKILL)
		}
	}

	return nil
}

// findProcessesUsingDirectory uses lsof to find processes with open files in directory
func (m *Manager) findProcessesUsingDirectory() ([]int, error) {
	// Use lsof to find processes with open files in our directory
	cmd := exec.Command("lsof", "+D", m.baseDir)
	output, err := cmd.Output()
	if err != nil {
		// lsof returns non-zero exit code if no files found, which is not an error
		return []int{}, nil
	}

	var pids []int
	lines := strings.Split(string(output), "\n")

	// Skip header line, parse PIDs from lsof output
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if pid, err := strconv.Atoi(fields[1]); err == nil {
				// Avoid duplicates
				found := false
				for _, existingPid := range pids {
					if existingPid == pid {
						found = true
						break
					}
				}
				if !found {
					pids = append(pids, pid)
				}
			}
		}
	}

	return pids, nil
}

// closeFileHandles attempts to close file handles using fuser
func (m *Manager) closeFileHandles() error {
	cmd := exec.Command("fuser", "-k", m.baseDir)
	cmd.Run()

	return nil
}

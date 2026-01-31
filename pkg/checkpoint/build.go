package checkpoint

// Dockerfile-based build process

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func BuildFromDockerfile(dockerfileDir, workspaceDir string) error {
	imageTag := fmt.Sprintf("ckptlite_%s:%d", filepath.Base(dockerfileDir), time.Now().Unix())

	run := func(cmd *exec.Cmd, capture bool) (string, error) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if capture {
			cmd.Stdout = &stdout
		} else {
			cmd.Stdout = os.Stdout
		}
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return stdout.String(),
				fmt.Errorf("command failed: %s\nstderr: %s",
					strings.Join(cmd.Args, " "),
					stderr.String())
		}
		return strings.TrimSpace(stdout.String()), nil
	}

	// 1. buildah bud
	if _, err := run(exec.Command(
		"buildah", "bud", "-t", imageTag, "-f", filepath.Join(dockerfileDir, "Dockerfile"), dockerfileDir,
	), false); err != nil {
		return err
	}

	// 2. buildah from -q
	cid, err := run(exec.Command("buildah", "from", "-q", imageTag), true)
	if err != nil {
		return err
	}
	if cid == "" {
		return fmt.Errorf("buildah from did not return a container id")
	}

	// Ensure cleanup
	defer func() {
		_, _ = run(exec.Command("buildah", "unmount", cid), false)
		_, _ = run(exec.Command("buildah", "rm", cid), false)
	}()

	// 3. buildah mount
	rootfs, err := run(exec.Command("buildah", "mount", cid), true)
	if err != nil {
		return err
	}
	if rootfs == "" {
		return fmt.Errorf("buildah mount did not return rootfs path")
	}

	// 4. Clean workspace
	if err := os.RemoveAll(workspaceDir); err != nil {
		return err
	}
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return err
	}

	// 5. Copy rootfs -> workspace
	if _, err := run(exec.Command(
		"rsync", "-a",
		rootfs+"/",
		workspaceDir,
	), false); err != nil {
		if _, err := run(exec.Command(
			"bash", "-lc",
			fmt.Sprintf("cp -a '%s/.' '%s'", rootfs, workspaceDir),
		), false); err != nil {
			return fmt.Errorf("failed to copy rootfs: %w", err)
		}
	}

	return nil
}

// bindMount performs a bind mount
func bindMount(source, target string) error {
	flags := syscall.MS_BIND
	return syscall.Mount(source, target, "", uintptr(flags), "")
}

// mountDevPts mounts devpts inside the environment
func mountDevPts(originalDir string) error {
	target := fmt.Sprintf("%s/dev/pts", originalDir)

	flags := uintptr(0)
	options := "newinstance,ptmxmode=0666"

	if err := syscall.Mount("devpts", target, "devpts", flags, options); err != nil {
		return fmt.Errorf("failed to mount devpts: %w", err)
	}

	return nil
}

func MountNecessary(originalDir string) error {
	// Bind mount /dev for PTY management
	devTarget := fmt.Sprintf("%s/dev", originalDir)
	if err := bindMount("/dev", devTarget); err != nil {
		return fmt.Errorf("failed to bind mount /dev: %w", err)
	}

	// Mount devpts for PTYs
	if err := mountDevPts(originalDir); err != nil {
		return fmt.Errorf("failed to mount devpts: %w", err)
	}

	// Mount /tmp for socket sharing
	// TODO: maybe sub-dir of /tmp for security?
	tmpTarget := fmt.Sprintf("%s/tmp", originalDir)
	if err := bindMount("/tmp", tmpTarget); err != nil {
		return fmt.Errorf("failed to bind mount /tmp: %w", err)
	}

	return nil
}

func (m *Manager) BuildEnvironment(dockerfileDir string) (string, int, error) {
	originalDir := filepath.Join(m.baseDir, "original")

	// Ensure originalDir is clean
	if err := os.RemoveAll(originalDir); err != nil {
		return "", 0, fmt.Errorf("failed to clean original directory: %w", err)
	}
	if err := os.MkdirAll(originalDir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create original directory: %w", err)
	}

	// Build from Dockerfile to create a virtual system environment
	buildahErr := BuildFromDockerfile(dockerfileDir, originalDir)
	if buildahErr != nil {
		return "", 0, fmt.Errorf("failed to build from Dockerfile: %w", buildahErr)
	}

	m.originalDir = originalDir

	// Mount necessary filesystems
	mountErr := MountNecessary(originalDir)
	if mountErr != nil {
		return "", 0, fmt.Errorf("failed to mount necessary filesystems: %w", mountErr)
	}

	// Copy bash_init binary into the environment root for later chroot execs
	bashInitSrc := "./bash_init" // TODO: Read from config or embed
	bashInitDst := filepath.Join(originalDir, "bash_init")
	bashData, readErr := os.ReadFile(bashInitSrc)
	if readErr != nil {
		return "", 0, fmt.Errorf("failed to read bash_init binary: %w", readErr)
	}
	if writeErr := os.WriteFile(bashInitDst, bashData, 0755); writeErr != nil {
		return "", 0, fmt.Errorf("failed to write bash_init binary: %w", writeErr)
	}

	// Now that we have a built environment ready.

	// Initialize overlay environment on top of it
	workDir, overlayErr := m.InitEnvironment(originalDir)
	if overlayErr != nil {
		return "", 0, fmt.Errorf("failed to initialize overlay environment: %w", overlayErr)
	}

	// Launch bash_init with chroot in background to set up the environment
	// TODO: Save PID and socketPath for later use
	socketPath := filepath.Join("/tmp", fmt.Sprintf("ckptlite_%s.sock", m.sessionID))
	cmd := exec.Command("chroot", workDir, "./bash_init", socketPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // new session = no controlling TTY
	}
	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("failed to start bash_init in chroot: %w", err)
	}

	// Update session info with originalDir and workOverlay
	if err := updateSessionEnvironment(m.sessionID, m.originalDir, m.workOverlay); err != nil {
		return "", 0, fmt.Errorf("failed to update session info: %w", err)
	}

	return workDir, cmd.Process.Pid, nil
}

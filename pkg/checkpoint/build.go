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
		fmt.Println("Running >> ", strings.Join(cmd.Args, " ")) // Debug print
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

func (m *Manager) mountSpecialFS() error {
	mergedDir := m.workOverlay

	mntDirs := []struct {
		src    string
		dst    string
		fstype string
		flags  uintptr
		data   string
	}{
		{"/dev", filepath.Join(mergedDir, "dev"), "", syscall.MS_BIND | syscall.MS_REC, ""},
		{"devpts", filepath.Join(mergedDir, "dev/pts"), "devpts", 0, "newinstance,ptmxmode=0666"},
		{"/tmp", filepath.Join(mergedDir, "tmp"), "", syscall.MS_BIND | syscall.MS_REC, ""},
	}

	for _, mnt := range mntDirs {
		os.MkdirAll(mnt.dst, 0755)
		if err := syscall.Mount(mnt.src, mnt.dst, mnt.fstype, mnt.flags, mnt.data); err != nil {
			return fmt.Errorf("mount %s failed: %w", mnt.dst, err)
		}
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

	// Mount special filesystems inside the overlay
	if mountErr := m.mountSpecialFS(); mountErr != nil {
		return "", 0, fmt.Errorf("failed to mount special filesystems: %w", mountErr)
	}

	// Launch bash_init with chroot in background to set up the environment
	// TODO: Save PID and socketPath for later use
	socketPath := filepath.Join("/tmp", fmt.Sprintf("ckptlite_%s.sock", m.sessionID))
	cmd := exec.Command("chroot", workDir, "./bash_init", socketPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // new session = no controlling TTY
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("failed to start bash_init in chroot: %w", err)
	}

	// Update session info with originalDir and workOverlay
	if err := updateSessionEnvironment(m.sessionID, m.originalDir, m.workOverlay); err != nil {
		return "", 0, fmt.Errorf("failed to update session info: %w", err)
	}

	return workDir, cmd.Process.Pid, nil
}

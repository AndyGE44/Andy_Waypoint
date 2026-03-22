package main

// pty-rpc-shell: A RPC shell implementation that turns an interactive process into a request–response execution service.
//
// GitHub Repository: https://github.com/Alex-XJK/pty-rpc-shell.git
// Designed and developed by Alex Jiakai Xu (https://alex-xjk.github.io/), DAPLab @ Columbia University (https://daplab.cs.columbia.edu/)

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Compile regexes once at package level for performance
var (
	ansiEscapeRegex  = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	oscSequenceRegex = regexp.MustCompile(`\x1b\][^\x07]*\x07`)
	otherEscapeRegex = regexp.MustCompile(`\x1b[>=]`)
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: bash_init <socket-path> <chroot-dir>")
		fmt.Println("Example: bash_init /tmp/bash_cmd.sock /checkpoint-sessions/xyz/work")
		os.Exit(1)
	}

	socketPath := os.Args[1]
	chrootDir := os.Args[2]

	// Ensure chroot directory exists
	if _, err := os.Stat(chrootDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Chroot directory does not exist: %s\n", chrootDir)
		os.Exit(1)
	}

	// Create PTY
	ptyMaster, ptySlave, err := pty.Open()
	if err != nil {
		panic(err)
	}

	// Create Unix domain socket for command communication
	os.Remove(socketPath) // Clean up old socket

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	// Start bash with PTY
	cmd := exec.Command(
		"/bin/bash",
		"--norc",
		"--noprofile",
		"--noediting",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: chrootDir,
		Setsid: true,
	}
	cmd.Dir = "/"
	cmd.Stdin = ptySlave
	cmd.Stdout = ptySlave
	cmd.Stderr = ptySlave

	if err := cmd.Start(); err != nil {
		panic(err)
	}

	bashPID := cmd.Process.Pid

	fmt.Println("Server pid:", os.Getpid())
	fmt.Println("Bash pid:", bashPID)
	fmt.Println("Socket path:", socketPath)
	fmt.Println("Ready to receive commands from Unix Domain Socket...")

	// Mutex to protect PTY reads/writes
	var ptyMutex sync.Mutex

	// Start a goroutine to continuously drain PTY output into a buffer
	outputBuffer := &syncBuffer{buf: &bytes.Buffer{}}
	go drainPTY(ptyMaster, outputBuffer)

	// Handle command connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		go handleClient(conn, ptyMaster, &ptyMutex, outputBuffer, bashPID)
	}
}

// syncBuffer is a thread-safe buffer
type syncBuffer struct {
	buf *bytes.Buffer
	mu  sync.Mutex
}

func (sb *syncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) ReadAndClear() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	data := sb.buf.String()
	sb.buf.Reset()
	return data
}

// drainPTY continuously reads from PTY and writes to buffer
func drainPTY(ptyMaster *os.File, outputBuffer *syncBuffer) {
	buf := make([]byte, 4096)
	for {
		n, err := ptyMaster.Read(buf)
		if n > 0 {
			outputBuffer.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// buildWrappedMarkerRegex matches a marker even if PTY line wrapping inserts '\n' inside it.
func buildWrappedMarkerRegex(marker string) *regexp.Regexp {
	parts := make([]string, 0, len(marker))
	for _, r := range marker {
		parts = append(parts, regexp.QuoteMeta(string(r)))
	}
	return regexp.MustCompile(strings.Join(parts, `\n*`))
}

func handleClient(conn net.Conn, ptyMaster *os.File, ptyMutex *sync.Mutex, outputBuffer *syncBuffer, bashPID int) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Read one command from client
	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		return
	}
	trim_line := strings.TrimSpace(line)

	ptyMutex.Lock()
	defer ptyMutex.Unlock()

	// Clear any stale output before sending command
	staleOutput := outputBuffer.ReadAndClear()
	if staleOutput != "" {
		fmt.Println("Cleanup >> ===== Start =====")
		fmt.Printf("%s\n", staleOutput)
		fmt.Println("Cleanup >> ===== End =====")
	}

	// Generate a unique marker for this command.
	markerNonce := time.Now().UnixNano()
	marker := fmt.Sprintf("__CMD_DONE_%d_%d__", bashPID, markerNonce)
	markerRegex := buildWrappedMarkerRegex(marker)

	fmt.Println("Recv >> ===== Start =====")
	fmt.Println(trim_line)
	fmt.Println("Recv >> ===== End =====")

	// Write command to PTY with a marker whose exact final value does NOT appear in the echoed input.
	cmdWithMarker := trim_line + fmt.Sprintf("; builtin printf '\\n%%s\\n' \"__CMD_DONE_$$_%d__\"\n", markerNonce)
	_, err = ptyMaster.WriteString(cmdWithMarker)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		return
	}

	// Wait for output with timeout
	timeout := time.After(60000 * time.Second)
	checkInterval := 10 * time.Millisecond
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	var allOutput strings.Builder

	for {
		select {
		case <-timeout:
			// Timeout - send what we have
			finalOutput := cleanOutput(allOutput.String(), cmdWithMarker, markerRegex)
			writer.WriteString(finalOutput)
			writer.Flush()
			return

		case <-ticker.C:
			// Check if we have output with the marker
			output := outputBuffer.ReadAndClear()
			if output != "" {
				allOutput.WriteString(output)

				normalized := stripControlChars(allOutput.String())
				if markerRegex.MatchString(normalized) {
					// Found actual marker - clean up output and send
					finalOutput := cleanOutput(allOutput.String(), cmdWithMarker, markerRegex)
					writer.WriteString(finalOutput)
					writer.Flush()
					return
				}
			}
		}
	}
}

// stripControlChars removes ANSI escape sequences - optimized version
func stripControlChars(s string) string {
	// Remove ANSI escape sequences
	s = ansiEscapeRegex.ReplaceAllString(s, "")
	s = oscSequenceRegex.ReplaceAllString(s, "")
	s = otherEscapeRegex.ReplaceAllString(s, "")

	// Normalize line endings
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	return s
}

// cleanOutput removes command echo and marker, leaving only actual output
func cleanOutput(raw, cmdSent string, markerRegex *regexp.Regexp) string {
	fmt.Println("Raw >> ===== Start =====")
	fmt.Printf("%q\n", raw)
	fmt.Println("Raw >> ===== End =====")

	// Strip control characters
	cleaned := stripControlChars(raw)

	// Remove the actual marker, even if PTY wrapping split it across lines.
	cleaned = markerRegex.ReplaceAllString(cleaned, "")

	// Use a byte-budget approach to skip the echoed command across wrapped lines.
	// Initialize with the exact length of the sent command (including trailing newline).
	remainingEcho := len(cmdSent)

	lines := strings.Split(cleaned, "\n")
	var result []string

	for i, line := range lines {
		fmt.Printf("Line %d: %q\n", i, line)

		trimmed := strings.TrimSpace(line)

		// While we are still within the echoed command byte budget, drop lines entirely.
		// Account for the implicit '\n' that was removed by strings.Split by adding 1.
		if remainingEcho > 0 {
			fmt.Println("Judge >> Echoed command (budget)")
			remainingEcho -= len(line) + 1
			continue
		}

		// Skip empty lines
		if trimmed == "" {
			fmt.Println("Judge >> Empty line")
			continue
		}

		// Skip bash prompt patterns
		if ((strings.HasPrefix(trimmed, "bash-") || strings.Contains(trimmed, "@")) && (strings.HasSuffix(trimmed, "#") || strings.HasSuffix(trimmed, "$"))) ||
			trimmed == "$" || trimmed == "#" {
			fmt.Println("Judge >> Bash prompt line")
			continue
		}

		result = append(result, line)
		fmt.Println("Judge >> Keep!")
	}

	stringResults := strings.Join(result, "\n")
	stringResults = strings.Trim(stringResults, "\n")

	fmt.Println("Return >> ===== Start =====")
	fmt.Printf("%s\n", stringResults)
	fmt.Println("Return >> ===== End =====")

	return stringResults
}
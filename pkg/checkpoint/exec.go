package checkpoint

// pty-rpc-shell: https://github.com/Alex-XJK/pty-rpc-shell.git

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

func execCommand(socketPath, command string) (string, error) {
	// Connect to Unix socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to socket: %v\n", err)
		return "", err
	}
	defer conn.Close()

	// Send command
	writer := bufio.NewWriter(conn)
	_, err = writer.WriteString(command)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write command: %v\n", err)
		return "", err
	}
	writer.Flush()

	// Read all output until connection closes
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	output, err := io.ReadAll(conn)
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "failed to read output: %v\n", err)
		return "", err
	}

	return string(output), nil
}

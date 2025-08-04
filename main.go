package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Alex-XJK/checkpoint-lite/pkg/checkpoint"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: checkpoint-lite <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println("  init <work-directory>                        - Initialize environment")
		fmt.Println("  create <session> <pid> <checkpoint-id>       - Create checkpoint")
		fmt.Println("  restore <session> <checkpoint-id>            - Restore checkpoint")
		fmt.Println("  list <session>                               - List checkpoints")
		fmt.Println("  cleanup <session>                            - Clean up session")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		if len(os.Args) != 3 {
			fmt.Println("Usage: init <work-directory>")
			os.Exit(1)
		}
		workDir := os.Args[2]

		// Create a new manager with a random session
		manager, sessionID, err := checkpoint.NewManagerWithSession()
		if err != nil {
			fmt.Printf("Error creating session: %v\n", err)
			os.Exit(1)
		}

		overlayPath, err := manager.InitEnvironment(workDir)
		if err != nil {
			fmt.Printf("Error initializing environment: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Environment initialized!\n")
		fmt.Printf("Session ID: %s\n", sessionID)
		fmt.Printf("Work in this directory: %s\n", overlayPath)
		fmt.Printf("\nSave the session ID for future operations!\n")

	case "create":
		if len(os.Args) != 5 {
			fmt.Println("Usage: create <session> <pid> <checkpoint-id>")
			os.Exit(1)
		}
		sessionID := os.Args[2]
		pid, err := strconv.Atoi(os.Args[3])
		if err != nil {
			fmt.Printf("Invalid PID: %s\n", os.Args[3])
			os.Exit(1)
		}
		checkpointID := os.Args[4]

		manager, err := checkpoint.LoadManager(sessionID)
		if err != nil {
			fmt.Printf("Error loading session: %v\n", err)
			os.Exit(1)
		}

		if err := manager.CreateCheckpointParallel(pid, checkpointID); err != nil {
			fmt.Printf("Error creating checkpoint: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Checkpoint '%s' created successfully\n", checkpointID)

	case "restore":
		if len(os.Args) != 4 {
			fmt.Println("Usage: restore <session> <checkpoint-id>")
			os.Exit(1)
		}
		sessionID := os.Args[2]
		checkpointID := os.Args[3]

		manager, err := checkpoint.LoadManager(sessionID)
		if err != nil {
			fmt.Printf("Error loading session: %v\n", err)
			os.Exit(1)
		}

		newPID, err := manager.RestoreCheckpoint(checkpointID)
		if err != nil {
			fmt.Printf("Error restoring checkpoint: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Checkpoint '%s' restored, new PID: %d\n", checkpointID, newPID)

	case "list":
		if len(os.Args) != 3 {
			fmt.Println("Usage: list <session>")
			os.Exit(1)
		}
		sessionID := os.Args[2]

		manager, err := checkpoint.LoadManager(sessionID)
		if err != nil {
			fmt.Printf("Error loading session: %v\n", err)
			os.Exit(1)
		}

		checkpoints, err := manager.ListCheckpoints()
		if err != nil {
			fmt.Printf("Error listing checkpoints: %v\n", err)
			os.Exit(1)
		}
		if len(checkpoints) == 0 {
			fmt.Println("No checkpoints found")
		} else {
			fmt.Println("Available checkpoints:")
			for _, cp := range checkpoints {
				fmt.Printf("  %s\n", cp)
			}
		}

	case "cleanup":
		if len(os.Args) < 3 {
			fmt.Println("Usage: cleanup <session> [--interactive | --force]")
			os.Exit(1)
		}
		sessionID := os.Args[2]

		manager, err := checkpoint.LoadManager(sessionID)
		if err != nil {
			fmt.Printf("Error loading session: %v\n", err)
			os.Exit(1)
		}

		interactive := len(os.Args) > 3 && os.Args[3] == "--interactive"
		force := len(os.Args) > 3 && os.Args[3] == "--force"

		if interactive {
			if err := manager.CleanupInteractive(); err != nil {
				fmt.Printf("Error cleaning up session interactively: %v\n", err)
				os.Exit(1)
			}
		} else if force {
			if err := manager.CleanupForce(); err != nil {
				fmt.Printf("Error cleaning up session forcefully: %v\n", err)
				fmt.Printf("Try: sudo ./checkpoint-lite cleanup %s --interactive\n", sessionID)
				os.Exit(1)
			}
		} else {
			if err = manager.Cleanup(); err != nil {
				fmt.Printf("Error cleaning up session: %v\n", err)
				fmt.Printf("Try: sudo ./checkpoint-lite cleanup %s --force\n", sessionID)
				os.Exit(1)
			}
		}
		fmt.Printf("Session '%s' cleaned up successfully\n", sessionID)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

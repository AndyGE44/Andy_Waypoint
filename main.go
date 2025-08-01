package main

import (
	"fmt"
	"os"

	"github.com/Alex-XJK/checkpoint-lite/pkg/checkpoint"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: lightweight-checkpoint <command> [args...]")
		fmt.Println("Commands: create, restore, list")
		os.Exit(1)
	}

	manager := checkpoint.NewManager("/tmp/checkpoints")

	switch os.Args[1] {
	case "create":
		// TODO: implement create command
		fmt.Println("Create command - TODO")
	case "restore":
		// TODO: implement restore command
		fmt.Println("Restore command - TODO")
	case "list":
		// TODO: implement list command
		fmt.Println("List command - TODO")
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

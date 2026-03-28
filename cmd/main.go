package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	excesscommands "github.com/Flafl/DevOpsCore/internal/excessCommands/Nokia"
)

func main() {
	_ = godotenv.Load()

	user := os.Getenv("OLT_SSH_USER")
	pass := os.Getenv("OLT_SSH_PASS")
	if user == "" || pass == "" {
		log.Fatal("OLT_SSH_USER and OLT_SSH_PASS must be set (via .env or environment)")
	}

	fmt.Println("=== Nokia Backup Test (standalone) ===")
	fmt.Println("This will run 6 commands per OLT with 2-minute pauses between each.")
	fmt.Println()

	results := excesscommands.Backups(user, pass)

	fmt.Println()
	fmt.Println("=== Results ===")
	for _, r := range results {
		status := "OK"
		if r.Err != nil {
			status = fmt.Sprintf("ERROR: %v", r.Err)
		}
		fmt.Printf("  %-20s %-15s  %d bytes  %s\n", r.Device, r.Host, len(r.Data), status)
	}
	fmt.Printf("\nTotal OLTs: %d\n", len(results))
}

// examples/pidfile: usage example for the pidfile package
//
// To observe duplicate execution prevention in action:
//
//  1. Terminal A: go run ./examples/pidfile -hold
//     (creates PID file and waits)
//  2. Terminal B: go run ./examples/pidfile
//     → confirm "already running" error
//
// Usage:
//
//	go run ./examples/pidfile
//	go run ./examples/pidfile -debug
//	go run ./examples/pidfile -hold   # hold PID file for 30 seconds (duplicate run test)
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kinkist/x-media-downloader/logger"
	"github.com/kinkist/x-media-downloader/pidfile"
)

func main() {
	debugFlag := flag.Bool("debug", false, "enable debug output")
	holdFlag := flag.Bool("hold", false, "hold PID file for 30 seconds (for duplicate run testing)")
	flag.Parse()

	logger.Enabled = *debugFlag

	// ── 1. CheckAndCreatePidFile() ────────────────────────────────
	// - No PID file        → create with current PID and proceed
	// - PID file + process alive  → return error (block duplicate execution)
	// - PID file + process gone   → treat as previous crash, recreate
	if err := pidfile.CheckAndCreatePidFile(); err != nil {
		fmt.Fprintln(os.Stderr, "duplicate execution detected:", err)
		os.Exit(1)
	}

	// ── 2. RemovePidFile() — always register with defer ───────────
	// Deletes the PID file on normal exit.
	// On abnormal exit (panic, kill) the file remains but is cleaned up on next start.
	defer pidfile.RemovePidFile()

	fmt.Printf("[PID] current process PID: %d\n", os.Getpid())

	if *holdFlag {
		fmt.Println("waiting 30 seconds... (try running the same command in another terminal)")
		time.Sleep(30 * time.Second)
	}

	fmt.Println("done. PID file will be removed.")
}

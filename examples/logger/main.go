// examples/logger: usage example for the logger package
//
// Usage:
//
//	go run ./examples/logger
//	go run ./examples/logger -debug   # includes debug output
package main

import (
	"flag"
	"fmt"

	"github.com/kinkist/x-media-downloader/logger"
)

func main() {
	debugFlag := flag.Bool("debug", false, "enable debug mode")
	flag.Parse()

	// ── 1. set debug mode ─────────────────────────────────────────
	// logger.Enabled defaults to false. Set to true to enable Debug() output.
	logger.Enabled = *debugFlag

	// ── 2. regular output (independent of logger) ─────────────────
	fmt.Println("[INFO] program started")

	// ── 3. Debug() — only prints when Enabled=true ────────────────
	// Output format: [DEBUG HH:MM:SS.mmm] message
	logger.Debug("debug mode active: %v", *debugFlag)
	logger.Debug("int: %d, float: %.3f, string: %q", 42, 3.14, "hello")

	// ── 4. runtime toggle of Enabled ──────────────────────────────
	// on/off can be toggled with a single flag, no function call needed
	logger.Enabled = false
	logger.Debug("this line is not printed") // ignored because Enabled=false

	logger.Enabled = true
	logger.Debug("re-enabled")

	fmt.Println("[INFO] done")
}

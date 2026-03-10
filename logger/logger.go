// Package logger provides a simple debug logging utility.
// Debug output is controlled by the Enabled variable.
// Set logger.Enabled = true in main() to activate debug mode.
package logger

import (
	"fmt"
	"time"
)

// Enabled controls whether debug output is printed.
// Set to true via CLI flag (-debug) or config.yaml {"debug": true}.
var Enabled bool

// Debug prints a timestamped debug line to stdout if debug mode is enabled.
// Format is the same as fmt.Printf.
func Debug(format string, args ...any) {
	if !Enabled {
		return
	}
	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("[DEBUG %s] %s\n", ts, msg)
}

// examples/config: usage example for the config package
//
// Usage:
//
//	go run ./examples/config                      # uses config.yaml in the project root
//	go run ./examples/config -config /path/to/config.yaml
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kinkist/x-media-downloader/config"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (default: executable directory or CWD)")
	flag.Parse()

	// ── 1. config.Load() ──────────────────────────────────────────
	// If path is "", it searches in the following order:
	//   1) config.yaml in the same directory as the binary executable
	//   2) config.yaml in the current working directory (CWD) ← used by go run
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(1)
	}

	// ── 2. print loaded values ─────────────────────────────────────
	fmt.Println("=== config.yaml loaded ===")
	fmt.Printf("  datadir        : %q\n", cfg.Datadir)
	fmt.Printf("  debug          : %v\n", cfg.Debug)
	fmt.Println()
	fmt.Println("  [DB]")
	fmt.Printf("  dbhost         : %q\n", cfg.Dbhost)
	fmt.Printf("  dbuser         : %q\n", cfg.Dbuser)
	fmt.Printf("  dbdatabasename : %q\n", cfg.Dbdatabasename)
	// ── 3. conditional usage pattern ──────────────────────────────
	fmt.Println()
	if cfg.Dbhost != "" && cfg.Dbdatabasename != "" {
		fmt.Printf("→ DB configured: %s/%s\n", cfg.Dbhost, cfg.Dbdatabasename)
	} else {
		fmt.Println("→ no DB config (disabled)")
	}
}

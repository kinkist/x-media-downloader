// examples/db: usage example for the db package
//
// Prerequisites:
//   - MySQL server must be running
//   - config.yaml must have dbhost, dbuser, dbpass, dbdatabasename set
//
// Usage:
//
//	go run ./examples/db
//	go run ./examples/db -config /path/to/config.yaml
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kinkist/x-media-downloader/config"
	"github.com/kinkist/x-media-downloader/db"
	"github.com/kinkist/x-media-downloader/logger"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml")
	debugFlag := flag.Bool("debug", false, "enable debug output")
	flag.Parse()

	logger.Enabled = *debugFlag

	// ── 1. load DB connection info from config ────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(1)
	}
	if cfg.Dbhost == "" || cfg.Dbdatabasename == "" {
		fmt.Fprintln(os.Stderr, "config.yaml must have dbhost / dbdatabasename set")
		os.Exit(1)
	}

	// ── 2. db.Init() — MySQL connection + table initialization ────
	// Database is created automatically if it does not exist.
	// On failure, db.DB == nil and execution can continue (tracking disabled).
	dbCfg := db.Config{
		Host:   cfg.Dbhost,
		User:   cfg.Dbuser,
		Pass:   cfg.Dbpass,
		DBName: cfg.Dbdatabasename,
	}
	if err := db.Init(dbCfg); err != nil {
		fmt.Fprintln(os.Stderr, "DB connection failed:", err)
		os.Exit(1)
	}
	defer db.DB.Close()
	fmt.Printf("[DB] connected: %s/%s\n\n", cfg.Dbhost, cfg.Dbdatabasename)

	// ── 3. IsURLTracked() — query download history ────────────────
	// Returns true if the URL was already downloaded → can be skipped
	testURL := "https://pbs.twimg.com/media/example_image.jpg"

	tracked, err := db.IsURLTracked(testURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query failed:", err)
		os.Exit(1)
	}
	fmt.Printf("URL query: %s\n  → tracked: %v\n\n", testURL, tracked)

	// ── 4. MarkFileDownloaded() — record completed download ───────
	// INSERT IGNORE: silently ignores duplicate URLs (prevents double-insert)
	if !tracked {
		err = db.MarkFileDownloaded(
			testURL,                           // httpURL (unique key)
			"/data/2026/03/05/image/test.jpg", // filePath
			"1234567890123456789",             // tweetID
			"example_user",                    // username
			"9876543210",                      // userID
			"image",                           // fileType: "image" | "video" | "text"
			false,                             // isRetweet
		)
		if err != nil {
			fmt.Fprintln(os.Stderr, "record failed:", err)
			os.Exit(1)
		}
		fmt.Println("MarkFileDownloaded: record saved")

		// re-query to confirm
		tracked2, _ := db.IsURLTracked(testURL)
		fmt.Printf("re-query result → tracked: %v\n", tracked2)
	} else {
		fmt.Println("URL already recorded, skipping MarkFileDownloaded")
	}
}

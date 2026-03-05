// examples/processor: usage example for the processor package
//
// Creates a mock tweet with real X.com image URLs and runs ProcessTweet().
// DB / NSFW are optional — downloading works normally without them.
//
// Usage:
//
//	go run ./examples/processor                         # download only, no DB/NSFW
//	go run ./examples/processor -config config.json     # include DB + NSFW
//	go run ./examples/processor -datadir /tmp/tweets -debug
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kinkist/x-media-downloader/config"
	"github.com/kinkist/x-media-downloader/db"
	"github.com/kinkist/x-media-downloader/logger"
	"github.com/kinkist/x-media-downloader/nsfw"
	"github.com/kinkist/x-media-downloader/processor"
	"github.com/kinkist/x-media-downloader/twitterscraper"
)

func main() {
	configPath := flag.String("config", "", "path to config.json (optional)")
	dataDirFlag := flag.String("datadir", "data", "media storage directory")
	debugFlag := flag.Bool("debug", false, "enable debug output")
	flag.Parse()

	logger.Enabled = *debugFlag

	dataDir := *dataDirFlag

	// ── 1. load config (optional) ─────────────────────────────────
	// Ignore errors so the example works without a config.json.
	cfg, cfgErr := config.Load(*configPath)
	if cfgErr == nil {
		if cfg.Debug {
			logger.Enabled = true
		}
		if cfg.Datadir != "" && *dataDirFlag == "data" {
			dataDir = cfg.Datadir
		}

		// ── 2. DB initialization (optional) ──────────────────────
		// When DB is available, download history is tracked to prevent duplicates.
		if cfg.Dbhost != "" && cfg.Dbdatabasename != "" {
			dbCfg := db.Config{
				Host:   cfg.Dbhost,
				User:   cfg.Dbuser,
				Pass:   cfg.Dbpass,
				DBName: cfg.Dbdatabasename,
			}
			if err := db.Init(dbCfg); err != nil {
				fmt.Fprintf(os.Stderr, "[WARN] DB connection failed, continuing without DB: %v\n", err)
			} else {
				defer db.DB.Close()
				fmt.Printf("[DB] connected: %s/%s\n", cfg.Dbhost, cfg.Dbdatabasename)
			}
		}

		// ── 3. NSFW initialization (optional) ────────────────────
		// When NSFW model is available, detection runs automatically after download.
		if cfg.Nsfwmodelpath != "" {
			if err := nsfw.Init(cfg.Nsfwmodelpath, cfg.Onnxlibpath,
				cfg.Nsfwinputname, cfg.Nsfwoutputname); err != nil {
				fmt.Fprintf(os.Stderr, "[WARN] NSFW initialization failed, continuing without NSFW: %v\n", err)
			} else {
				defer nsfw.Close()
			}
		}
	} else {
		fmt.Printf("[INFO] config load skipped (%v), using defaults\n", cfgErr)
	}

	// ── 4. create mock tweet ──────────────────────────────────────
	// In real usage, pass *twitterscraper.Tweet returned by scraper.GetHomeTweets().
	//
	// Media types handled by processor.ProcessTweet:
	//   - Photos : Twitter images (pbs.twimg.com/media/...)
	//   - Videos : mp4 videos (video.twimg.com/...)
	//   - GIFs   : Twitter GIFs (mp4 format)
	//
	// Storage layout:
	//   {dataDir}/date/{YYYY}/{MM}/{DD}/image/{username}-{userID}-{tweetID}-{time}.jpg
	//   {dataDir}/date/{YYYY}/{MM}/{DD}/video/{username}-{userID}-{tweetID}-{time}.mp4
	//   {dataDir}/date/{YYYY}/{MM}/{DD}/text/{username}-{userID}-{tweetID}-{time}.txt
	tweet := &twitterscraper.Tweet{
		ID:          "1234567890123456789",
		Username:    "example_user",
		UserID:      "9876543210",
		Name:        "Example User",
		Text:        "This is a mock tweet for the processor example. #golang #example",
		TimeParsed:  time.Now(),
		Timestamp:   time.Now().Unix(),
		PermanentURL: "https://x.com/example_user/status/1234567890123456789",
		Likes:       42,
		Retweets:    7,
		Replies:     3,
		IsRetweet:   false,
		// real X.com image URL (public media)
		Photos: []twitterscraper.Photo{
			{
				ID:  "media_001",
				URL: "https://pbs.twimg.com/media/GqExample1?format=jpg&name=large",
			},
		},
	}

	// ── 5. run ProcessTweet() ─────────────────────────────────────
	// Internal steps:
	//   a. return immediately if no media (skip text-only tweets)
	//   b. check DB for duplicate URL → skip if already tracked
	//   c. download images/videos/GIFs
	//   d. run NSFW detection and save {file}.nsfwvalue.txt (if enabled)
	//   e. record downloaded URL in DB (if enabled)
	fmt.Printf("\n[PROCESSOR] processing tweet — ID=%s @%s\n", tweet.ID, tweet.Username)
	fmt.Printf("  storage path: %s\n\n", dataDir)

	if err := processor.ProcessTweet(tweet, dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "ProcessTweet failed:", err)
		os.Exit(1)
	}

	fmt.Println("\n[PROCESSOR] done")

	// ── 6. retweet example ────────────────────────────────────────
	// Retweets store media under .../retwitted/ using RetweetedStatus content.
	rtTweet := &twitterscraper.Tweet{
		ID:        "9999999999999999999",
		Username:  "retweeter",
		UserID:    "1111111111",
		IsRetweet: true,
		RetweetedStatus: &twitterscraper.Tweet{
			ID:         "1234567890123456789",
			Username:   "original_author",
			UserID:     "2222222222",
			Text:       "Original tweet text",
			TimeParsed: time.Now().Add(-time.Hour),
			Photos: []twitterscraper.Photo{
				{ID: "media_rt_001", URL: "https://pbs.twimg.com/media/GqExample2?format=jpg&name=large"},
			},
		},
	}

	fmt.Printf("\n[PROCESSOR] processing retweet — RT by @%s → original @%s\n",
		rtTweet.Username, rtTweet.RetweetedStatus.Username)
	if err := processor.ProcessTweet(rtTweet, dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "ProcessTweet(RT) failed:", err)
	}
	fmt.Println("[PROCESSOR] done")
}

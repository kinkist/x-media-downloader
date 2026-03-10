package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kinkist/x-media-downloader/config"
	"github.com/kinkist/x-media-downloader/cookieswithchromedp"
	"github.com/kinkist/x-media-downloader/db"
	"github.com/kinkist/x-media-downloader/loadcookies"
	"github.com/kinkist/x-media-downloader/logger"
	"github.com/kinkist/x-media-downloader/nsfw"
	"github.com/kinkist/x-media-downloader/pidfile"
	"github.com/kinkist/x-media-downloader/processor"
	"github.com/kinkist/x-media-downloader/twitterscraper"
)

const (
	cookieFile      = "cookies.json"
	maxConsecErrors = 5
)

func main() {
	// --- CLI flags ---
	var (
		debugFlag  = flag.Bool("debug", false, "enable debug mode")
		configPath = flag.String("config", "", "config file path (default: config.yaml next to executable or CWD)")
		countFlag  = flag.Int("count", 100, "maximum number of tweets to collect")
	)
	flag.Parse()

	// --- load config ---
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(1)
	}

	// --- debug mode (CLI flag takes priority, falls back to config) ---
	if *debugFlag || cfg.Debug {
		logger.Enabled = true
		logger.Debug("=== debug mode enabled ===")
	}
	logger.Debug("config loaded — datadir=%q dbhost=%q opennsfw2=%q nudenetv2=%q",
		cfg.Datadir, cfg.Dbhost, cfg.Opennsfw2modelpath, cfg.Nudenetv2modelpath)

	// --- prevent duplicate execution ---
	if err := pidfile.CheckAndCreatePidFile(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pidfile.RemovePidFile()

	// --- MySQL DB connection (only when dbhost/dbname is set in config) ---
	if cfg.Dbhost != "" && cfg.Dbdatabasename != "" {
		logger.Debug("DB config detected — host=%s db=%s", cfg.Dbhost, cfg.Dbdatabasename)
		dbCfg := db.Config{
			Host:   cfg.Dbhost,
			User:   cfg.Dbuser,
			Pass:   cfg.Dbpass,
			DBName: cfg.Dbdatabasename,
		}
		if err := db.Init(dbCfg); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] DB connection failed, continuing without tracking: %v\n", err)
		} else {
			defer db.DB.Close()
			fmt.Printf("[DB] MySQL connection established (%s/%s)\n", cfg.Dbhost, cfg.Dbdatabasename)
		}
	} else {
		fmt.Println("[DB] no DB config, continuing without file tracking")
		logger.Debug("no DB config (dbhost=%q dbdatabasename=%q)", cfg.Dbhost, cfg.Dbdatabasename)
	}

	// --- NSFW detector initialization (both can run simultaneously) ---
	nsfwAny := false

	if cfg.Opennsfw2modelpath != "" {
		logger.Debug("OpenNSFW2 config detected — path=%s", cfg.Opennsfw2modelpath)
		if err := nsfw.Init(cfg.Opennsfw2modelpath, cfg.Onnxlibpath,
			cfg.Opennsfw2inputname, cfg.Opennsfw2outputname); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] OpenNSFW2 init failed, continuing without it: %v\n", err)
		} else {
			defer nsfw.Close()
			nsfwAny = true
		}
	}

	if cfg.Nudenetv2modelpath != "" {
		logger.Debug("NudeNet v2 config detected — path=%s", cfg.Nudenetv2modelpath)
		if err := nsfw.InitNudeNet(cfg.Nudenetv2modelpath, cfg.Onnxlibpath); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] NudeNet v2 init failed, continuing without it: %v\n", err)
		} else {
			defer nsfw.CloseNudeNet()
			nsfwAny = true
		}
	}

	if !nsfwAny {
		fmt.Println("[NSFW] no NSFW model configured, skipping")
		logger.Debug("NSFW disabled (opennsfw2modelpath and nudenetv2modelpath not set)")
	}

	// --- determine data storage path ---
	dataDir := cfg.Datadir
	if dataDir == "" {
		dataDir = "data"
	}
	logger.Debug("data storage path: %s", dataDir)

	// --- branch depending on whether cookies.json exists ---
	if _, err := os.Stat(cookieFile); os.IsNotExist(err) {
		// no cookies.json → open Chrome for manual login and save cookies
		logger.Debug("cookies.json not found — starting login mode")
		if err := cookieswithchromedp.Run(cookieFile); err != nil {
			fmt.Fprintln(os.Stderr, "login failed:", err)
			os.Exit(1)
		}
	} else {
		// cookies.json found → fetch home timeline tweets and download media
		logger.Debug("cookies.json found — starting tweet fetch mode (count=%d)", *countFlag)
		if err := fetchTweets(cookieFile, dataDir, *countFlag, cfg.Exceptpromoted); err != nil {
			fmt.Fprintln(os.Stderr, "failed to fetch tweets:", err)
			os.Exit(1)
		}
	}
}

// fetchTweets reads cookies from cookieFile, collects up to count home timeline
// tweets, and saves media to dataDir. Cookie changes during the session are
// persisted back to cookieFile.
func fetchTweets(cookieFile, dataDir string, count int, exceptPromoted bool) error {
	// load cookies and initialize scraper
	raw, err := loadcookies.LoadRaw(cookieFile)
	if err != nil {
		return fmt.Errorf("failed to load cookies: %w", err)
	}
	httpCookies, origValues := loadcookies.ToHTTPCookies(raw)

	scraper := twitterscraper.New()
	scraper.SetCookies(httpCookies)

	logger.Debug("starting home timeline collection — up to %d tweets, path: %s", count, dataDir)
	fmt.Printf("fetching home timeline tweets... (up to %d)\n\n", count)

	totalStart := time.Now()
	fetched := 0
	consecErrors := 0

	for result := range scraper.GetHomeTweets(context.Background(), count) {
		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %v\n", result.Error)
			consecErrors++
			if consecErrors >= maxConsecErrors {
				fmt.Fprintf(os.Stderr, "[FATAL] exceeded %d consecutive errors, stopping\n", maxConsecErrors)
				break
			}
			continue
		}
		consecErrors = 0
		fetched++

		tw := &result.Tweet
		printTweet(fetched, tw)

		// skip promoted (ad) tweets if configured
		if exceptPromoted && tw.IsPromoted {
			logger.Debug("tweet [%d] skipped — promoted tweet (exceptpromoted=true)", fetched)
			fmt.Printf("  [SKIP] promoted tweet\n")
			continue
		}

		// download media (images/videos/GIFs)
		tweetStart := time.Now()
		if err := processor.ProcessTweet(tw, dataDir); err != nil {
			fmt.Fprintf(os.Stderr, "  [ERROR] failed to process tweet %s: %v\n", tw.ID, err)
		}
		logger.Debug("tweet [%d] processed (%.3fs)", fetched, time.Since(tweetStart).Seconds())
	}

	elapsed := time.Since(totalStart)
	fmt.Printf("\ntotal %d tweets processed (path: %s, elapsed: %.1fs)\n",
		fetched, dataDir, elapsed.Seconds())

	// persist any cookie changes that occurred during the session
	if loadcookies.Sync(cookieFile, raw, origValues, scraper.GetCookies()) {
		fmt.Printf("cookies updated and saved to %s\n", cookieFile)
	}

	return nil
}

// printTweet prints a tweet summary to stdout.
func printTweet(n int, tw *twitterscraper.Tweet) {
	prefix := ""
	if tw.IsRetweet && tw.RetweetedStatus != nil {
		prefix = fmt.Sprintf("[RT @%s] ", tw.RetweetedStatus.Username)
	}

	mediaTag := ""
	if len(tw.Photos) > 0 || len(tw.Videos) > 0 || len(tw.GIFs) > 0 {
		src := tw
		if tw.IsRetweet && tw.RetweetedStatus != nil {
			src = tw.RetweetedStatus
		}
		mediaTag = fmt.Sprintf(" [📎 photos:%d videos:%d GIFs:%d]",
			len(src.Photos), len(src.Videos), len(src.GIFs))
	}

	fmt.Printf("─────────────────────────────────────────\n")
	fmt.Printf("[%d] @%s%s\n", n, tw.Username, mediaTag)
	fmt.Printf("%s%s\n", prefix, tw.Text)
	fmt.Printf("♥ %d  🔁 %d  💬 %d  %s\n\n",
		tw.Likes, tw.Retweets, tw.Replies,
		tw.TimeParsed.Format("2006-01-02 15:04"))

	logger.Debug("tweet [%d] ID=%s @%s time=%s photos=%d videos=%d gifs=%d rt=%v",
		n, tw.ID, tw.Username,
		tw.TimeParsed.Format("2006-01-02 15:04:05"),
		len(tw.Photos), len(tw.Videos), len(tw.GIFs),
		tw.IsRetweet)
}

// examples/processor: usage example for the processor package
//
// Creates a mock tweet with real X.com image URLs and runs ProcessTweet().
// DB / NSFW are optional — downloading works normally without them.
//
// Usage:
//
//	go run ./examples/twitterscraper
//	go run ./examples/twitterscraper -cookies /path/to/cookies.json
package main

import (
	"os"
	"flag"
	"fmt"
	"context"

	"github.com/kinkist/x-media-downloader/loadcookies"
	"github.com/kinkist/x-media-downloader/twitterscraper"
)

func main() {
	const count = 10

	// flag for Load Cookie, github.com/kinkist/x-media-downloader/twitterscraper could not login with ID/PASS
	cookiePath := flag.String("cookies", "cookies.json", "path to cookies.json")
	flag.Parse()

	raw, err := loadcookies.LoadRaw(*cookiePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load cookies:", err)
		os.Exit(1)
	}

	// Load cookie json file
	httpCookies, origValues := loadcookies.ToHTTPCookies(raw)

	// Create scraper from twitterscraper module
	scraper := twitterscraper.New()
	// set cookie to scraper github.com/kinkist/x-media-downloader/twitterscraper/auth.go
	scraper.SetCookies(httpCookies)

	// use GetHomeTweets function of github.com/kinkist/x-media-downloader/twitterscraper/tweets.go
	// GetHomeTweets -> fetchHomeTweets -> request to "https://x.com/i/api/graphql/9EwYy8pLBOSFlEoSP2STiQ/HomeLatestTimeline"
	HomeLatestTimeline := scraper.GetHomeTweets(context.Background(), count)

	for result := range HomeLatestTimeline {
		fmt.Println(result)
	}

	// use GetHomeTweets function of github.com/kinkist/x-media-downloader/twitterscraper/bookmarks.go
	Bookmarks := scraper.GetBookmarks(context.Background(), count)

	for result := range Bookmarks {
		fmt.Println(result)
	}



	// persist any cookie changes that occurred during the session
	if loadcookies.Sync(*cookiePath, raw, origValues, scraper.GetCookies()) {
		fmt.Printf("cookies updated and saved to %s\n", cookiePath)
	}



}
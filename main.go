package main

import (
	"fmt"
	"os"

	"github.com/kinkist/x-media-downloader/cookieswithchromedp"
	"github.com/kinkist/x-media-downloader/gethometweets"
)

const cookieFile = "cookies.json"

func main() {
	if _, err := os.Stat(cookieFile); os.IsNotExist(err) {
		// No cookies.json found — open Chrome for manual login and save cookies.
		if err := cookieswithchromedp.Run(cookieFile); err != nil {
			fmt.Fprintln(os.Stderr, "Login failed:", err)
			os.Exit(1)
		}
	} else {
		// cookies.json exists — fetch and print home timeline tweets.
		if err := gethometweets.Run(cookieFile); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to fetch tweets:", err)
			os.Exit(1)
		}
	}
}

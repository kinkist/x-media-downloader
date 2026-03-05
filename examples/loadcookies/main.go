// examples/loadcookies: usage example for the loadcookies package
//
// cookies.json must exist in the project root.
//
// Usage:
//
//	go run ./examples/loadcookies
//	go run ./examples/loadcookies -cookies /path/to/cookies.json
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/kinkist/x-media-downloader/loadcookies"
)

func main() {
	cookiePath := flag.String("cookies", "cookies.json", "path to cookies.json")
	flag.Parse()

	// ── 1. LoadRaw() — file → struct slice ────────────────────────
	// All cookie fields (domain, path, expires, httpOnly, secure, sameSite)
	// are preserved so that Sync() can round-trip without metadata loss.
	raw, err := loadcookies.LoadRaw(*cookiePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load cookies:", err)
		os.Exit(1)
	}

	fmt.Printf("loaded cookies: %d\n\n", len(raw))
	for _, c := range raw {
		exp := time.Unix(int64(c.Expires), 0).Format("2006-01-02")
		fmt.Printf("  %-20s = %-30s  (domain=%s expires=%s httpOnly=%v)\n",
			c.Name, truncate(c.Value, 30), c.Domain, exp, c.HTTPOnly)
	}

	// ── 2. ToHTTPCookies() — struct → http.Cookie + original value map ──
	// Converts to stripped values ('"' characters removed).
	// origValues is used by Sync() as the baseline for change detection.
	httpCookies, origValues := loadcookies.ToHTTPCookies(raw)

	fmt.Printf("\nhttp.Cookie conversion: %d\n", len(httpCookies))
	fmt.Println("(origValues: used by Sync() as the change detection baseline)")

	// ── 3. Sync() — save changed cookies to file after session ────
	// Simulation: modify the ct0 cookie value to verify Sync behavior.
	// In real usage, pass the return value of scraper.GetCookies().
	simulatedJar := cloneWithChange(httpCookies, "ct0", "new_csrf_token_example")

	tmpPath := *cookiePath + ".example_sync.json"
	defer os.Remove(tmpPath) // remove temp file after example run

	// copy original file to temp path for change detection
	if data, err := os.ReadFile(*cookiePath); err == nil {
		os.WriteFile(tmpPath, data, 0600)
	}

	// Sync: update only changed cookies based on origValues, preserve all other metadata
	if loadcookies.Sync(tmpPath, raw, origValues, simulatedJar) {
		fmt.Printf("\nSync result: change detected → saved to %s\n", tmpPath)
	} else {
		fmt.Println("\nSync result: no changes (file not written)")
	}
}

// truncate cuts a string to max bytes with "…" if it exceeds max.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// cloneWithChange copies httpCookies and replaces the value of the named cookie with newValue.
// In real code, scraper.GetCookies() provides this functionality.
func cloneWithChange(cookies []*http.Cookie, name, newValue string) []*http.Cookie {
	out := make([]*http.Cookie, len(cookies))
	for i, c := range cookies {
		clone := *c
		if clone.Name == name {
			clone.Value = newValue
		}
		out[i] = &clone
	}
	return out
}

package gethometweets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kinkist/x-media-downloader/twitterscraper"
)

// cookie is the full-fidelity representation of a single entry in cookies.json.
// All fields are preserved so that the file can be round-tripped without data loss.
type cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite,omitempty"`
}

// Run loads cookies from cookieFile, fetches the 20 most recent home timeline
// tweets, and saves cookies back to cookieFile if any values were updated
// during the session (e.g. ct0 refresh).
func Run(cookieFile string) error {
	raw, err := loadRawCookies(cookieFile)
	if err != nil {
		return fmt.Errorf("failed to load cookies: %w", err)
	}

	// Build the http.Cookie slice for the scraper, stripping illegal '"' bytes.
	// Track the stripped value alongside the raw struct so change detection
	// compares against what was actually stored in the cookie jar.
	origValues := make(map[string]string, len(raw))
	httpCookies := make([]*http.Cookie, 0, len(raw))
	for _, c := range raw {
		stripped := strings.ReplaceAll(c.Value, "\"", "")
		origValues[c.Name] = stripped
		httpCookies = append(httpCookies, &http.Cookie{
			Name:     c.Name,
			Value:    stripped,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  time.Unix(int64(c.Expires), 0),
			HttpOnly: c.HTTPOnly,
			Secure:   c.Secure,
		})
	}

	scraper := twitterscraper.New()
	scraper.SetCookies(httpCookies)

	fmt.Println("Fetching home timeline tweets...\n")

	count := 0
	for result := range scraper.GetHomeTweets(context.Background(), 20) {
		if result.Error != nil {
			return fmt.Errorf("tweet error: %w", result.Error)
		}
		count++
		t := result.Tweet
		fmt.Printf("─────────────────────────────────────────\n")
		fmt.Printf("[%d] %s (@%s)  %s\n", count, t.Name, t.Username, t.TimeParsed.Format("2006-01-02 15:04"))
		fmt.Printf("%s\n", t.Text)
		fmt.Printf("♥ %d  🔁 %d  💬 %d\n", t.Likes, t.Retweets, t.Replies)
		fmt.Printf("%s\n\n", t.PermanentURL)
	}

	fmt.Printf("Printed %d tweets in total.\n", count)

	// Persist any cookie changes that occurred during the session.
	if updated := syncCookies(cookieFile, raw, origValues, scraper.GetCookies()); updated {
		fmt.Printf("Cookies updated and saved to %s.\n", cookieFile)
	}

	return nil
}

// loadRawCookies reads cookieFile and returns the parsed cookie list.
func loadRawCookies(path string) ([]*cookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cookies []*cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}
	return cookies, nil
}

// syncCookies compares the current jar cookies against the original stripped
// values. If any cookie value changed or a new cookie appeared, the raw list
// is updated in-place and written back to path. Returns true if the file was
// written.
func syncCookies(path string, raw []*cookie, orig map[string]string, current []*http.Cookie) bool {
	// Index the raw list by name for O(1) lookup.
	byName := make(map[string]*cookie, len(raw))
	for _, c := range raw {
		byName[c.Name] = c
	}

	changed := false
	for _, cur := range current {
		if rc, ok := byName[cur.Name]; ok {
			// Existing cookie — update value if it changed.
			if orig[cur.Name] != cur.Value {
				rc.Value = cur.Value
				changed = true
			}
		} else {
			// New cookie added by X.com during the session.
			raw = append(raw, &cookie{
				Name:   cur.Name,
				Value:  cur.Value,
				Domain: ".x.com",
				Path:   "/",
			})
			changed = true
		}
	}

	if !changed {
		return false
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to marshal cookies: %v\n", err)
		return false
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", path, err)
		return false
	}
	return true
}

package cookieswithchromedp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	loginURL     = "https://x.com/i/flow/login"
	pollInterval = time.Second
	maxWait      = 5 * time.Minute
)

// Cookie represents a browser cookie stored in the JSON file.
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite,omitempty"`
}

// Run opens a visible Chrome window, navigates to the x.com login page,
// waits for the user to log in, then saves all cookies to cookieFile.
func Run(cookieFile string) error {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "+
			"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	fmt.Println("Opening Chrome and navigating to x.com login page...")
	fmt.Println("Please log in using the browser.")

	if err := chromedp.Run(ctx, chromedp.Navigate(loginURL)); err != nil {
		return fmt.Errorf("failed to navigate to login page: %w", err)
	}

	fmt.Printf("Waiting for login to complete... (timeout: %s)\n", maxWait)

	cookies, err := waitForLogin(ctx, maxWait)
	if err != nil {
		return err
	}

	if err := saveCookies(cookieFile, cookies); err != nil {
		return fmt.Errorf("failed to save cookies: %w", err)
	}

	fmt.Printf("\nLogin successful! Saved %d cookies to %s.\n", len(cookies), cookieFile)

	// Print a preview of the critical auth cookies.
	for _, c := range cookies {
		if c.Name == "auth_token" || c.Name == "ct0" {
			preview := c.Value
			if len(preview) > 20 {
				preview = preview[:20] + "..."
			}
			fmt.Printf("  %-12s: %s\n", c.Name, preview)
		}
	}

	return nil
}

// waitForLogin polls the browser's cookies every second until auth_token is set
// or the timeout expires.
func waitForLogin(ctx context.Context, timeout time.Duration) ([]*Cookie, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		var raw []*network.Cookie
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			raw, err = network.GetCookies().Do(ctx)
			return err
		})); err != nil {
			// Browser may not be ready yet — retry silently.
			continue
		}

		for _, c := range raw {
			if c.Name == "auth_token" && c.Value != "" {
				fmt.Println("\n✓ auth_token cookie detected — login complete!")
				return convertCookies(raw), nil
			}
		}
	}

	return nil, fmt.Errorf("login timed out after %s", timeout)
}

// convertCookies converts cdproto network cookies to the JSON-serializable Cookie struct.
func convertCookies(raw []*network.Cookie) []*Cookie {
	out := make([]*Cookie, 0, len(raw))
	for _, c := range raw {
		out = append(out, &Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  float64(c.Expires),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: c.SameSite.String(),
		})
	}
	return out
}

// saveCookies writes cookies to a JSON file with restricted permissions.
func saveCookies(path string, cookies []*Cookie) error {
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

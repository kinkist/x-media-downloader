// Package loadcookies provides utilities for loading, converting, and
// persisting X.com session cookies stored in cookies.json.
package loadcookies

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Cookie is the full-fidelity representation of a single entry in cookies.json.
// All fields are preserved so that the file can be round-tripped without data loss.
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

// LoadRaw reads the cookies.json file at path and returns the raw cookie list.
// All original fields (domain, path, expires, flags) are preserved for later
// round-tripping via Sync.
func LoadRaw(path string) ([]*Cookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cookies []*Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}
	return cookies, nil
}

// ToHTTPCookies converts raw cookies to the []*http.Cookie format required by
// the scraper, stripping illegal '"' bytes from values (e.g. twid, g_state).
//
// It also returns origValues — a map of name → stripped value — which serves
// as the baseline for change detection in Sync.
func ToHTTPCookies(raw []*Cookie) ([]*http.Cookie, map[string]string) {
	origValues := make(map[string]string, len(raw))
	httpCookies := make([]*http.Cookie, 0, len(raw))

	for _, c := range raw {
		// Go's net/http silently drops '"' bytes from cookie values.
		// X.com cookies like twid ("u=12345") and g_state ({"i_l":0,...}) contain them,
		// so we strip them in advance to store cookies in the jar without warnings.
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
	return httpCookies, origValues
}

// Sync compares the scraper's current cookie jar (current) against the
// original stripped values (orig). If any cookie value changed or a new
// cookie appeared, the raw list is updated in-place and written back to path.
//
// Only cookie values are updated; domain/path/expires/flags from the original
// file are preserved. Returns true if the file was written.
func Sync(path string, raw []*Cookie, orig map[string]string, current []*http.Cookie) bool {
	// index the raw list by name
	byName := make(map[string]*Cookie, len(raw))
	for _, c := range raw {
		byName[c.Name] = c
	}

	changed := false
	for _, cur := range current {
		if rc, ok := byName[cur.Name]; ok {
			// existing cookie — update only if value changed
			if orig[cur.Name] != cur.Value {
				rc.Value = cur.Value
				changed = true
			}
		} else {
			// new cookie added by X.com during the session
			raw = append(raw, &Cookie{
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
		fmt.Fprintf(os.Stderr, "[WARN] failed to serialize cookies: %v\n", err)
		return false
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] failed to save %s: %v\n", path, err)
		return false
	}
	return true
}

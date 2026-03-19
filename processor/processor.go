// Package processor handles tweet media downloading and file organization.
package processor

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kinkist/x-media-downloader/db"
	"github.com/kinkist/x-media-downloader/logger"
	"github.com/kinkist/x-media-downloader/nsfw"
	"github.com/kinkist/x-media-downloader/twitterscraper"
)

// ProcessTweet processes only tweets that contain media (images/videos/GIFs).
// Retweets are stored under the retwitted/ subdirectory using the original tweet's content and media.
//
// Storage layout:
//
//	date/{YYYY}/{MM}/{DD}/image[/retwitted]/{screenname}-{userid}-{tweetID}.{ext}
//	date/{YYYY}/{MM}/{DD}/video[/retwitted]/{screenname}-{userid}-{tweetID}.mp4
//	date/{YYYY}/{MM}/{DD}/video[/retwitted]/{screenname}-{userid}-{tweetID}.jpg  (thumb)
//	date/{YYYY}/{MM}/{DD}/text[/retwitted]/{screenname}-{userid}-{tweetID}.txt
func ProcessTweet(tweet *twitterscraper.Tweet, dataDir string) error {
	// self-retweets (user retweeting their own tweet) are treated as originals
	isRT := tweet.IsRetweet && tweet.RetweetedStatus != nil &&
		!strings.EqualFold(tweet.Username, tweet.RetweetedStatus.Username)

	// source tweet (uses RetweetedStatus for retweets)
	src := tweet
	if isRT {
		src = tweet.RetweetedStatus
	}

	logger.Debug("processing tweet — ID=%s @%s isRT=%v photos=%d videos=%d gifs=%d",
		src.ID, src.Username, isRT, len(src.Photos), len(src.Videos), len(src.GIFs))

	hasMedia := len(src.Photos) > 0 || len(src.Videos) > 0 || len(src.GIFs) > 0
	if !hasMedia {
		logger.Debug("no media, skipping")
		return nil
	}

	// date-based root path (based on the time of command execution)
	now := time.Now()
	dateBase := filepath.Join(dataDir, "date",
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		fmt.Sprintf("%02d", now.Day()),
	)
	logger.Debug("base storage path: %s", dateBase)

	// subdirectory based on retweet status
	rtSub := ""
	if isRT {
		rtSub = "retwitted"
	}

	// base filename: {screenname}-{userid}-{tweetID}-{tweet creation time}
	tweetTime := src.TimeParsed.Format("20060102_150405")
	baseName := fmt.Sprintf("%s-%s-%s-%s", sanitizeUsername(src.Username), src.UserID, src.ID, tweetTime)
	logger.Debug("base filename: %s", baseName)

	// --- save text ---
	textDir := filepath.Join(dateBase, "text", rtSub)
	if err := os.MkdirAll(textDir, 0755); err != nil {
		return fmt.Errorf("mkdir text: %w", err)
	}
	txtPath := filepath.Join(textDir, baseName+".txt")
	logger.Debug("saving text: %s", txtPath)
	txtContent := buildTextContent(tweet, src, isRT)
	if err := os.WriteFile(txtPath, []byte(txtContent), 0644); err != nil {
		return fmt.Errorf("write txt: %w", err)
	}

	// --- save images ---
	if len(src.Photos) > 0 {
		imgDir := filepath.Join(dateBase, "image", rtSub)
		if err := os.MkdirAll(imgDir, 0755); err != nil {
			return fmt.Errorf("mkdir image: %w", err)
		}
		for i, photo := range src.Photos {
			ext := photoExt(photo.URL)
			name := indexedName(baseName, i, len(src.Photos))
			dst := filepath.Join(imgDir, name+ext)
			logger.Debug("downloading image [%d/%d] URL=%s → %s",
				i+1, len(src.Photos), photo.URL, dst)
			downloadTrackAndNSFW(photo.URL, dst, src.ID, src.Username, src.UserID, "image", isRT)
		}
	}

	// --- save videos (including GIFs — Twitter GIFs are mp4) ---
	videos := src.Videos
	gifs := src.GIFs
	if len(videos) > 0 || len(gifs) > 0 {
		vidDir := filepath.Join(dateBase, "video", rtSub)
		if err := os.MkdirAll(vidDir, 0755); err != nil {
			return fmt.Errorf("mkdir video: %w", err)
		}

		for i, video := range videos {
			if video.URL == "" {
				logger.Debug("video [%d] no URL, skipping", i)
				continue
			}
			name := indexedName(baseName, i, len(videos)+len(gifs))
			dst := filepath.Join(vidDir, name+".mp4")
			logger.Debug("downloading video [%d/%d] URL=%s → %s",
				i+1, len(videos), video.URL, dst)
			downloadTrackAndNSFW(video.URL, dst, src.ID, src.Username, src.UserID, "video", isRT)

			if video.Preview != "" {
				vidPreviewDir := filepath.Join(dateBase, "image", rtSub)
				if err := os.MkdirAll(vidPreviewDir, 0755); err != nil {
					return fmt.Errorf("mkdir video Preview : %w", err)
				}
				thumbExt := photoExt(video.Preview)
				thumbDst := filepath.Join(vidPreviewDir, name+thumbExt)
				logger.Debug("video thumbnail URL=%s → %s", video.Preview, thumbDst)
				downloadTrackAndNSFW(video.Preview, thumbDst, src.ID, src.Username, src.UserID, "image", isRT)
			} else {
				logger.Debug("video [%d] no thumbnail", i+1)
			}
		}

		offset := len(videos)
		for i, gif := range gifs {
			if gif.URL == "" {
				logger.Debug("GIF [%d] no URL, skipping", i)
				continue
			}
			name := indexedName(baseName, offset+i, len(videos)+len(gifs))
			dst := filepath.Join(vidDir, name+".mp4")
			logger.Debug("downloading GIF [%d/%d] URL=%s → %s",
				i+1, len(gifs), gif.URL, dst)
			downloadTrackAndNSFW(gif.URL, dst, src.ID, src.Username, src.UserID, "video", isRT)
		}
	}

	return nil
}

// decideDownload determines whether a file needs to be downloaded for rawURL/dst.
//
// With DB connected:
//
//	URL tracked + file exists  → [SKIP]   return false
//	URL tracked + file missing → [RETRY]  return true
//	URL not tracked            → download needed, return true
//
// Without DB:
//
//	file exists  → [SKIP]  return false
//	file missing → download needed, return true
func decideDownload(rawURL, dst string) bool {
	name := filepath.Base(dst)
	dbURL := stripQuery(rawURL) // strip ?tag=12 etc. before DB lookup
	if db.DB != nil {
		tracked, err := db.IsURLTracked(dbURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [WARN] DB query failed (%s): %v — proceeding with download\n", name, err)
			return true
		}
		if tracked {
			if _, statErr := os.Stat(dst); statErr == nil {
				fmt.Printf("  [SKIP] already saved: %s\n", name)
				return false
			}
			fmt.Printf("  [RETRY] file missing, re-downloading: %s\n", name)
			return true
		}
		logger.Debug("not in DB, proceeding with download: %s", name)
		return true
	}
	// DB not connected: decide based on file existence
	if _, statErr := os.Stat(dst); statErr == nil {
		fmt.Printf("  [SKIP] file already exists: %s\n", name)
		return false
	}
	logger.Debug("DB disabled, file not found → downloading: %s", name)
	return true
}

// hasNSFWValue reports whether filePath + ".nsfwvalue.txt" exists.
func hasNSFWValue(filePath string) bool {
	_, err := os.Stat(filePath + ".nsfwvalue.txt")
	return err == nil
}

// downloadTrackAndNSFW performs download decision → download → NSFW detection → DB record in sequence.
//
//  1. decideDownload: return immediately if skip condition met
//  2. on download failure, log warning and return (no DB record)
//  3. NSFW: skip if .nsfwvalue.txt already exists (treated as success);
//     otherwise run detection — on failure, no DB record
//  4. DB record only when both download and NSFW succeeded
func downloadTrackAndNSFW(rawURL, dst, tweetID, username, userID, fileType string, isRetweet bool) {
	if !decideDownload(rawURL, dst) {
		return
	}

	// download
	t0 := time.Now()
	if err := downloadFile(rawURL, dst); err != nil {
		fmt.Fprintf(os.Stderr, "  [WARN] download failed (%s): %v\n", filepath.Base(dst), err)
		return
	}
	logger.Debug("download complete (%.3fs): %s", time.Since(t0).Seconds(), filepath.Base(dst))

	// NSFW detection (skip if .nsfwvalue.txt already exists — treated as success)
	nsfwOK := true
	if hasNSFWValue(dst) {
		logger.Debug("NSFW result already exists, skipping: %s", filepath.Base(dst))
	} else {
		nsfwOK = nsfw.DetectAndSaveNSFW(dst)
	}

	// record in DB only when both download and NSFW succeeded
	// store URL without query params so lookups are stable regardless of ?tag= value
	if db.DB != nil && nsfwOK {
		if err := db.MarkFileDownloaded(stripQuery(rawURL), dst, tweetID, username, userID, fileType, isRetweet); err != nil {
			fmt.Fprintf(os.Stderr, "  [WARN] DB record failed (%s): %v\n", filepath.Base(dst), err)
		}
	}
}

// indexedName returns {baseName}-{N} when there are 2+ media items,
// or baseName unchanged for a single item.
func indexedName(baseName string, idx, total int) string {
	if total <= 1 {
		return baseName
	}
	return fmt.Sprintf("%s-%d", baseName, idx)
}

// buildTextContent builds the content of the text file to be saved.
func buildTextContent(tweet, src *twitterscraper.Tweet, isRT bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ID: %s\n", src.ID))
	sb.WriteString(fmt.Sprintf("Username: @%s\n", src.Username))
	sb.WriteString(fmt.Sprintf("UserID: %s\n", src.UserID))
	sb.WriteString(fmt.Sprintf("Time: %s\n", src.TimeParsed.Format("2006-01-02 15:04:05")))
	if isRT {
		sb.WriteString(fmt.Sprintf("Retweeted by: @%s\n", tweet.Username))
	}
	sb.WriteString("---\n")
	if isRT {
		sb.WriteString(fmt.Sprintf("[RT @%s]\n", src.Username))
	}
	sb.WriteString(src.Text)
	return sb.String()
}

// downloadFile fetches a URL and saves the response body to dst.
func downloadFile(rawURL, dst string) error {
	logger.Debug("HTTP GET: %s", rawURL)
	resp, err := http.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	logger.Debug("HTTP response: %d %s", resp.StatusCode, resp.Status)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, rawURL)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	logger.Debug("file saved: %s (%d bytes)", filepath.Base(dst), n)
	return err
}

// reUnsafeUsername matches any character not allowed in a Twitter username [A-Za-z0-9_].
// Compiled once at package level for efficiency.
var reUnsafeUsername = regexp.MustCompile(`[^A-Za-z0-9_]`)

// sanitizeUsername strips characters that are unsafe for filenames and enforces a
// maximum length of 50. Returns "_" if the result would otherwise be empty.
//
// Twitter usernames are officially [A-Za-z0-9_], but the API occasionally returns
// empty or non-ASCII values (e.g. for restricted accounts). Without sanitization,
// baseName would start with "-" and produce paths like "-userid-tweetid-time.txt".
func sanitizeUsername(name string) string {
	s := reUnsafeUsername.ReplaceAllString(name, "")
	if len(s) > 50 {
		s = s[:50]
	}
	if s == "" {
		return "_"
	}
	return s
}

// stripQuery removes query parameters from a URL, returning scheme://host/path only.
// Used to normalize media URLs before DB lookup and storage — e.g.
// "https://video.twimg.com/.../file.mp4?tag=12" → "https://video.twimg.com/.../file.mp4"
// Falls back to the original URL if parsing fails.
func stripQuery(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	return u.String()
}

// photoExt extracts the file extension from a Twitter image URL.
// Twitter URL format: https://pbs.twimg.com/media/XXX?format=jpg&name=large
func photoExt(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ".jpg"
	}
	if format := u.Query().Get("format"); format != "" {
		logger.Debug("extracted extension from URL format param: %s", format)
		return "." + format
	}
	if ext := filepath.Ext(u.Path); ext != "" {
		logger.Debug("extracted extension from URL path: %s", ext)
		return ext
	}
	return ".jpg"
}

# x-media-downloader

A Go CLI tool that logs into X.com (formerly Twitter) via a real Chrome browser, saves session cookies, and automatically collects and downloads media (images, videos, GIFs) from your home timeline.

---

## Features

| Feature | Description |
|---------|-------------|
| Home timeline collection | Fetches tweets automatically (`-count` flag controls the limit) |
| Liked tweets collection | Fetches the authenticated user's liked tweets via `GetLikes` / `FetchLikes`; `userId` is auto-extracted from the `twid` cookie |
| Media download | Downloads images, videos, and GIFs; video thumbnails are saved to `image/` |
| Retweet handling | Stored under `retwitted/` subdirectory using the original author's info; self-retweets (user retweeting their own tweet) are treated as originals and stored in the standard directory |
| Promoted tweet detection | `Tweet.IsPromoted` and `Tweet.PromotedMetadata` populated for ad tweets; `exceptpromoted: true` in config skips media download for promoted tweets |
| Date-based directories | Auto-organized as `date/YYYY/MM/DD/` |
| Duplicate prevention | MySQL DB-based download tracking (optional); falls back to file-existence check |
| NSFW detection | GPU-accelerated image/video classification via ONNX (optional); supports OpenNSFW2 and NudeNet v2 simultaneously |
| Consecutive error guard | Stops automatically after 5 consecutive errors in the tweet fetch loop |
| Debug mode | Verbose internal logging via `-debug` flag or `config.yaml` |
| Duplicate execution guard | PID file prevents multiple simultaneous instances |
| Cookie auto-save | Cookie changes made by X.com during the session are written back to `cookies.json` automatically |

---

## How It Works

1. **First run** — if `cookies.json` does not exist, Chrome opens and navigates to the X.com login page. Log in manually. Once the `auth_token` cookie is detected, all cookies are saved to `cookies.json` and the program exits.
2. **Subsequent runs** — `cookies.json` is loaded, home timeline tweets are fetched, and media files are downloaded.
3. **Cookie auto-save** — after each session, the scraper's cookie jar is compared against the originally loaded values. If X.com refreshed any cookie (e.g. `ct0` which expires in ~6 hours), the changes are written back to `cookies.json` automatically.

To force re-login:

```bash
rm cookies.json && go run main.go
```

---

## Prerequisites

- **Go** 1.21+
- **CGo enabled** (`CGO_ENABLED=1`, the default)
- **Google Chrome** installed
- macOS or Linux (Windows untested)
- **ffmpeg** — required only for video NSFW detection
  - macOS: `brew install ffmpeg`
  - Ubuntu: `sudo apt install ffmpeg`

> **Cross-compilation is not supported.** The `onnxruntime_go` package uses CGo, so the binary must be built on the target machine.
> Apple Silicon Mac → `arm64` binary; Intel Mac → `amd64` binary.

---

## Build

```bash
# Build for the current OS/arch (recommended)
make

# Or manually
go build -o x-media-downloader .
```

Output binary: `x-media-downloader-{os}-{arch}` (e.g. `x-media-downloader-darwin-arm64`)

```bash
# Remove build artifacts
make clean
```

---

## Running

```bash
# Basic run
./x-media-downloader-darwin-arm64

# Specify tweet count (default: 100)
./x-media-downloader-darwin-arm64 -count 200

# Enable debug output
./x-media-downloader-darwin-arm64 -debug

# Custom config path
./x-media-downloader-darwin-arm64 -config /path/to/config.yaml
```

### CLI flags

| Flag | Description |
|------|-------------|
| `-count <n>` | Maximum number of tweets to collect (default: 100) |
| `-debug` | Enable verbose debug output |
| `-config <path>` | Path to `config.yaml` (default: executable directory, then CWD) |

---

## Configuration (`config.yaml`)

`config.yaml` is **optional**. If the file is not found, the program uses defaults:

| Setting | Default |
|---------|---------|
| `datadir` | `./data` |
| DB settings | DB connection skipped |
| NSFW settings | NSFW detection disabled |

Place `config.yaml` in the **same directory as the binary** (or in the current working directory when using `go run`) to override defaults.

### Full example

```yaml
# General
datadir: "/path/to/save"
debug: false

# Skip downloading media from promoted (ad) tweets (default: false)
exceptpromoted: false

# Database (optional)
dbhost: "127.0.0.1:3306"
dbuser: ""
dbpass: ""
dbdatabasename: ""

# NSFW detection (optional)
onnxlibpath: ""

opennsfw2modelpath:  "/path/to/nsfw_model.onnx"
opennsfw2inputname:  ""
opennsfw2outputname: ""

nudenetv2modelpath: "/path/to/detector_v2_default_checkpoint.onnx"
```

### Core settings

| Key | Default | Description |
|-----|---------|-------------|
| `datadir` | `./data` | Root path for downloaded media |
| `debug` | `false` | `true` to enable verbose debug output |
| `exceptpromoted` | `false` | `true` to skip media download for promoted (ad) tweets |

### MySQL download tracking (optional)

DB connection failure is non-fatal — the program continues with file-existence check only.

| Key | Description |
|-----|-------------|
| `dbhost` | MySQL host:port (e.g. `127.0.0.1:3306`) |
| `dbuser` | DB username |
| `dbpass` | DB password |
| `dbdatabasename` | Database name (auto-created if it does not exist) |

### NSFW detection (optional)

Two models are supported and can **run simultaneously** when both paths are set.

| Key | Description |
|-----|-------------|
| `onnxlibpath` | ONNX Runtime shared library path (empty = system default; shared by both models) |
| `opennsfw2modelpath` | Path to the OpenNSFW2 `.onnx` model file (empty = disabled) |
| `opennsfw2inputname` | Input tensor name (empty = auto-detected, fallback `"input"`) |
| `opennsfw2outputname` | Output tensor name (empty = auto-detected, fallback `"output"`) |
| `nudenetv2modelpath` | Path to `detector_v2_default_checkpoint.onnx` (empty = disabled) |

> **NudeNet v2 detection threshold** is fixed at `0.1` and is not configurable.

---

## NSFW Setup

### 1. Install ONNX Runtime

**macOS:**
```bash
brew install onnxruntime
```

**Ubuntu (CPU only):**
```bash
ONNX_VER="1.20.1"
wget https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VER}/onnxruntime-linux-x64-${ONNX_VER}.tgz
tar xzf onnxruntime-linux-x64-${ONNX_VER}.tgz
sudo cp onnxruntime-linux-x64-${ONNX_VER}/lib/libonnxruntime*.so* /usr/local/lib/
sudo ldconfig
```

**Ubuntu (GPU — CUDA required):**
```bash
ONNX_VER="1.20.1"
wget https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VER}/onnxruntime-linux-x64-gpu-${ONNX_VER}.tgz
tar xzf onnxruntime-linux-x64-gpu-${ONNX_VER}.tgz
sudo cp onnxruntime-linux-x64-gpu-${ONNX_VER}/lib/libonnxruntime*.so* /usr/local/lib/
sudo ldconfig
```

### 2. Prepare model files

**OpenNSFW2** — binary SFW/NSFW classifier:

```bash
pip install opennsfw2 tf2onnx tensorflow

python3 - <<'EOF'
import opennsfw2 as n2
import tf2onnx, tensorflow as tf

model = n2.make_open_nsfw_model()
input_spec = (tf.TensorSpec((None, 224, 224, 3), tf.float32, name="input"),)
tf2onnx.convert.from_keras(model, input_signature=input_spec,
                            output_path="nsfw_model.onnx")
print("Done: nsfw_model.onnx")
EOF
```

**NudeNet v2** — object detector for specific content classes:

Download [detector_v2_default_checkpoint.onnx](https://huggingface.co/gqfwqgw/NudeNet_classifier_model/tree/main)

Detectable classes: `FEMALE_BREAST_EXPOSED`, `FEMALE_GENITALIA_EXPOSED`, `MALE_GENITALIA_EXPOSED`, `ANUS_EXPOSED`, `BUTTOCKS_EXPOSED`, `FACE_FEMALE`, `MALE_BREAST_EXPOSED`, `FEET_EXPOSED`, `ARMPITS_EXPOSED`, `BELLY_EXPOSED`, `FEMALE_BREAST_COVERED`, `FEMALE_GENITALIA_COVERED`, `BUTTOCKS_COVERED`, `ANUS_COVERED`, `FACE_COVERED`, `FEET_COVERED`, `ARMPITS_COVERED`, `BELLY_COVERED`

### 3. Result format

Results are saved alongside each downloaded file as `{filename}.nsfwvalue.txt`.

When **both** models are enabled:
```
SFW:  0.9234
NSFW: 0.0766
NUDENET: FEMALE_BREAST_EXPOSED:0.8732 ANUS_EXPOSED:0.6123
```

When only **OpenNSFW2** is enabled:
```
SFW:  0.9234
NSFW: 0.0766
```

When only **NudeNet v2** is enabled:
```
NUDENET: FEMALE_BREAST_EXPOSED:0.8732
```

### GPU acceleration

| Platform | Order |
|----------|-------|
| macOS | CoreML (Apple Neural Engine) → CPU |
| Linux / Windows | CUDA (NVIDIA GPU) → CPU |

### NSFW execution conditions

| Condition | Action |
|-----------|--------|
| File newly downloaded | Run NSFW |
| File already existed (SKIP) | Skip NSFW |
| `.nsfwvalue.txt` already exists | Skip NSFW (reuse previous result) |
| NSFW disabled (no model path) | Always skip |

---

## Login and Authentication

The first run opens Chrome and navigates to the X.com login page. Log in manually. Cookies are saved to `cookies.json` next to the binary (or in CWD for `go run`).

```
binary directory/
├── x-media-downloader-darwin-arm64   ← binary
├── config.yaml                        ← create this
└── cookies.json                       ← auto-generated after login
```

> **Why manual login?**
> X.com uses [Castle.io](https://castle.io) bot detection. The `LoginEnterUserIdentifierSSO` step requires a `castle_token` generated from real browser signals that cannot be reproduced programmatically.
>
> | Approach | Result |
> |----------|--------|
> | Go HTTP client replicating the login API | Error 399 — `castle_token` required |
> | Hardcoding a captured `castle_token` | Token expires within minutes |
> | chromedp automated login | Castle.io detects headless Chrome |
> | **Current: chromedp + manual login** | **Works reliably** |

---

## Storage Layout

- **Date directory**: based on the **time of program execution** (`YYYY/MM/DD`)
- **Filename**: `{screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}` (tweet creation time)

```
{datadir}/
└── date/
    └── {YYYY}/
        └── {MM}/
            └── {DD}/
                ├── image/
                │   ├── {screenname}-{userid}-{tweetID}-{time}.jpg
                │   ├── {screenname}-{userid}-{tweetID}-{time}.jpg.nsfwvalue.txt
                │   └── retwitted/
                │       └── {screenname}-{userid}-{tweetID}-{time}.jpg
                ├── video/
                │   ├── {screenname}-{userid}-{tweetID}-{time}.mp4
                │   ├── {screenname}-{userid}-{tweetID}-{time}.mp4.nsfwvalue.txt
                │   └── retwitted/
                │       └── {screenname}-{userid}-{tweetID}-{time}.mp4
                └── text/
                    ├── {screenname}-{userid}-{tweetID}-{time}.txt
                    └── retwitted/
                        └── {screenname}-{userid}-{tweetID}-{time}.txt
```

- Multiple media per tweet: `{baseName}-0.jpg`, `{baseName}-1.jpg`, …
- Retweets: stored under `retwitted/` using the original author's info (self-retweets are stored in the standard directory)
- Twitter GIFs: saved as `.mp4` (Twitter API delivers GIFs as mp4)
- Video thumbnails: saved alongside images in `image/`

### Text file format (`.txt`)

```
ID: 1234567890
Username: @example
UserID: 9876543
Time: 2026-02-26 14:23:00
Retweeted by: @retweeter     ← only for retweets
---
[RT @example]                ← only for retweets
tweet body text
```

---

## MySQL Download Tracking (optional)

When DB is configured, downloaded files are recorded and duplicate downloads are prevented. The database and table are **created automatically** on first run.

```sql
CREATE TABLE downloaded_files (
  id         BIGINT        AUTO_INCREMENT PRIMARY KEY,
  http_url   VARCHAR(512)  NOT NULL DEFAULT '',
  file_path  VARCHAR(512)  NOT NULL DEFAULT '',
  tweet_id   VARCHAR(64)   NOT NULL,
  username   VARCHAR(128)  NOT NULL,
  user_id    VARCHAR(64)   NOT NULL,
  file_type  VARCHAR(16)   NOT NULL COMMENT 'image|video|text',
  is_retweet TINYINT(1)    NOT NULL DEFAULT 0,
  created_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_http_url (http_url)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**With DB connected:**

| URL in DB | File exists | Action |
|-----------|-------------|--------|
| No | — | Download → NSFW → DB record |
| Yes | Yes | Skip (`[SKIP]`) |
| Yes | No | Re-download → NSFW → DB record (`[RETRY]`) |

**Without DB:**

| File exists | Action |
|-------------|--------|
| No | Download → NSFW |
| Yes | Skip (`[SKIP]`) |

> DB record is written **only when both download and NSFW succeed**.

---

## Project Structure

```
.
├── main.go                          # Entry point: flags, init, login or fetch branch
│
├── config/config.go                 # Config struct and JSON loader (no username/password)
├── logger/logger.go                 # Simple debug logger (Enabled bool + Debug())
├── pidfile/pidfile.go               # Duplicate execution guard via PID file
├── loadcookies/cookies.go           # Cookie load, conversion, and sync utilities
│
├── cookieswithchromedp/
│   └── login.go                     # Opens Chrome, polls for auth_token, saves cookies.json
│
├── db/db.go                         # MySQL download tracking (optional)
│
├── nsfw/
│   ├── nsfw.go                      # Shared utils (frame extraction) + DetectAndSaveNSFW()
│   ├── opennsfw2.go                 # OpenNSFW2 binary classifier (Init, Close, detect*)
│   └── nudenetv2.go                 # NudeNet v2 object detector (InitNudeNet, CloseNudeNet, detect*)
│
├── processor/processor.go           # Tweet media downloader and file organizer
│
├── twitterscraper/                  # Local fork of imperatrona/twitter-scraper (modified)
│   ├── auth.go                      # SetCookies auto-sets isLogged + bearerToken1
│   ├── tweets.go                    # GetHomeTweets, GetForYouTweets, etc.
│   ├── likes.go                     # GetLikes, FetchLikes — liked tweets via GraphQL Likes endpoint
│   ├── timeline_v2.go               # timelineV2, likesTimelineV2, bookmarksTimelineV2, parseTweets()
│   └── ...
│
├── examples/                        # Runnable usage examples for each package
│   ├── config/main.go
│   ├── db/main.go
│   ├── loadcookies/main.go
│   ├── logger/main.go
│   ├── nsfw/main.go
│   ├── pidfile/main.go
│   └── processor/main.go
│
├── cookies.json                     # Auto-generated after login (keep out of version control)
├── config.yaml                      # Create this manually
├── Makefile
├── go.mod
└── go.sum
```

### Package dependency graph

```
main
 ├── config       (YAML config loader)
 ├── logger       (debug output)
 ├── pidfile      → logger
 ├── loadcookies  (cookie file I/O)
 ├── cookieswithchromedp  (Chrome login)
 ├── db           → logger
 ├── nsfw         → logger  (opennsfw2.go + nudenetv2.go + nsfw.go)
 ├── processor    → db, nsfw, logger, twitterscraper
 └── twitterscraper
```

### Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/chromedp/chromedp` | v0.14.2 | Chrome control via CDP for manual login |
| `github.com/chromedp/cdproto` | — | CDP types and commands |
| `github.com/go-sql-driver/mysql` | v1.9.3 | MySQL driver |
| `github.com/yalue/onnxruntime_go` | v1.27.0 | ONNX Runtime Go bindings (CGo) |
| `golang.org/x/net` | v0.51.0 | SOCKS5 proxy support in twitterscraper |

---

## Cookie Schema (`cookies.json`)

```json
[
  {
    "name": "auth_token",
    "value": "...",
    "domain": ".x.com",
    "path": "/",
    "expires": 1807228866.0,
    "httpOnly": true,
    "secure": true,
    "sameSite": "None"
  }
]
```

Key cookies:

| Cookie | Lifetime | Purpose |
|--------|----------|---------|
| `auth_token` | ~1 year | User authentication — primary credential |
| `ct0` | ~6 hours | CSRF token sent as `X-CSRF-Token` header |
| `twid` | ~1 year | User ID (value contains quotes: `"u=12345"`) |

> **Cookie value sanitization:** some X.com cookies (e.g. `twid`, `g_state`) contain `"` characters.
> Go's `net/http` silently drops these bytes. They are stripped via `strings.ReplaceAll` before
> cookies are stored in the jar.

---

## Design Notes

### twitterscraper modifications

The local `twitterscraper/` fork modifies `auth.go`'s `SetCookies`:

```go
func (s *Scraper) SetCookies(cookies []*http.Cookie) {
    s.client.Jar.SetCookies(twURL, cookies)
    for _, c := range cookies {
        if c.Name == "auth_token" && c.Value != "" {
            s.isLogged = true
            s.setBearerToken(bearerToken1)  // ← added
            break
        }
    }
}
```

When `auth_token` is found, two fields are set automatically:
- `s.isLogged = true` — so `prepareRequest` uses cookie auth instead of guest token
- `s.setBearerToken(bearerToken1)` — switches to the authenticated app bearer token; without this, GraphQL requests return HTTP 401

### Why poll `auth_token` instead of watching the DOM or URL?

After login, X.com redirects to different paths depending on account state (2FA, phone verification, etc.). Polling for the `auth_token` cookie every second is the most reliable universal indicator of successful login.

### Promoted tweet detection (`promotedMetadata`)

Promoted (ad) tweets in the HomeTimeline response include a `promotedMetadata` field at the `itemContent` level — **not** inside `tweet_results.result`.

```
itemContent {
  __typename: "TimelineTweet"
  promotedMetadata: { ... }   ← only present for ad tweets
  tweet_results: { result: { ... } }
}
```

Two fields are added to the `Tweet` struct (`twitterscraper/types.go`):

| Field | Type | Description |
|-------|------|-------------|
| `IsPromoted` | `bool` | `true` when the tweet is a promoted/ad tweet |
| `PromotedMetadata` | `*PromotedMetadata` | `nil` for organic tweets; populated for ad tweets |

`PromotedMetadata` sub-fields:

| Field | Type | Description |
|-------|------|-------------|
| `AdMetadataContainer` | struct | `isQuickPromote`, `renderLegacyWebsiteCard`, `renderSalesCtaWebsiteCard` |
| `AdvertiserResults.Result` | struct | Advertiser user info: `id`, `rest_id`, `is_blue_verified`, `core.name`, `core.screen_name` |
| `ClickTrackingInfo.URLParams` | `[]struct{Key, Value}` | Click tracking params (e.g. `twclid`) |
| `DisclosureType` | string | e.g. `"NoDisclosure"` |
| `ExperimentValues` | `[]struct{Key, Value}` | A/B test experiment keys and values |
| `ImpressionID` | string | Unique impression identifier |

`parseTweets()` (`twitterscraper/tweets.go`) also handles `TweetWithVisibilityResults` typename in addition to `"Tweet"`.

#### Skipping promoted tweet downloads

Set `exceptpromoted: true` in `config.yaml` to skip media downloads for promoted tweets:

```yaml
exceptpromoted: true
```

When a promoted tweet is encountered, the output shows `[SKIP] promoted tweet` and the tweet is counted but its media is not downloaded. The `Tweet.IsPromoted` field is always populated regardless of this setting — it is the download step that is skipped, not the detection.

### Likes API (`twitterscraper/likes.go`)

The Likes endpoint returns tweets the authenticated user has liked.

```go
for tweet := range scraper.GetLikes(ctx, 100) {
    // tweet.Tweet, tweet.Err
}
```

| Item | Detail |
|------|--------|
| Endpoint | `GET https://x.com/i/api/graphql/j-O2fOmYBTqofGfn6LMb8g/Likes` |
| Response path | `data.user.result.`**`timeline`**`.timeline.instructions` |
| Standard timeline path | `data.user.result.`**`timeline_v2`**`.timeline.instructions` |
| `userId` source | Extracted automatically from the `twid` cookie (`u%3D12345` → `12345`) |
| Response struct | `likesTimelineV2` (defined in `timeline_v2.go`) |

> **Note 1:** The Likes API uses `timeline` as the JSON key, not `timeline_v2`. A dedicated `likesTimelineV2` struct is used instead of the shared `timelineV2` to correctly map this path.

> **Note 2 — user schema difference:** The Likes API places `screen_name` and `name` inside `result.core` instead of `result.legacy`. Other endpoints use `result.legacy.screen_name`. `result.parse()` in `timeline_v2.go` handles this with a fallback: if `legacy.ScreenName` is empty, it reads from `Core.ScreenName`.

---

## Security Notes

### Credential files

- `cookies.json` contains your X.com session token (`auth_token`). **Add it to `.gitignore`** and never commit it.
  - Saved with `0600` permissions (owner read/write only).
- `config.yaml` may contain MySQL credentials. Keep it out of version control as well.

### Known issues and mitigations

| Severity | Location | Issue | Mitigation |
|----------|----------|-------|------------|
| 🔴 Medium | `processor/processor.go` | **No HTTP response size limit** — `io.Copy` without `LimitReader`; an oversized response could exhaust disk space | Add `io.LimitReader(resp.Body, maxSize)` |
| 🔴 Medium | `processor/processor.go` | **No HTTP client timeout** — `http.Get` uses the default client; a hung server blocks indefinitely | Use `&http.Client{Timeout: 60*time.Second}` |
| 🟡 Low | `main.go` | **`consecErrors` exceeded returns no error** — loop breaks but `fetchTweets` returns `nil`; caller cannot distinguish normal exit from error-stop | Return a sentinel error after `maxConsecErrors` |
| 🟡 Low | `processor/processor.go` | **`photoExt` extension not validated** — `format` query param used as file extension with no allowlist | Restrict to `jpg`, `jpeg`, `png`, `gif`, `webp`, `mp4` |
| 🟡 Low | `db/db.go` | **No DB connection pool settings** — `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` not configured | Set reasonable pool limits after `Open` |
| 🟡 Low | `cookieswithchromedp/login.go` | **`--no-sandbox` Chrome flag** — disables browser sandboxing | Remove if the environment supports sandboxing |
| 🟡 Low | `pidfile/pidfile.go` | **PID file permissions `0644`** — readable by all local users | Change to `0600` |
| 🟢 Info | `db/db.go` | **`SELECT COUNT(*)` for URL tracking** — slightly wasteful | Replace with `SELECT 1 FROM ... WHERE ... LIMIT 1` |
| 🟢 Info | `db/db.go` | **No TLS for MySQL** — plaintext on remote hosts | Add `tls=skip-verify` or configure CA for remote DB |

### Items verified as safe

| Item | Status | Reason |
|------|--------|--------|
| SQL injection | ✅ Safe | All queries use `?` parameterized placeholders |
| Shell injection (ffmpeg/ffprobe) | ✅ Safe | `exec.Command` receives arguments as separate strings, not via a shell |
| `cookies.json` file permissions | ✅ Safe | Written with `0600` in both `cookieswithchromedp` and `loadcookies` |
| DB password in logs | ✅ Safe | DSN is logged with password replaced by `***` |
| Path traversal via username | ✅ Safe in practice | Twitter usernames are restricted to `[A-Za-z0-9_]` |

# x-media-downloader

A Go CLI tool that logs into X.com (formerly Twitter) via a real Chrome browser, saves session cookies, and automatically collects and downloads media (images, videos, GIFs) from your home timeline.

---

## Features

| Feature | Description |
|---------|-------------|
| Home timeline collection | Fetches tweets automatically (`-count` flag controls the limit) |
| Media download | Downloads images, videos, and GIFs; video thumbnails are saved to `image/` |
| Retweet handling | Stored under `retwitted/` subdirectory using the original author's info |
| Date-based directories | Auto-organized as `date/YYYY/MM/DD/` |
| Duplicate prevention | MySQL DB-based download tracking (optional); falls back to file-existence check |
| NSFW detection | GPU-accelerated image/video classification via ONNX model (optional; ffmpeg required for video) |
| Consecutive error guard | Stops automatically after 5 consecutive errors in the tweet fetch loop |
| Debug mode | Verbose internal logging via `-debug` flag or `config.json` |
| Duplicate execution guard | PID file prevents multiple simultaneous instances |
| Cookie auto-save | Cookie changes made by X.com during the session are written back to `cookies.json` automatically |

---

## How It Works

1. **First run** — if `cookies.json` does not exist, Chrome opens automatically and navigates to the X.com login page. Log in manually. Once the `auth_token` cookie is detected, all cookies are saved to `cookies.json` and the program exits.
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

## Configuration (`config.json`)

`config.json` is **optional**. If the file is not found (and no `-config` path was given), the program starts with sensible defaults:

| Setting | Default |
|---------|---------|
| `datadir` | `./data` (relative to working directory) |
| DB settings | DB connection skipped |
| NSFW settings | NSFW detection disabled |

Place `config.json` in the **same directory as the binary** (or in the current working directory when using `go run`) to override any of these defaults.

```json
{
  "datadir":  "/path/to/save",
  "debug":    false,

  "dbhost":         "",
  "dbuser":         "",
  "dbpass":         "",
  "dbdatabasename": "",

  "nsfwmodelpath":  "",
  "onnxlibpath":    "",
  "nsfwinputname":  "",
  "nsfwoutputname": ""
}
```

### Core settings

| Key | Description |
|-----|-------------|
| `datadir` | Root path for downloaded media (default: `./data` if empty) |
| `debug` | Set to `true` to enable verbose debug output |

### Optional — MySQL download tracking

If not configured, the program runs without DB tracking. DB connection failure is non-fatal (continues with file-existence check only).

| Key | Description |
|-----|-------------|
| `dbhost` | MySQL host:port (e.g. `127.0.0.1:3306`) |
| `dbuser` | DB username |
| `dbpass` | DB password |
| `dbdatabasename` | Database name (auto-created if it does not exist) |

### Optional — NSFW detection

If `nsfwmodelpath` is empty, NSFW detection is disabled.

| Key | Description |
|-----|-------------|
| `nsfwmodelpath` | Path to the ONNX model file |
| `onnxlibpath` | ONNX Runtime shared library path (empty = system default) |
| `nsfwinputname` | Model input tensor name (empty = auto-detected, fallback `"input"`) |
| `nsfwoutputname` | Model output tensor name (empty = auto-detected, fallback `"output"`) |

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
./x-media-downloader-darwin-arm64 -config /path/to/config.json

# Help
./x-media-downloader-darwin-arm64 -help
```

### CLI flags

| Flag | Description |
|------|-------------|
| `-count <n>` | Maximum number of tweets to collect (default: 100) |
| `-debug` | Enable verbose debug output |
| `-config <path>` | Path to `config.json` (default: executable directory, then CWD) |

### Debug output format

```
[DEBUG 14:23:01.123] message
```

| Package | What it logs |
|---------|--------------|
| `main` | Flag values, config summary, per-tweet media count and type, processing time |
| `pidfile` | PID file path, signal(0) result for process existence check |
| `db` | DSN (password masked as `***`), ping result, each SQL query and record |
| `nsfw` | Model load, GPU provider selection, image dimensions, raw inference values, elapsed time |
| `processor` | URL → file path, HTTP status, file size, download elapsed time |

---

## Login and Authentication

The first run opens Chrome and navigates to the X.com login page. Log in manually with your account. Once authenticated, cookies are saved to `cookies.json` next to the binary (or in CWD for `go run`).

```
binary directory/
├── x-media-downloader-darwin-arm64   ← binary
├── config.json                        ← create this
└── cookies.json                       ← auto-generated after login
```

> **Why manual login?**
> X.com uses [Castle.io](https://castle.io) bot detection. The `LoginEnterUserIdentifierSSO` step requires a `castle_token` generated from real browser signals (mouse movement, timing, Canvas fingerprint) that cannot be reproduced programmatically.
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
                │   ├── {screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}.jpg
                │   ├── {screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}.jpg.nsfwvalue.txt
                │   └── retwitted/
                │       └── {screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}.jpg
                ├── video/
                │   ├── {screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}.mp4
                │   ├── {screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}.mp4.nsfwvalue.txt
                │   └── retwitted/
                │       └── {screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}.mp4
                └── text/
                    ├── {screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}.txt
                    └── retwitted/
                        └── {screenname}-{userid}-{tweetID}-{YYYYMMDD_HHMMSS}.txt
```

- Multiple media per tweet: `{baseName}-0.jpg`, `{baseName}-1.jpg`, …
- Retweets: stored under `retwitted/` using the original author's info
- Twitter GIFs: saved as `.mp4` (Twitter API delivers GIFs as mp4)
- Video thumbnails: saved alongside images in `image/`
- NSFW results: `{filename}.nsfwvalue.txt`

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

## Optional Features

### MySQL download tracking

When DB is configured, downloaded files are recorded and duplicate downloads are prevented.

The database and table are **created automatically** on first run.

```sql
CREATE TABLE downloaded_files (
  id         BIGINT        AUTO_INCREMENT PRIMARY KEY,
  http_url   VARCHAR(512)  NOT NULL DEFAULT '',   -- original download URL (dedup key)
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
> DB connection failure logs a warning and the program continues without tracking.

---

### NSFW detection

Runs ONNX-based NSFW classification on downloaded images and videos.

**NSFW execution conditions:**

| Condition | Action |
|-----------|--------|
| File newly downloaded | Run NSFW |
| File already existed (SKIP) | Skip NSFW |
| `.nsfwvalue.txt` already exists | Skip NSFW (reuse previous result) |
| NSFW disabled (no model path) | Always skip |

**GPU priority:**

| Platform | Order |
|----------|-------|
| macOS | CoreML (Apple Neural Engine) → CPU |
| Linux / Windows | CUDA (NVIDIA GPU) → CPU |

#### 1. Install ONNX Runtime

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

#### 2. Prepare the NSFW ONNX model (opennsfw2)

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

#### 3. Add to config.json

```json
{
  "nsfwmodelpath":  "/path/to/nsfw_model.onnx",
  "onnxlibpath":    "",
  "nsfwinputname":  "",
  "nsfwoutputname": ""
}
```

> Leaving `nsfwinputname` / `nsfwoutputname` empty triggers auto-detection from the model.
> Falls back to `"input"` / `"output"` if detection fails.

**NSFW result file format:**
```
SFW:  0.9234
NSFW: 0.0766
```

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
├── nsfw/nsfw.go                     # ONNX Runtime NSFW classifier (optional)
├── processor/processor.go           # Tweet media downloader and file organizer
│
├── twitterscraper/                  # Local fork of imperatrona/twitter-scraper (modified)
│   ├── auth.go                      # SetCookies auto-sets isLogged + bearerToken1
│   ├── tweets.go                    # GetHomeTweets, GetForYouTweets, etc.
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
├── config.json                      # Create this manually
├── Makefile
├── go.mod
└── go.sum
```

### Package dependency graph

```
main
 ├── config       (JSON config loader)
 ├── logger       (debug output)
 ├── pidfile      → logger
 ├── loadcookies  (cookie file I/O)
 ├── cookieswithchromedp  (Chrome login)
 ├── db           → logger
 ├── nsfw         → logger
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

---

## Known Limitations

- **Cross-compilation not supported** — `onnxruntime_go` uses CGo; build on the target machine.
- **NSFW model not included** — prepare and provide the `.onnx` model file separately.
- **Video NSFW requires ffmpeg** — if not installed, video NSFW detection is skipped (non-fatal).
- **Twitter GIFs are mp4** — the Twitter API delivers GIFs as mp4, so they are saved with `.mp4`.

---

## Security Notes

### Credential files

- `cookies.json` contains your X.com session token (`auth_token`). **Add it to `.gitignore`** and never commit it.
  - Saved with `0600` permissions (owner read/write only).
- `config.json` may contain MySQL credentials. Keep it out of version control as well.

### Known issues and mitigations

| Severity | Location | Issue | Mitigation |
|----------|----------|-------|------------|
| 🔴 Medium | `processor/processor.go` | **No HTTP response size limit** — `io.Copy` reads the full response body without a cap; a malformed or oversized response could exhaust disk space | Add `io.LimitReader(resp.Body, maxSize)` |
| 🔴 Medium | `processor/processor.go` | **No HTTP client timeout** — `http.Get` uses the default client with no timeout; a slow or hung server blocks indefinitely | Use `&http.Client{Timeout: 60*time.Second}` |
| 🟡 Low | `processor/processor.go` | **`photoExt` extension not validated** — the `format` query param from the URL is used as-is for the file extension with no allowlist check | Restrict to `jpg`, `jpeg`, `png`, `gif`, `webp`, `mp4` |
| 🟡 Low | `cookieswithchromedp/login.go` | **`--no-sandbox` Chrome flag** — disables browser sandboxing; increases attack surface if a malicious page loads during login | Remove if the environment supports sandboxing (test on macOS first) |
| 🟡 Low | `pidfile/pidfile.go` | **PID file permissions `0644`** — readable by all local users; PID exposure is minimal risk but inconsistent with the `0600` used for `cookies.json` | Change to `0600` |
| 🟢 Info | `pidfile/pidfile.go` | **TOCTOU race condition** — small window between reading and writing the PID file where a second instance could slip through | Acceptable for a personal single-user tool |

### Items verified as safe

| Item | Status | Reason |
|------|--------|--------|
| SQL injection | ✅ Safe | All queries use `?` parameterized placeholders |
| Shell injection (ffmpeg/ffprobe) | ✅ Safe | `exec.Command` receives arguments as separate strings, not via a shell |
| `cookies.json` file permissions | ✅ Safe | Written with `0600` in both `cookieswithchromedp` and `loadcookies` |
| DB password in logs | ✅ Safe | DSN is logged with password replaced by `***` |
| Path traversal via username | ✅ Safe in practice | Twitter usernames are restricted to `[A-Za-z0-9_]`; `/` and `..` are not allowed |

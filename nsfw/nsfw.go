// Package nsfw provides ONNX-based NSFW detection for images and videos.
//
// Two independent detectors are supported and can run simultaneously:
//   - OpenNSFW2  (opennsfw2.go): binary SFW/NSFW classifier
//   - NudeNet v2 (nudenetv2.go): object-level nude content detector
//
// Enable each by setting the corresponding model path in config.yaml.
// If both are configured, both run on every file.
//
// Result file: {original file}.nsfwvalue.txt
//
//	SFW:  0.9234                                        ← OpenNSFW2  (omitted when disabled)
//	NSFW: 0.0766
//	NUDENET: FEMALE_BREAST_EXPOSED:0.8732 ANUS_EXPOSED:0.6123  ← NudeNet v2 (omitted when disabled)
package nsfw

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kinkist/x-media-downloader/logger"
)

const videoFrameCount = 5

// videoExts contains file extensions recognized as video.
var videoExts = map[string]bool{
	".mp4": true, ".webm": true, ".mov": true,
	".avi": true, ".mkv": true,
}

// isVideoFile reports whether path has a recognized video extension.
func isVideoFile(path string) bool {
	return videoExts[strings.ToLower(filepath.Ext(path))]
}

// getVideoDuration returns the video duration in seconds via ffprobe. Returns 0 on failure.
func getVideoDuration(videoPath string) float64 {
	out, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	).Output()
	if err != nil {
		return 0
	}
	dur, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return dur
}

// extractVideoFrames extracts up to videoFrameCount frames from a video using ffmpeg.
// Returns the extracted frame paths and a cleanup function. Caller must call cleanup().
func extractVideoFrames(videoPath string) (frames []string, cleanup func(), err error) {
	if _, lookErr := exec.LookPath("ffmpeg"); lookErr != nil {
		return nil, nil, fmt.Errorf("ffmpeg not found (please install it): %w", lookErr)
	}

	tmpDir, err := os.MkdirTemp("", "nsfw_frames_*")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	cleanup = func() { os.RemoveAll(tmpDir) }

	dur := getVideoDuration(videoPath)
	n := videoFrameCount
	if dur > 0 && dur < float64(n) {
		n = max(1, int(dur))
	}
	baseDur := dur
	if baseDur <= 0 {
		baseDur = 1
	}
	fpsExpr := fmt.Sprintf("%.6f", float64(n)/baseDur)
	outPattern := filepath.Join(tmpDir, "frame%03d.jpg")

	logger.Debug("extracting video frames: %s (duration=%.1fs, n=%d, fps=%s)",
		filepath.Base(videoPath), dur, n, fpsExpr)

	out, runErr := exec.Command("ffmpeg",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=%s", fpsExpr),
		"-frames:v", strconv.Itoa(n),
		"-y", outPattern,
	).CombinedOutput()
	if runErr != nil {
		cleanup()
		return nil, nil, fmt.Errorf("ffmpeg frame extraction failed: %w\n%s", runErr, string(out))
	}

	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil || len(entries) == 0 {
		cleanup()
		return nil, nil, fmt.Errorf("no frames extracted")
	}
	for _, e := range entries {
		if !e.IsDir() {
			frames = append(frames, filepath.Join(tmpDir, e.Name()))
		}
	}
	logger.Debug("frame extraction complete: %d frames", len(frames))
	return frames, cleanup, nil
}

// DetectAndSaveNSFW runs all enabled NSFW detectors on filePath and writes
// results to {filePath}.nsfwvalue.txt.
//
// Videos (.mp4, .webm, etc.) are processed by extracting frames with ffmpeg.
// Both OpenNSFW2 and NudeNet v2 can run simultaneously when both are enabled.
// Returns true when at least one detector ran and all active detectors succeeded.
func DetectAndSaveNSFW(filePath string) bool {
	if !enabled && !nudeEnabled {
		logger.Debug("all NSFW detectors disabled, skipping: %s", filepath.Base(filePath))
		return true
	}

	var lines []string
	allOK := true

	// ── OpenNSFW2 ─────────────────────────────────────────────────────────────
	if enabled {
		var sfw, nsfwVal float32
		var err error
		if isVideoFile(filePath) {
			sfw, nsfwVal, err = detectVideoOpenNSFW2(filePath)
		} else {
			sfw, nsfwVal, err = detectOpenNSFW2(filePath)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [WARN] OpenNSFW2 detection failed (%s): %v\n",
				filepath.Base(filePath), err)
			allOK = false
		} else {
			lines = append(lines,
				fmt.Sprintf("SFW:  %.4f", sfw),
				fmt.Sprintf("NSFW: %.4f", nsfwVal),
			)
			fmt.Printf("  [OpenNSFW2] %s → SFW=%.4f NSFW=%.4f\n",
				filepath.Base(filePath), sfw, nsfwVal)
		}
	}

	// ── NudeNet v2 ────────────────────────────────────────────────────────────
	if nudeEnabled {
		var dets []NudeDetection
		var err error
		if isVideoFile(filePath) {
			dets, err = detectVideoNudeNet(filePath)
		} else {
			dets, err = detectNudeNet(filePath)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [WARN] NudeNet v2 detection failed (%s): %v\n",
				filepath.Base(filePath), err)
			allOK = false
		} else if len(dets) == 0 {
			lines = append(lines, "NUDENET: (none above threshold)")
		} else {
			line := "NUDENET:"
			fmt.Printf("  [NudeNet v2] %s →", filepath.Base(filePath))
			for _, d := range dets {
				line += fmt.Sprintf(" %s:%.4f", d.Class, d.Score)
				fmt.Printf(" %s:%.4f", d.Class, d.Score)
			}
			fmt.Println()
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return false
	}

	content := strings.Join(lines, "\n") + "\n"
	txtPath := filePath + ".nsfwvalue.txt"
	if err := os.WriteFile(txtPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "  [WARN] failed to save NSFW result (%s): %v\n",
			filepath.Base(filePath), err)
		return false
	}
	logger.Debug("NSFW result saved: %s", txtPath)
	return allOK
}

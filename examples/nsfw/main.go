// examples/nsfw: usage example for the nsfw package
//
// Demonstrates running OpenNSFW2 and/or NudeNet v2 on an image or video file.
// Both detectors can run simultaneously when both model paths are set in config.yaml.
//
// Prerequisites:
//   - At least one ONNX model file (opennsfw2modelpath or nudenetv2modelpath in config.yaml)
//   - libonnxruntime.dylib (macOS) or libonnxruntime.so (Linux) required
//   - ffmpeg must be installed for video NSFW detection
//
// Usage:
//
//	go run ./examples/nsfw -image /path/to/image.jpg
//	go run ./examples/nsfw -image /path/to/video.mp4 -debug
//	go run ./examples/nsfw -config /path/to/config.yaml -image /path/to/image.jpg
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kinkist/x-media-downloader/config"
	"github.com/kinkist/x-media-downloader/logger"
	"github.com/kinkist/x-media-downloader/nsfw"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml")
	imagePath := flag.String("image", "", "image or video file path to inspect (required)")
	debugFlag := flag.Bool("debug", false, "enable debug output")
	flag.Parse()

	if *imagePath == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/nsfw -image <filepath>")
		os.Exit(1)
	}

	logger.Enabled = *debugFlag

	// ── 1. load model paths from config ───────────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(1)
	}
	if cfg.Opennsfw2modelpath == "" && cfg.Nudenetv2modelpath == "" {
		fmt.Fprintln(os.Stderr, "config.yaml must have opennsfw2modelpath and/or nudenetv2modelpath set")
		os.Exit(1)
	}

	// ── 2. nsfw.Init() — initialize OpenNSFW2 (optional) ─────────
	// Execution device priority:
	//   macOS → CoreML (Apple Neural Engine) → CPU
	//   Other → CUDA (NVIDIA GPU)            → CPU
	//
	// Parameters:
	//   modelPath  : path to .onnx model file
	//   libPath    : ONNX Runtime library path (empty string = system default)
	//   inName     : input tensor name (empty string = auto-detect)
	//   outName    : output tensor name (empty string = auto-detect)
	nsfwAny := false
	if cfg.Opennsfw2modelpath != "" {
		if err := nsfw.Init(
			cfg.Opennsfw2modelpath,
			cfg.Onnxlibpath,
			cfg.Opennsfw2inputname,
			cfg.Opennsfw2outputname,
		); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] OpenNSFW2 initialization failed: %v\n", err)
		} else {
			defer nsfw.Close()
			nsfwAny = true
		}
	}

	// ── 3. nsfw.InitNudeNet() — initialize NudeNet v2 (optional) ─
	// Detection threshold is fixed at 0.6 (not configurable).
	// Can run alongside OpenNSFW2 when both are initialized.
	if cfg.Nudenetv2modelpath != "" {
		if err := nsfw.InitNudeNet(
			cfg.Nudenetv2modelpath,
			cfg.Onnxlibpath,
		); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] NudeNet v2 initialization failed: %v\n", err)
		} else {
			defer nsfw.CloseNudeNet()
			nsfwAny = true
		}
	}

	if !nsfwAny {
		fmt.Fprintln(os.Stderr, "no NSFW detector initialized successfully")
		os.Exit(1)
	}

	// ── 4. DetectAndSaveNSFW() — inspect file + save result ──────
	// Image: direct inference
	// Video: frames extracted via ffmpeg, highest per-frame score used
	//
	// Result file: {original file}.nsfwvalue.txt
	//   SFW:  0.9234                                  ← OpenNSFW2 (if enabled)
	//   NSFW: 0.0766
	//   NUDENET: FEMALE_BREAST_EXPOSED:0.8732          ← NudeNet v2 (if enabled)
	//
	// Returns: true=success or disabled, false=error occurred
	fmt.Printf("inspecting: %s\n", *imagePath)
	ok := nsfw.DetectAndSaveNSFW(*imagePath)
	if !ok {
		fmt.Fprintln(os.Stderr, "NSFW detection failed (check logs)")
		os.Exit(1)
	}

	resultPath := *imagePath + ".nsfwvalue.txt"
	if data, err := os.ReadFile(resultPath); err == nil {
		fmt.Printf("\nresult file (%s):\n%s", resultPath, string(data))
	}
}

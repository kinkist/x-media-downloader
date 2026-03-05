// examples/nsfw: usage example for the nsfw package
//
// Prerequisites:
//   - nsfw_model.onnx file is required (see nsfwmodelpath in config.json)
//   - libonnxruntime.dylib (macOS) or libonnxruntime.so (Linux) required
//   - ffmpeg must be installed for video NSFW detection
//
// Usage:
//
//	go run ./examples/nsfw -image /path/to/image.jpg
//	go run ./examples/nsfw -image /path/to/video.mp4 -debug
//	go run ./examples/nsfw -config /path/to/config.json -image /path/to/image.jpg
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
	configPath := flag.String("config", "", "path to config.json")
	imagePath := flag.String("image", "", "image or video file path to inspect (required)")
	debugFlag := flag.Bool("debug", false, "enable debug output")
	flag.Parse()

	if *imagePath == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/nsfw -image <filepath>")
		os.Exit(1)
	}

	logger.Enabled = *debugFlag

	// ── 1. load NSFW model path from config ───────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(1)
	}
	if cfg.Nsfwmodelpath == "" {
		fmt.Fprintln(os.Stderr, "config.json must have nsfwmodelpath set")
		os.Exit(1)
	}

	// ── 2. nsfw.Init() — initialize ONNX session ─────────────────
	// Execution device priority:
	//   macOS → CoreML (Apple Neural Engine) → CPU
	//   Other → CUDA (NVIDIA GPU)            → CPU
	//
	// Parameters:
	//   modelPath  : path to .onnx model file
	//   libPath    : ONNX Runtime library path (empty string = system default)
	//   inName     : input tensor name (empty string = auto-detect)
	//   outName    : output tensor name (empty string = auto-detect)
	if err := nsfw.Init(
		cfg.Nsfwmodelpath,
		cfg.Onnxlibpath,
		cfg.Nsfwinputname,
		cfg.Nsfwoutputname,
	); err != nil {
		fmt.Fprintln(os.Stderr, "NSFW initialization failed:", err)
		os.Exit(1)
	}
	// ── 3. nsfw.Close() — always defer resource release ──────────
	defer nsfw.Close()

	// ── 4. DetectAndSaveNSFW() — inspect file + save result ──────
	// Image: direct inference
	// Video: frames extracted via ffmpeg, highest per-frame NSFW score used
	//
	// Result file: {original file}.nsfwvalue.txt
	//   SFW:  0.9234
	//   NSFW: 0.0766
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

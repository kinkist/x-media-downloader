// Package nsfw provides ONNX Runtime-based NSFW image/video classification.
//
// Execution device priority:
//
//	macOS  → CoreML (Apple Neural Engine / GPU) → CPU
//	Other  → CUDA (NVIDIA GPU)                  → CPU
//
// Model: OpenNSFW-compatible ONNX
//
//	Input  : [1, 224, 224, 3] float32 (NHWC, ImageNet mean subtracted)
//	Output : [1, 2] float32 → [SFW probability, NSFW probability]
//
// Video: frames are extracted with ffmpeg and the maximum per-frame NSFW score is used.
//
// Result file: {original file}.nsfwvalue.txt
//
//	SFW:  0.9234
//	NSFW: 0.0766
package nsfw

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kinkist/x-media-downloader/logger"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	imgSize = 224
	// ImageNet channel mean (RGB order)
	meanR = float32(122.7717)
	meanG = float32(115.6290)
	meanB = float32(100.4038)
	// maximum number of frames to extract from a video
	videoFrameCount = 5
)

var (
	session    *ort.AdvancedSession
	inTensor   *ort.Tensor[float32]
	outTensor  *ort.Tensor[float32]
	enabled    bool
	inputName  = "input"
	outputName = "output"

	// file extensions recognized as video
	videoExts = map[string]bool{
		".mp4": true, ".webm": true, ".mov": true,
		".avi": true, ".mkv": true,
	}
)

// Init initializes the ONNX NSFW detector.
// On failure it remains in enabled=false state and detection is skipped (non-fatal).
//
//	modelPath  : path to the .onnx model file
//	libPath    : ONNX Runtime shared library path (empty = system default)
//	inName     : ONNX input tensor name (empty = "input")
//	outName    : ONNX output tensor name (empty = "output")
func Init(modelPath, libPath, inName, outName string) error {
	logger.Debug("NSFW Init — model=%s lib=%q inputName=%q outputName=%q",
		modelPath, libPath, inName, outName)

	if modelPath == "" {
		return fmt.Errorf("NSFW model path not set")
	}
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("NSFW model file not found: %s", modelPath)
	}
	logger.Debug("model file confirmed: %s", modelPath)

	if libPath != "" {
		logger.Debug("setting ONNX Runtime library path: %s", libPath)
		ort.SetSharedLibraryPath(libPath)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("failed to initialize ONNX Runtime: %w", err)
	}
	logger.Debug("ONNX Runtime environment initialized")

	// apply config-specified values first
	if inName != "" {
		inputName = inName
	}
	if outName != "" {
		outputName = outName
	}

	// auto-detect unspecified names from the model
	if inName == "" || outName == "" {
		infInputs, infOutputs, infErr := ort.GetInputOutputInfo(modelPath)
		if infErr != nil {
			logger.Debug("auto-detect tensor names failed (%v), using defaults", infErr)
		} else {
			if inName == "" && len(infInputs) > 0 {
				inputName = infInputs[0].Name
				logger.Debug("auto-detected input tensor name: %q", inputName)
			}
			if outName == "" && len(infOutputs) > 0 {
				outputName = infOutputs[0].Name
				logger.Debug("auto-detected output tensor name: %q", outputName)
			}
		}
	}

	logger.Debug("tensor names — input=%q output=%q", inputName, outputName)

	// pre-allocate input/output tensors (reused on each inference)
	var err error
	inTensor, err = ort.NewEmptyTensor[float32](
		ort.NewShape(1, imgSize, imgSize, 3))
	if err != nil {
		return fmt.Errorf("failed to create input tensor: %w", err)
	}
	logger.Debug("input tensor created: shape=[1,%d,%d,3]", imgSize, imgSize)

	outTensor, err = ort.NewEmptyTensor[float32](ort.NewShape(1, 2))
	if err != nil {
		inTensor.Destroy()
		inTensor = nil
		return fmt.Errorf("failed to create output tensor: %w", err)
	}
	logger.Debug("output tensor created: shape=[1,2]")

	// create session options and attempt to add GPU provider
	opts, err := ort.NewSessionOptions()
	if err != nil {
		inTensor.Destroy()
		outTensor.Destroy()
		inTensor, outTensor = nil, nil
		return fmt.Errorf("failed to create session options: %w", err)
	}
	defer opts.Destroy()

	gpuTag := "CPU"
	if runtime.GOOS == "darwin" {
		logger.Debug("macOS detected — attempting to add CoreML")
		// macOS: CoreML (Apple Neural Engine / GPU / CPU auto-selected)
		if err := opts.AppendExecutionProviderCoreML(0); err == nil {
			gpuTag = "CoreML (Apple)"
			logger.Debug("CoreML added successfully")
		} else {
			logger.Debug("CoreML failed (%v), using CPU", err)
		}
	} else {
		logger.Debug("non-macOS — attempting to add CUDA")
		// Linux / Windows: NVIDIA CUDA
		if cudaOpts, cudaErr := ort.NewCUDAProviderOptions(); cudaErr == nil {
			if appendErr := opts.AppendExecutionProviderCUDA(cudaOpts); appendErr == nil {
				gpuTag = "CUDA (NVIDIA GPU)"
				logger.Debug("CUDA added successfully")
			} else {
				logger.Debug("CUDA AppendExecutionProvider failed (%v), using CPU", appendErr)
			}
			cudaOpts.Destroy()
		} else {
			logger.Debug("NewCUDAProviderOptions failed (%v), using CPU", cudaErr)
		}
	}

	logger.Debug("creating ONNX session...")
	session, err = ort.NewAdvancedSession(
		modelPath,
		[]string{inputName},
		[]string{outputName},
		[]ort.ArbitraryTensor{inTensor},
		[]ort.ArbitraryTensor{outTensor},
		opts,
	)
	if err != nil {
		inTensor.Destroy()
		outTensor.Destroy()
		inTensor, outTensor = nil, nil
		return fmt.Errorf("failed to create ONNX session (check tensor names — input=%q, output=%q): %w",
			inputName, outputName, err)
	}

	enabled = true
	fmt.Printf("[NSFW] initialized — device: %s, model: %s\n",
		gpuTag, filepath.Base(modelPath))
	return nil
}

// Close releases NSFW detector resources. Call it via defer in main.
func Close() {
	logger.Debug("releasing NSFW resources...")
	if session != nil {
		session.Destroy()
		session = nil
	}
	if inTensor != nil {
		inTensor.Destroy()
		inTensor = nil
	}
	if outTensor != nil {
		outTensor.Destroy()
		outTensor = nil
	}
	enabled = false
	logger.Debug("NSFW resources released")
}

// detectNSFW runs NSFW inference on an image file and
// returns (sfwScore, nsfwScore).
func detectNSFW(imagePath string) (sfwScore, nsfwScore float32, err error) {
	logger.Debug("NSFW inference start: %s", imagePath)

	f, err := os.Open(imagePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open image: %w", err)
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode image: %w", err)
	}
	bounds := img.Bounds()
	logger.Debug("image loaded: format=%s size=%dx%d",
		format, bounds.Dx(), bounds.Dy())

	fillTensor(inTensor.GetData(), img)
	logger.Debug("tensor filled (224x224 resize + mean subtraction applied)")

	t0 := time.Now()
	if err := session.Run(); err != nil {
		return 0, 0, fmt.Errorf("NSFW inference failed: %w", err)
	}
	elapsed := time.Since(t0)

	out := outTensor.GetData()
	logger.Debug("inference complete (%.3fs) — raw output: [%.6f, %.6f]",
		elapsed.Seconds(), out[0], out[1])

	return out[0], out[1], nil
}

// fillTensor scales img down to 224x224 and fills a NHWC float32 tensor.
// Subtracts the ImageNet channel mean (R=122.77, G=115.63, B=100.40).
func fillTensor(data []float32, img image.Image) {
	b := img.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	idx := 0
	for y := 0; y < imgSize; y++ {
		for x := 0; x < imgSize; x++ {
			srcX := b.Min.X + x*srcW/imgSize
			srcY := b.Min.Y + y*srcH/imgSize
			r, g, bv, _ := img.At(srcX, srcY).RGBA()
			// RGBA() returns [0, 65535]; >>8 converts to [0, 255]
			data[idx] = float32(r>>8) - meanR
			data[idx+1] = float32(g>>8) - meanG
			data[idx+2] = float32(bv>>8) - meanB
			idx += 3
		}
	}
}

// isVideoFile reports whether the file extension of path is a recognized video format.
func isVideoFile(path string) bool {
	return videoExts[strings.ToLower(filepath.Ext(path))]
}

// getVideoDuration returns the video duration in seconds via ffprobe.
// Returns 0 on failure.
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

// extractVideoFrames extracts up to videoFrameCount frames from a video using ffmpeg,
// saves them to a temp directory, and returns the path list with a cleanup function.
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
	// fps = n/duration → evenly extract n frames from the entire video
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

// detectVideoNSFW extracts frames from a video and runs NSFW inference on each frame.
// Returns the highest NSFW score across all frames (worst-case basis).
func detectVideoNSFW(videoPath string) (sfwScore, nsfwScore float32, err error) {
	frames, cleanup, err := extractVideoFrames(videoPath)
	if err != nil {
		return 0, 0, err
	}
	defer cleanup()

	var maxNSFW float32 = -1
	var pairedSFW float32
	succeeded := 0

	for i, frame := range frames {
		sfw, nsfw, ferr := detectNSFW(frame)
		if ferr != nil {
			logger.Debug("frame [%d/%d] inference failed: %v", i+1, len(frames), ferr)
			continue
		}
		logger.Debug("frame [%d/%d] SFW=%.4f NSFW=%.4f", i+1, len(frames), sfw, nsfw)
		succeeded++
		if nsfw > maxNSFW {
			maxNSFW = nsfw
			pairedSFW = sfw
		}
	}

	if succeeded == 0 {
		return 0, 0, fmt.Errorf("all frame inferences failed (%d frames)", len(frames))
	}
	logger.Debug("video NSFW result (%d/%d frames succeeded) — max: SFW=%.4f NSFW=%.4f",
		succeeded, len(frames), pairedSFW, maxNSFW)
	return pairedSFW, maxNSFW, nil
}

// DetectAndSaveNSFW runs NSFW detection on a file and
// saves the result to {file}.nsfwvalue.txt.
// Videos (.mp4 etc.) are processed by extracting frames with ffmpeg.
// Returns true on success (or when disabled), false on error.
func DetectAndSaveNSFW(filePath string) bool {
	if !enabled {
		logger.Debug("NSFW disabled, skipping detection: %s", filepath.Base(filePath))
		return true
	}

	var sfw, nsfwVal float32
	var err error

	if isVideoFile(filePath) {
		logger.Debug("video NSFW detection start: %s", filepath.Base(filePath))
		sfw, nsfwVal, err = detectVideoNSFW(filePath)
	} else {
		sfw, nsfwVal, err = detectNSFW(filePath)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "  [WARN] NSFW detection failed (%s): %v\n",
			filepath.Base(filePath), err)
		return false
	}

	content := fmt.Sprintf("SFW:  %.4f\nNSFW: %.4f\n", sfw, nsfwVal)
	txtPath := filePath + ".nsfwvalue.txt"
	logger.Debug("saving NSFW result: %s", txtPath)
	if err := os.WriteFile(txtPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "  [WARN] failed to save NSFW result (%s): %v\n",
			filepath.Base(filePath), err)
		return false
	}
	fmt.Printf("  [NSFW] %s → SFW=%.4f NSFW=%.4f\n",
		filepath.Base(filePath), sfw, nsfwVal)
	return true
}

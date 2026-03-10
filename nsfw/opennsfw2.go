// OpenNSFW2 binary NSFW classifier for the nsfw package.
//
// Model: OpenNSFW-compatible ONNX (e.g. opennsfw2 / Yahoo OpenNSFW)
//
//	Input  : input_1 [1, 224, 224, 3] float32 NHWC, ImageNet mean subtracted (RGB)
//	Output : [1, 2] float32 → [SFW probability, NSFW probability]
//
// GPU priority:
//
//	macOS  → CoreML (Apple Neural Engine / GPU) → CPU
//	Other  → CUDA (NVIDIA GPU)                  → CPU
package nsfw

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kinkist/x-media-downloader/logger"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	openImgSize = 224
	// ImageNet channel mean (RGB order)
	meanR = float32(122.7717)
	meanG = float32(115.6290)
	meanB = float32(100.4038)
)

var (
	openSession    *ort.AdvancedSession
	openInTensor   *ort.Tensor[float32]
	openOutTensor  *ort.Tensor[float32]
	enabled        bool
	openInputName  = "input"
	openOutputName = "output"
)

// Init initializes the OpenNSFW2 classifier.
//
//	modelPath : path to the .onnx model file
//	libPath   : ONNX Runtime shared library path (empty = system default)
//	inName    : ONNX input tensor name  (empty = auto-detect, fallback "input")
//	outName   : ONNX output tensor name (empty = auto-detect, fallback "output")
func Init(modelPath, libPath, inName, outName string) error {
	logger.Debug("OpenNSFW2 Init — model=%s lib=%q inputName=%q outputName=%q",
		modelPath, libPath, inName, outName)

	if modelPath == "" {
		return fmt.Errorf("OpenNSFW2 model path not set")
	}
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("OpenNSFW2 model file not found: %s", modelPath)
	}

	if libPath != "" {
		ort.SetSharedLibraryPath(libPath)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		logger.Debug("ort.InitializeEnvironment (OpenNSFW2): %v (may be already initialized)", err)
	}

	// apply config-specified names
	if inName != "" {
		openInputName = inName
	}
	if outName != "" {
		openOutputName = outName
	}

	// auto-detect unspecified names from the model
	if inName == "" || outName == "" {
		infInputs, infOutputs, infErr := ort.GetInputOutputInfo(modelPath)
		if infErr != nil {
			logger.Debug("auto-detect tensor names failed (%v), using defaults", infErr)
		} else {
			if inName == "" && len(infInputs) > 0 {
				openInputName = infInputs[0].Name
				logger.Debug("auto-detected input tensor name: %q", openInputName)
			}
			if outName == "" && len(infOutputs) > 0 {
				openOutputName = infOutputs[0].Name
				logger.Debug("auto-detected output tensor name: %q", openOutputName)
			}
		}
	}
	logger.Debug("OpenNSFW2 tensor names — input=%q output=%q", openInputName, openOutputName)

	var err error
	openInTensor, err = ort.NewEmptyTensor[float32](ort.NewShape(1, openImgSize, openImgSize, 3))
	if err != nil {
		return fmt.Errorf("OpenNSFW2: failed to create input tensor: %w", err)
	}
	openOutTensor, err = ort.NewEmptyTensor[float32](ort.NewShape(1, 2))
	if err != nil {
		openInTensor.Destroy()
		openInTensor = nil
		return fmt.Errorf("OpenNSFW2: failed to create output tensor: %w", err)
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		openInTensor.Destroy()
		openOutTensor.Destroy()
		openInTensor, openOutTensor = nil, nil
		return fmt.Errorf("OpenNSFW2: failed to create session options: %w", err)
	}
	defer opts.Destroy()

	gpuTag := "CPU"
	if runtime.GOOS == "darwin" {
		if nudeEnabled {
			// NudeNet v2 is already using CoreML — avoid EP conflict by falling back to CPU.
			logger.Debug("OpenNSFW2: CoreML skipped (NudeNet v2 already occupies CoreML), using CPU")
		} else if e := opts.AppendExecutionProviderCoreML(0); e == nil {
			gpuTag = "CoreML (Apple)"
			logger.Debug("OpenNSFW2: CoreML added")
		} else {
			logger.Debug("OpenNSFW2: CoreML unavailable (%v), using CPU", e)
		}
	} else {
		if cudaOpts, cudaErr := ort.NewCUDAProviderOptions(); cudaErr == nil {
			if appendErr := opts.AppendExecutionProviderCUDA(cudaOpts); appendErr == nil {
				gpuTag = "CUDA (NVIDIA GPU)"
				logger.Debug("OpenNSFW2: CUDA added")
			} else {
				logger.Debug("OpenNSFW2: CUDA unavailable (%v), using CPU", appendErr)
			}
			cudaOpts.Destroy()
		}
	}

	openSession, err = ort.NewAdvancedSession(
		modelPath,
		[]string{openInputName},
		[]string{openOutputName},
		[]ort.ArbitraryTensor{openInTensor},
		[]ort.ArbitraryTensor{openOutTensor},
		opts,
	)
	if err != nil {
		openInTensor.Destroy()
		openOutTensor.Destroy()
		openInTensor, openOutTensor = nil, nil
		return fmt.Errorf("OpenNSFW2: failed to create session (input=%q output=%q): %w",
			openInputName, openOutputName, err)
	}

	enabled = true
	fmt.Printf("[NSFW] OpenNSFW2 initialized — device: %s, model: %s\n",
		gpuTag, filepath.Base(modelPath))
	return nil
}

// Close releases OpenNSFW2 resources. Call via defer in main.
func Close() {
	logger.Debug("releasing OpenNSFW2 resources...")
	if openSession != nil {
		openSession.Destroy()
		openSession = nil
	}
	if openInTensor != nil {
		openInTensor.Destroy()
		openInTensor = nil
	}
	if openOutTensor != nil {
		openOutTensor.Destroy()
		openOutTensor = nil
	}
	enabled = false
	logger.Debug("OpenNSFW2 resources released")
}

// detectOpenNSFW2 runs inference on a single image and returns (sfwScore, nsfwScore).
func detectOpenNSFW2(imagePath string) (sfwScore, nsfwScore float32, err error) {
	logger.Debug("OpenNSFW2 inference: %s", imagePath)

	f, err := os.Open(imagePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open image: %w", err)
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode image: %w", err)
	}
	logger.Debug("OpenNSFW2 image: format=%s size=%dx%d", format, img.Bounds().Dx(), img.Bounds().Dy())

	fillOpenNSFW2Tensor(openInTensor.GetData(), img)

	t0 := time.Now()
	if err := openSession.Run(); err != nil {
		return 0, 0, fmt.Errorf("OpenNSFW2 inference failed: %w", err)
	}
	logger.Debug("OpenNSFW2 inference complete (%.3fs)", time.Since(t0).Seconds())

	out := openOutTensor.GetData()
	return out[0], out[1], nil
}

// fillOpenNSFW2Tensor scales img to 224×224 NHWC float32, subtracting ImageNet channel mean (RGB).
func fillOpenNSFW2Tensor(data []float32, img image.Image) {
	b := img.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	idx := 0
	for y := 0; y < openImgSize; y++ {
		for x := 0; x < openImgSize; x++ {
			srcX := b.Min.X + x*srcW/openImgSize
			srcY := b.Min.Y + y*srcH/openImgSize
			r, g, bv, _ := img.At(srcX, srcY).RGBA()
			data[idx] = float32(r>>8) - meanR
			data[idx+1] = float32(g>>8) - meanG
			data[idx+2] = float32(bv>>8) - meanB
			idx += 3
		}
	}
}

// detectVideoOpenNSFW2 extracts frames and returns the highest NSFW score across all frames.
func detectVideoOpenNSFW2(videoPath string) (sfwScore, nsfwScore float32, err error) {
	frames, cleanup, err := extractVideoFrames(videoPath)
	if err != nil {
		return 0, 0, err
	}
	defer cleanup()

	var maxNSFW float32 = -1
	var pairedSFW float32
	succeeded := 0

	for i, frame := range frames {
		sfw, nsfw, ferr := detectOpenNSFW2(frame)
		if ferr != nil {
			logger.Debug("OpenNSFW2 frame [%d/%d] failed: %v", i+1, len(frames), ferr)
			continue
		}
		logger.Debug("OpenNSFW2 frame [%d/%d] SFW=%.4f NSFW=%.4f", i+1, len(frames), sfw, nsfw)
		succeeded++
		if nsfw > maxNSFW {
			maxNSFW = nsfw
			pairedSFW = sfw
		}
	}
	if succeeded == 0 {
		return 0, 0, fmt.Errorf("OpenNSFW2: all frame inferences failed (%d frames)", len(frames))
	}
	return pairedSFW, maxNSFW, nil
}

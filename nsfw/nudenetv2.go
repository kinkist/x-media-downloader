// NudeNet v2 object detector for the nsfw package.
//
// Model: detector_v2_default_checkpoint.onnx (EfficientDet/RetinaNet-based)
//
//	Input  : input_1:0  [1, 320, 320, 3] float32 NHWC BGR, raw pixel [0, 255]
//	         *** OpenCV BGR order, NO /255.0 normalization — raw uint8 values ***
//	Output1: boxes      [1, 300, 4] float32  (bounding boxes)
//	Output2: labels     [1, 300] int32       (class index, 0-based)
//	Output3: scores     [1, 300] float32     (detection confidence; -1 = padding)
//
// The model always returns exactly 300 candidate detections (padded with -1).
// Only detections with score > 0 and >= defaultNudeThreshold are reported.
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
	"runtime"
	"sort"

	"github.com/kinkist/x-media-downloader/logger"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	nudeImgSize = 320
	// defaultNudeThreshold is the fixed detection confidence threshold.
	// Not configurable — always 0.6 to reduce false positives.
	defaultNudeThreshold = float32(0.6)

	// ONNX tensor names (verified from model introspection)
	nudeInputName  = "input_1:0"
	nudeBoxesName  = "filtered_detections/map/TensorArrayStack/TensorArrayGatherV3:0"
	nudeLabelsName = "filtered_detections/map/TensorArrayStack_2/TensorArrayGatherV3:0"
	nudeScoresName = "filtered_detections/map/TensorArrayStack_1/TensorArrayGatherV3:0"
)

// nudeNetClasses maps label index → human-readable class name.
// Order matches the NudeNet v2 detector_v2_default_checkpoint.onnx training labels.
var nudeNetClasses = []string{
	"FEMALE_GENITALIA_COVERED", // 0
	"FACE_COVERED",             // 1
	"BUTTOCKS_EXPOSED",         // 2
	"FEMALE_BREAST_EXPOSED",    // 3
	"FEMALE_GENITALIA_EXPOSED", // 4
	"MALE_BREAST_EXPOSED",      // 5
	"ANUS_EXPOSED",             // 6
	"FEET_EXPOSED",             // 7
	"BELLY_COVERED",            // 8
	"FEET_COVERED",             // 9
	"ARMPITS_COVERED",          // 10
	"ARMPITS_EXPOSED",          // 11
	"FACE_FEMALE",              // 12  ← NOT "FACE_COVERED" (that is index 1)
	"BELLY_EXPOSED",            // 13
	"MALE_GENITALIA_EXPOSED",   // 14
	"ANUS_COVERED",             // 15
	"FEMALE_BREAST_COVERED",    // 16
	"BUTTOCKS_COVERED",         // 17
}

var (
	nudeSession     *ort.AdvancedSession
	nudeInTensor    *ort.Tensor[float32]
	nudeBoxTensor   *ort.Tensor[float32]
	nudeLabelTensor *ort.Tensor[int32]
	nudeScoreTensor *ort.Tensor[float32]
	nudeEnabled     bool
)

// NudeDetection represents a single detected content class.
type NudeDetection struct {
	Class string
	Score float32
}

// InitNudeNet initializes the NudeNet v2 detector.
//
//	modelPath : path to detector_v2_default_checkpoint.onnx
//	libPath   : ONNX Runtime shared library path (empty = system default)
//
// Detection threshold is fixed at 0.6 (not configurable).
// Safe to call after Init() (OpenNSFW2) — ONNX environment re-init is a no-op.
func InitNudeNet(modelPath, libPath string) error {
	logger.Debug("NudeNet v2 Init — model=%s lib=%q threshold=%.2f", modelPath, libPath, defaultNudeThreshold)

	if modelPath == "" {
		return fmt.Errorf("NudeNet v2 model path not set")
	}
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("NudeNet v2 model file not found: %s", modelPath)
	}

	if libPath != "" {
		ort.SetSharedLibraryPath(libPath)
	}
	// InitializeEnvironment is safe to call multiple times.
	if err := ort.InitializeEnvironment(); err != nil {
		logger.Debug("ort.InitializeEnvironment (NudeNet v2): %v (may be already initialized)", err)
	}

	var err error
	nudeInTensor, err = ort.NewEmptyTensor[float32](ort.NewShape(1, nudeImgSize, nudeImgSize, 3))
	if err != nil {
		return fmt.Errorf("NudeNet v2: failed to create input tensor: %w", err)
	}

	nudeBoxTensor, err = ort.NewEmptyTensor[float32](ort.NewShape(1, 300, 4))
	if err != nil {
		nudeInTensor.Destroy()
		nudeInTensor = nil
		return fmt.Errorf("NudeNet v2: failed to create box tensor: %w", err)
	}
	nudeLabelTensor, err = ort.NewEmptyTensor[int32](ort.NewShape(1, 300))
	if err != nil {
		nudeInTensor.Destroy()
		nudeBoxTensor.Destroy()
		nudeInTensor, nudeBoxTensor = nil, nil
		return fmt.Errorf("NudeNet v2: failed to create label tensor: %w", err)
	}
	nudeScoreTensor, err = ort.NewEmptyTensor[float32](ort.NewShape(1, 300))
	if err != nil {
		nudeInTensor.Destroy()
		nudeBoxTensor.Destroy()
		nudeLabelTensor.Destroy()
		nudeInTensor, nudeBoxTensor, nudeLabelTensor = nil, nil, nil
		return fmt.Errorf("NudeNet v2: failed to create score tensor: %w", err)
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		nudeInTensor.Destroy()
		nudeBoxTensor.Destroy()
		nudeLabelTensor.Destroy()
		nudeScoreTensor.Destroy()
		nudeInTensor, nudeBoxTensor, nudeLabelTensor, nudeScoreTensor = nil, nil, nil, nil
		return fmt.Errorf("NudeNet v2: failed to create session options: %w", err)
	}
	defer opts.Destroy()

	gpuTag := "CPU"
	if runtime.GOOS == "darwin" {
		if enabled {
			// OpenNSFW2 is already using CoreML — avoid EP conflict by falling back to CPU.
			logger.Debug("NudeNet v2: CoreML skipped (OpenNSFW2 already occupies CoreML), using CPU")
		} else if e := opts.AppendExecutionProviderCoreML(0); e == nil {
			gpuTag = "CoreML (Apple)"
			logger.Debug("NudeNet v2: CoreML added")
		} else {
			logger.Debug("NudeNet v2: CoreML unavailable (%v), using CPU", e)
		}
	} else {
		if cudaOpts, cudaErr := ort.NewCUDAProviderOptions(); cudaErr == nil {
			if appendErr := opts.AppendExecutionProviderCUDA(cudaOpts); appendErr == nil {
				gpuTag = "CUDA (NVIDIA GPU)"
				logger.Debug("NudeNet v2: CUDA added")
			} else {
				logger.Debug("NudeNet v2: CUDA unavailable (%v), using CPU", appendErr)
			}
			cudaOpts.Destroy()
		}
	}

	nudeSession, err = ort.NewAdvancedSession(
		modelPath,
		[]string{nudeInputName},
		[]string{nudeBoxesName, nudeLabelsName, nudeScoresName},
		[]ort.ArbitraryTensor{nudeInTensor},
		[]ort.ArbitraryTensor{nudeBoxTensor, nudeLabelTensor, nudeScoreTensor},
		opts,
	)
	if err != nil {
		nudeInTensor.Destroy()
		nudeBoxTensor.Destroy()
		nudeLabelTensor.Destroy()
		nudeScoreTensor.Destroy()
		nudeInTensor, nudeBoxTensor, nudeLabelTensor, nudeScoreTensor = nil, nil, nil, nil
		return fmt.Errorf("NudeNet v2: failed to create session: %w", err)
	}

	nudeEnabled = true
	fmt.Printf("[NSFW] NudeNet v2 initialized — device: %s, model: %s, threshold: %.2f\n",
		gpuTag, modelPath, defaultNudeThreshold)
	return nil
}

// CloseNudeNet releases all NudeNet v2 resources. Call via defer in main.
func CloseNudeNet() {
	logger.Debug("releasing NudeNet v2 resources...")
	if nudeSession != nil {
		nudeSession.Destroy()
		nudeSession = nil
	}
	if nudeInTensor != nil {
		nudeInTensor.Destroy()
		nudeInTensor = nil
	}
	if nudeBoxTensor != nil {
		nudeBoxTensor.Destroy()
		nudeBoxTensor = nil
	}
	if nudeLabelTensor != nil {
		nudeLabelTensor.Destroy()
		nudeLabelTensor = nil
	}
	if nudeScoreTensor != nil {
		nudeScoreTensor.Destroy()
		nudeScoreTensor = nil
	}
	nudeEnabled = false
	logger.Debug("NudeNet v2 resources released")
}

// IsNudeNetEnabled reports whether NudeNet v2 detection is active.
func IsNudeNetEnabled() bool { return nudeEnabled }

// fillNudeTensor resizes img to 320×320 NHWC float32 in BGR channel order.
//
// NudeNet v2 was trained with OpenCV images (BGR order, raw 0–255 pixel values).
// IMPORTANT: No /255.0 normalization — raw uint8 values [0, 255] are used as-is.
func fillNudeTensor(data []float32, img image.Image) {
	b := img.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	idx := 0
	for y := 0; y < nudeImgSize; y++ {
		for x := 0; x < nudeImgSize; x++ {
			srcX := b.Min.X + x*srcW/nudeImgSize
			srcY := b.Min.Y + y*srcH/nudeImgSize
			r, g, bv, _ := img.At(srcX, srcY).RGBA()
			// BGR order (OpenCV), raw 0-255 (no /255.0)
			data[idx] = float32(bv >> 8) // B
			data[idx+1] = float32(g >> 8)  // G
			data[idx+2] = float32(r >> 8)  // R
			idx += 3
		}
	}
}

// detectNudeNet runs NudeNet v2 inference on a single image file.
// Returns detections with score >= defaultNudeThreshold (may be empty, not an error).
func detectNudeNet(imagePath string) ([]NudeDetection, error) {
	logger.Debug("NudeNet v2 inference: %s", imagePath)

	f, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}
	logger.Debug("NudeNet v2 image: format=%s size=%dx%d", format, img.Bounds().Dx(), img.Bounds().Dy())

	fillNudeTensor(nudeInTensor.GetData(), img)

	if err := nudeSession.Run(); err != nil {
		return nil, fmt.Errorf("NudeNet v2 inference failed: %w", err)
	}

	scores := nudeScoreTensor.GetData() // [300]
	labels := nudeLabelTensor.GetData() // [300]

	var detections []NudeDetection
	for i := 0; i < 300; i++ {
		score := scores[i]
		if score < defaultNudeThreshold {
			continue
		}
		labelIdx := int(labels[i])
		if labelIdx < 0 || labelIdx >= len(nudeNetClasses) {
			logger.Debug("NudeNet v2: invalid label index %d at detection %d, skipping", labelIdx, i)
			continue
		}
		detections = append(detections, NudeDetection{
			Class: nudeNetClasses[labelIdx],
			Score: score,
		})
	}
	logger.Debug("NudeNet v2: %d detections above threshold %.2f", len(detections), defaultNudeThreshold)
	return detections, nil
}

// detectVideoNudeNet extracts video frames and runs NudeNet v2 on each.
// Returns the union of all detections, keeping the highest score per class.
func detectVideoNudeNet(videoPath string) ([]NudeDetection, error) {
	frames, cleanup, err := extractVideoFrames(videoPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	best := make(map[string]float32)
	succeeded := 0
	for i, frame := range frames {
		dets, ferr := detectNudeNet(frame)
		if ferr != nil {
			logger.Debug("NudeNet v2 frame [%d/%d] failed: %v", i+1, len(frames), ferr)
			continue
		}
		succeeded++
		for _, d := range dets {
			if d.Score > best[d.Class] {
				best[d.Class] = d.Score
			}
		}
	}
	if succeeded == 0 {
		return nil, fmt.Errorf("NudeNet v2: all frame inferences failed (%d frames)", len(frames))
	}

	result := make([]NudeDetection, 0, len(best))
	for class, score := range best {
		result = append(result, NudeDetection{Class: class, Score: score})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	logger.Debug("NudeNet v2 video: %d unique classes (%d/%d frames succeeded)",
		len(result), succeeded, len(frames))
	return result, nil
}

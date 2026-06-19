package onnx

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sync"
	"time"

	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	defaultImageSize           = 640
	defaultConfidenceThreshold = 0.25
	defaultIOUThreshold        = 0.7
	defaultMaxDetections       = 300
	inputName                  = "images"
	outputName                 = "output0"
)

var runtimeMu sync.Mutex

type Config struct {
	ModelPath           string
	RuntimeLibraryPath  string
	ImageSize           int
	ConfidenceThreshold float64
	IOUThreshold        float64
	MaxDetections       int
}

type Detector struct {
	session *ort.DynamicAdvancedSession
	cfg     Config
	runMu   sync.Mutex
}

type DetectorSet struct {
	detectors []*Detector
	cfg       Config
}

func NewDetector(cfg Config) (*Detector, error) {
	cfg = normalizeConfig(cfg)
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("onnx model path is required")
	}

	if err := initializeRuntime(cfg.RuntimeLibraryPath); err != nil {
		return nil, err
	}

	session, err := ort.NewDynamicAdvancedSession(cfg.ModelPath, []string{inputName}, []string{outputName}, nil)
	if err != nil {
		return nil, fmt.Errorf("create onnx session: %w", err)
	}

	return &Detector{session: session, cfg: cfg}, nil
}

func NewDetectorSet(cfg Config, modelPaths []string) (*DetectorSet, error) {
	cfg = normalizeConfig(cfg)
	if len(modelPaths) == 0 {
		return nil, fmt.Errorf("at least one onnx model path is required")
	}

	detectors := make([]*Detector, 0, len(modelPaths))
	for _, modelPath := range modelPaths {
		modelCfg := cfg
		modelCfg.ModelPath = modelPath
		detector, err := NewDetector(modelCfg)
		if err != nil {
			_ = closeDetectors(detectors)
			return nil, fmt.Errorf("create detector for %s: %w", modelPath, err)
		}
		detectors = append(detectors, detector)
	}

	return &DetectorSet{detectors: detectors, cfg: cfg}, nil
}

func (detector *Detector) Detect(
	ctx context.Context,
	img image.Image,
) (detectionmodel.ImageResult, error) {
	if err := ctx.Err(); err != nil {
		return detectionmodel.ImageResult{}, fmt.Errorf("detect context: %w", err)
	}

	preprocessStarted := time.Now()
	inputData, letterbox := prepareInput(img, detector.cfg.ImageSize)
	preprocessMS := millisecondsSince(preprocessStarted)

	inferenceStarted := time.Now()
	outputTensor, err := detector.run(inputData)
	inferenceMS := millisecondsSince(inferenceStarted)
	if err != nil {
		return detectionmodel.ImageResult{}, err
	}
	defer func() {
		_ = outputTensor.Destroy()
	}()

	postprocessStarted := time.Now()
	detections, err := parseDetections(outputTensor.GetData(), []int64(outputTensor.GetShape()), letterbox, detector.cfg)
	if err != nil {
		return detectionmodel.ImageResult{}, err
	}
	postprocessMS := millisecondsSince(postprocessStarted)

	return detectionmodel.ImageResult{
		Shape:   letterbox.OriginalShape,
		Results: detections,
		Speed: detectionmodel.Speed{
			Preprocess:  preprocessMS,
			Inference:   inferenceMS,
			Postprocess: postprocessMS,
		},
	}, nil
}

func (set *DetectorSet) Detect(
	ctx context.Context,
	img image.Image,
) (detectionmodel.ImageResult, error) {
	if set == nil || len(set.detectors) == 0 {
		return detectionmodel.ImageResult{}, fmt.Errorf("onnx detector set is empty")
	}

	result := detectionmodel.ImageResult{}
	for _, detector := range set.detectors {
		imageResult, err := detector.Detect(ctx, img)
		if err != nil {
			return detectionmodel.ImageResult{}, err
		}
		result = mergeImageResults(result, imageResult)
	}
	result.Results = nonMaxSuppressionClassAgnostic(result.Results, set.cfg.IOUThreshold, set.cfg.MaxDetections)

	return result, nil
}

func (detector *Detector) Close() error {
	if detector == nil || detector.session == nil {
		return nil
	}

	if err := detector.session.Destroy(); err != nil {
		return fmt.Errorf("destroy onnx session: %w", err)
	}

	return nil
}

func (set *DetectorSet) Close() error {
	if set == nil {
		return nil
	}

	return closeDetectors(set.detectors)
}

func closeDetectors(detectors []*Detector) error {
	var joined error
	for _, detector := range detectors {
		if err := detector.Close(); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	return joined
}

func mergeImageResults(
	base detectionmodel.ImageResult,
	next detectionmodel.ImageResult,
) detectionmodel.ImageResult {
	if base.Shape == [2]int{} {
		base.Shape = next.Shape
	}
	base.Results = append(base.Results, next.Results...)
	base.Speed.Preprocess += next.Speed.Preprocess
	base.Speed.Inference += next.Speed.Inference
	base.Speed.Postprocess += next.Speed.Postprocess

	return base
}

func (detector *Detector) run(inputData []float32) (*ort.Tensor[float32], error) {
	inputShape := ort.NewShape(1, 3, int64(detector.cfg.ImageSize), int64(detector.cfg.ImageSize))
	inputTensor, err := ort.NewTensor(inputShape, inputData)
	if err != nil {
		return nil, fmt.Errorf("create onnx input tensor: %w", err)
	}
	defer func() {
		_ = inputTensor.Destroy()
	}()

	outputs := []ort.Value{nil}
	detector.runMu.Lock()
	err = detector.session.Run([]ort.Value{inputTensor}, outputs)
	detector.runMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("run onnx session: %w", err)
	}

	outputTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		_ = outputs[0].Destroy()
		return nil, fmt.Errorf("onnx output is not a float32 tensor")
	}

	return outputTensor, nil
}

func initializeRuntime(sharedLibraryPath string) error {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()

	if ort.IsInitialized() {
		return nil
	}
	if sharedLibraryPath != "" {
		ort.SetSharedLibraryPath(sharedLibraryPath)
	}
	if err := ort.InitializeEnvironment(ort.WithLogLevelWarning()); err != nil {
		return fmt.Errorf("initialize onnx runtime: %w", err)
	}
	if err := ort.DisableTelemetry(); err != nil {
		return fmt.Errorf("disable onnx runtime telemetry: %w", err)
	}

	return nil
}

func normalizeConfig(cfg Config) Config {
	if cfg.ImageSize <= 0 {
		cfg.ImageSize = defaultImageSize
	}
	if cfg.ConfidenceThreshold <= 0 {
		cfg.ConfidenceThreshold = defaultConfidenceThreshold
	}
	if cfg.IOUThreshold <= 0 {
		cfg.IOUThreshold = defaultIOUThreshold
	}
	if cfg.MaxDetections <= 0 {
		cfg.MaxDetections = defaultMaxDetections
	}

	return cfg
}

func millisecondsSince(started time.Time) float64 {
	return float64(time.Since(started).Microseconds()) / 1_000
}

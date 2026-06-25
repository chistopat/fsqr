package onnx

import (
	"fmt"
	"math"
	"sort"

	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"
)

type outputLayout struct {
	Channels      int
	Anchors       int
	ChannelsFirst bool
}

func parseDetections(
	data []float32,
	shape []int64,
	letterbox letterboxMetadata,
	cfg Config,
) ([]detectionmodel.Detection, error) {
	layout, err := detectOutputLayout(shape)
	if err != nil {
		return nil, err
	}
	if layout.Channels*layout.Anchors > len(data) {
		return nil, fmt.Errorf("onnx output shape %v exceeds output data length %d", shape, len(data))
	}

	candidates := make([]detectionmodel.Detection, 0)
	for anchor := 0; anchor < layout.Anchors; anchor++ {
		detection, ok := decodeAnchor(data, layout, anchor, letterbox, cfg)
		if ok {
			candidates = append(candidates, detection)
		}
	}

	return nonMaxSuppression(candidates, cfg.IOUThreshold, cfg.MaxDetections), nil
}

func detectOutputLayout(shape []int64) (outputLayout, error) {
	dims := shape
	if len(dims) == 3 {
		dims = dims[1:]
	}
	if len(dims) != 2 {
		return outputLayout{}, fmt.Errorf("unsupported onnx output shape %v", shape)
	}

	first := int(dims[0])
	second := int(dims[1])
	if first >= 5 && first <= 256 && second > first {
		return outputLayout{Channels: first, Anchors: second, ChannelsFirst: true}, nil
	}
	if second >= 5 {
		return outputLayout{Channels: second, Anchors: first, ChannelsFirst: false}, nil
	}

	return outputLayout{}, fmt.Errorf("unsupported onnx output dimensions %v", shape)
}

func decodeAnchor(
	data []float32,
	layout outputLayout,
	anchor int,
	letterbox letterboxMetadata,
	cfg Config,
) (detectionmodel.Detection, bool) {
	confidence, classID := classConfidence(data, layout, anchor)
	if confidence < cfg.ConfidenceThreshold {
		return detectionmodel.Detection{}, false
	}

	box, ok := outputBox(data, layout, anchor, letterbox)
	if !ok {
		return detectionmodel.Detection{}, false
	}

	return detectionmodel.Detection{
		Class:      classID,
		Name:       "object",
		Confidence: round5(confidence),
		Box:        box,
	}, true
}

func classConfidence(data []float32, layout outputLayout, anchor int) (confidence float64, classID int) {
	bestScore := float64(0)
	bestClass := 0
	for classIndex := 4; classIndex < layout.Channels; classIndex++ {
		score := float64(outputValue(data, layout, anchor, classIndex))
		if score > bestScore {
			bestScore = score
			bestClass = classIndex - 4
		}
	}

	return bestScore, bestClass
}

func outputBox(
	data []float32,
	layout outputLayout,
	anchor int,
	letterbox letterboxMetadata,
) (detectionmodel.Box, bool) {
	centerX := float64(outputValue(data, layout, anchor, 0))
	centerY := float64(outputValue(data, layout, anchor, 1))
	width := float64(outputValue(data, layout, anchor, 2))
	height := float64(outputValue(data, layout, anchor, 3))

	x1 := (centerX - width/2 - letterbox.PadX) / letterbox.Scale
	y1 := (centerY - height/2 - letterbox.PadY) / letterbox.Scale
	x2 := (centerX + width/2 - letterbox.PadX) / letterbox.Scale
	y2 := (centerY + height/2 - letterbox.PadY) / letterbox.Scale

	box := clampBox(detectionmodel.Box{X1: x1, Y1: y1, X2: x2, Y2: y2}, letterbox.OriginalShape)
	if box.X2 <= box.X1 || box.Y2 <= box.Y1 {
		return detectionmodel.Box{}, false
	}

	return roundBox(box), true
}

func outputValue(data []float32, layout outputLayout, anchor, channel int) float32 {
	if layout.ChannelsFirst {
		return data[channel*layout.Anchors+anchor]
	}

	return data[anchor*layout.Channels+channel]
}

func nonMaxSuppression(
	detections []detectionmodel.Detection,
	iouThreshold float64,
	maxDetections int,
) []detectionmodel.Detection {
	return nonMaxSuppressionWithClassMatching(detections, iouThreshold, maxDetections, true)
}

func nonMaxSuppressionClassAgnostic(
	detections []detectionmodel.Detection,
	iouThreshold float64,
	maxDetections int,
) []detectionmodel.Detection {
	return nonMaxSuppressionWithClassMatching(detections, iouThreshold, maxDetections, false)
}

func nonMaxSuppressionWithClassMatching(
	detections []detectionmodel.Detection,
	iouThreshold float64,
	maxDetections int,
	matchClass bool,
) []detectionmodel.Detection {
	sort.Slice(detections, func(i, j int) bool {
		return detections[i].Confidence > detections[j].Confidence
	})

	selected := make([]detectionmodel.Detection, 0, min(len(detections), maxDetections))
	for _, candidate := range detections {
		if len(selected) >= maxDetections {
			break
		}
		if overlapsSelected(candidate, selected, iouThreshold, matchClass) {
			continue
		}
		selected = append(selected, candidate)
	}

	return selected
}

func overlapsSelected(
	candidate detectionmodel.Detection,
	selected []detectionmodel.Detection,
	iouThreshold float64,
	matchClass bool,
) bool {
	for _, existing := range selected {
		classMatches := !matchClass || candidate.Class == existing.Class
		if classMatches && boxIoU(candidate.Box, existing.Box) > iouThreshold {
			return true
		}
	}

	return false
}

func boxIoU(a, b detectionmodel.Box) float64 {
	interX1 := math.Max(a.X1, b.X1)
	interY1 := math.Max(a.Y1, b.Y1)
	interX2 := math.Min(a.X2, b.X2)
	interY2 := math.Min(a.Y2, b.Y2)
	interWidth := math.Max(0, interX2-interX1)
	interHeight := math.Max(0, interY2-interY1)
	intersection := interWidth * interHeight
	if intersection == 0 {
		return 0
	}

	areaA := (a.X2 - a.X1) * (a.Y2 - a.Y1)
	areaB := (b.X2 - b.X1) * (b.Y2 - b.Y1)

	return intersection / (areaA + areaB - intersection)
}

func clampBox(box detectionmodel.Box, shape [2]int) detectionmodel.Box {
	height := float64(shape[0])
	width := float64(shape[1])
	box.X1 = math.Max(0, math.Min(width, box.X1))
	box.Y1 = math.Max(0, math.Min(height, box.Y1))
	box.X2 = math.Max(0, math.Min(width, box.X2))
	box.Y2 = math.Max(0, math.Min(height, box.Y2))

	return box
}

func roundBox(box detectionmodel.Box) detectionmodel.Box {
	return detectionmodel.Box{
		X1: round5(box.X1),
		Y1: round5(box.Y1),
		X2: round5(box.X2),
		Y2: round5(box.Y2),
	}
}

func round5(value float64) float64 {
	return math.Round(value*100_000) / 100_000
}

package onnx

import (
	"testing"

	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"
)

func TestParseDetectionsAppliesThresholdAndNMS(t *testing.T) {
	t.Parallel()

	data := []float32{
		50, 52, 80, 1, 2, 3, // x center
		50, 52, 80, 1, 2, 3, // y center
		20, 20, 10, 1, 1, 1, // width
		20, 20, 10, 1, 1, 1, // height
		0.90, 0.80, 0.10, 0, 0, 0, // class confidence
	}
	detections, err := parseDetections(data, []int64{1, 5, 6}, letterboxMetadata{
		OriginalShape: [2]int{100, 100},
		Scale:         1,
	}, Config{
		ConfidenceThreshold: 0.25,
		IOUThreshold:        0.5,
		MaxDetections:       10,
	})
	if err != nil {
		t.Fatalf("parse detections: %v", err)
	}

	if len(detections) != 1 {
		t.Fatalf("expected one detection after threshold and nms, got %d", len(detections))
	}
	if detections[0].Confidence != 0.9 {
		t.Fatalf("expected highest confidence detection, got %f", detections[0].Confidence)
	}
	if detections[0].Box.X1 != 40 || detections[0].Box.Y1 != 40 {
		t.Fatalf("unexpected box: %#v", detections[0].Box)
	}
}

func TestMergeImageResultsCreatesCommonDetectionPool(t *testing.T) {
	t.Parallel()

	first := detectionmodel.ImageResult{
		Shape: [2]int{100, 100},
		Results: []detectionmodel.Detection{{
			Class:      0,
			Confidence: 0.90,
			Box:        detectionmodel.Box{X1: 10, Y1: 10, X2: 50, Y2: 50},
		}},
		Speed: detectionmodel.Speed{Preprocess: 1, Inference: 2, Postprocess: 3},
	}
	second := detectionmodel.ImageResult{
		Shape: [2]int{100, 100},
		Results: []detectionmodel.Detection{
			{
				Class:      1,
				Confidence: 0.80,
				Box:        detectionmodel.Box{X1: 11, Y1: 11, X2: 51, Y2: 51},
			},
			{
				Class:      0,
				Confidence: 0.70,
				Box:        detectionmodel.Box{X1: 60, Y1: 60, X2: 90, Y2: 90},
			},
		},
		Speed: detectionmodel.Speed{Preprocess: 4, Inference: 5, Postprocess: 6},
	}

	merged := mergeImageResults(first, second)
	merged.Results = nonMaxSuppressionClassAgnostic(merged.Results, 0.5, 300)

	if merged.Shape != [2]int{100, 100} {
		t.Fatalf("expected original shape to be preserved, got %#v", merged.Shape)
	}
	if merged.Speed != (detectionmodel.Speed{Preprocess: 5, Inference: 7, Postprocess: 9}) {
		t.Fatalf("expected speeds to be accumulated, got %#v", merged.Speed)
	}
	if len(merged.Results) != 2 {
		t.Fatalf("expected merged detections after nms, got %d", len(merged.Results))
	}
	if merged.Results[0].Confidence != 0.90 || merged.Results[1].Confidence != 0.70 {
		t.Fatalf("expected detections sorted after nms, got %#v", merged.Results)
	}
}

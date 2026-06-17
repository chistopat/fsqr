package onnx

import "testing"

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

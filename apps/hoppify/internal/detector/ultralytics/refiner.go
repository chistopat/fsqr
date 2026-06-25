package ultralytics

import (
	"context"
	"fmt"
	"image"
)

type CropRefiner struct {
	client *Client
}

func NewCropRefiner(cfg Config) (*CropRefiner, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &CropRefiner{client: client}, nil
}

func (refiner *CropRefiner) RefineCrop(
	ctx context.Context,
	img image.Image,
) (refined image.Image, metadata map[string]any, applied bool, err error) {
	response, err := refiner.client.predict(ctx, img)
	if err != nil {
		return nil, nil, false, err
	}
	if len(response.Images) == 0 || len(response.Images[0].Results) == 0 {
		return img, map[string]any{"applied": false, "reason": "no_detection"}, false, nil
	}

	result := bestResult(response.Images[0].Results)
	shape := shapeFromSlice(response.Images[0].Shape)
	if shape == [2]int{} {
		shape = imageShape(img)
	}
	points, geometry, ok := result.points(shape)
	if !ok {
		return img, map[string]any{"applied": false, "reason": "no_geometry"}, false, nil
	}
	if geometry == boxGeometry {
		return img, map[string]any{"applied": false, "reason": "axis_aligned_box"}, false, nil
	}

	refined, err = extractOrientedCrop(img, points)
	if err != nil {
		return nil, nil, false, fmt.Errorf("extract refined crop: %w", err)
	}
	if !validRefinedCrop(refined) {
		return img, map[string]any{"applied": false, "reason": "invalid_refined_dimensions"}, false, nil
	}

	metadata = map[string]any{
		"applied":    true,
		"geometry":   geometry,
		"class":      result.Class,
		"name":       result.Name,
		"confidence": round5(result.Confidence),
		"points":     metadataPoints(points),
	}

	return refined, metadata, true, nil
}

func validRefinedCrop(img image.Image) bool {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 16 || height < 16 {
		return false
	}

	ratio := float64(max(width, height)) / float64(min(width, height))

	return ratio <= 8
}

func bestResult(results []predictResult) *predictResult {
	bestIndex := 0
	for index := 1; index < len(results); index++ {
		if results[index].Confidence > results[bestIndex].Confidence {
			bestIndex = index
		}
	}

	return &results[bestIndex]
}

func metadataPoints(points []point) [][]float64 {
	values := make([][]float64, 0, len(points))
	for index := range points {
		values = append(values, []float64{round5(points[index].X), round5(points[index].Y)})
	}

	return values
}

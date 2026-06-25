package ultralytics

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"
)

type point struct {
	X float64
	Y float64
}

func (result *predictResult) points(shape [2]int) ([]point, string, bool) {
	if points, ok := pointsFromRaw(result.OBB, shape); ok {
		return points, "obb", true
	}
	if points, ok := pointsFromRaw(result.XYXYXYXY, shape); ok {
		return points, "xyxyxyxy", true
	}
	if points, ok := pointsFromRaw(result.Polygon, shape); ok {
		return points, "polygon", true
	}
	if points, ok := pointsFromRaw(result.Points, shape); ok {
		return points, "points", true
	}
	if points, ok := pointsFromXYWHR(result.XYWHR, shape); ok {
		return points, "xywhr", true
	}
	if result.Box != nil {
		box := scaleNormalizedBox(*result.Box, shape)
		return pointsFromBox(box), "box", true
	}

	return nil, "", false
}

func pointsFromRaw(raw json.RawMessage, shape [2]int) ([]point, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}

	points, ok := pointsFromValue(value)
	if !ok {
		return nil, false
	}

	return scaleNormalizedPoints(points, shape), true
}

func pointsFromValue(value any) ([]point, bool) {
	switch typed := value.(type) {
	case []any:
		return pointsFromArray(typed)
	case map[string]any:
		return pointsFromMap(typed)
	default:
		return nil, false
	}
}

func pointsFromArray(values []any) ([]point, bool) {
	if len(values) == 4 {
		points := make([]point, 0, 4)
		for index := range values {
			pointValue, ok := pointFromValue(values[index])
			if !ok {
				return nil, false
			}
			points = append(points, pointValue)
		}

		return points, true
	}
	if len(values) == 8 {
		points := make([]point, 0, 4)
		for index := 0; index < len(values); index += 2 {
			x, xOK := number(values[index])
			y, yOK := number(values[index+1])
			if !xOK || !yOK {
				return nil, false
			}
			points = append(points, point{X: x, Y: y})
		}

		return points, true
	}

	return nil, false
}

func pointsFromMap(values map[string]any) ([]point, bool) {
	for _, key := range []string{"xyxyxyxy", "points", "polygon", "vertices", "corners", "obb"} {
		if child, ok := values[key]; ok {
			if points, ok := pointsFromValue(child); ok {
				return points, true
			}
		}
	}
	if points, ok := pointsFromCornerMap(values); ok {
		return points, true
	}
	if points, ok := pointsFromXYWHRMap(values); ok {
		return points, true
	}

	return nil, false
}

func pointsFromCornerMap(values map[string]any) ([]point, bool) {
	points := make([]point, 0, 4)
	for index := 1; index <= 4; index++ {
		x, xOK := number(values[fmt.Sprintf("x%d", index)])
		y, yOK := number(values[fmt.Sprintf("y%d", index)])
		if !xOK || !yOK {
			return nil, false
		}
		points = append(points, point{X: x, Y: y})
	}

	return points, true
}

func pointsFromXYWHR(raw json.RawMessage, shape [2]int) ([]point, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	points, ok := pointsFromXYWHRValue(value)
	if !ok {
		return nil, false
	}

	return scaleNormalizedPoints(points, shape), true
}

func pointsFromXYWHRValue(value any) ([]point, bool) {
	switch typed := value.(type) {
	case []any:
		if len(typed) != 5 {
			return nil, false
		}
		xywhr := make([]float64, 0, 5)
		for index := range typed {
			value, ok := number(typed[index])
			if !ok {
				return nil, false
			}
			xywhr = append(xywhr, value)
		}

		return pointsFromRotatedBox(xywhr[0], xywhr[1], xywhr[2], xywhr[3], xywhr[4]), true
	case map[string]any:
		return pointsFromXYWHRMap(typed)
	default:
		return nil, false
	}
}

func pointsFromXYWHRMap(values map[string]any) ([]point, bool) {
	centerX, xOK := firstNumber(values, "x", "cx", "center_x", "xcenter")
	centerY, yOK := firstNumber(values, "y", "cy", "center_y", "ycenter")
	width, widthOK := firstNumber(values, "w", "width")
	height, heightOK := firstNumber(values, "h", "height")
	angle, angleOK := firstNumber(values, "r", "angle", "rotation")
	if !xOK || !yOK || !widthOK || !heightOK || !angleOK {
		return nil, false
	}

	return pointsFromRotatedBox(centerX, centerY, width, height, angle), true
}

func pointsFromRotatedBox(centerX, centerY, width, height, angle float64) []point {
	if math.Abs(angle) > math.Pi*2 {
		angle = angle * math.Pi / 180
	}

	cosAngle := math.Cos(angle)
	sinAngle := math.Sin(angle)
	halfWidth := width / 2
	halfHeight := height / 2
	offsets := []point{
		{X: -halfWidth, Y: -halfHeight},
		{X: halfWidth, Y: -halfHeight},
		{X: halfWidth, Y: halfHeight},
		{X: -halfWidth, Y: halfHeight},
	}

	points := make([]point, 0, 4)
	for index := range offsets {
		points = append(points, point{
			X: centerX + offsets[index].X*cosAngle - offsets[index].Y*sinAngle,
			Y: centerY + offsets[index].X*sinAngle + offsets[index].Y*cosAngle,
		})
	}

	return points
}

func pointFromValue(value any) (point, bool) {
	switch typed := value.(type) {
	case []any:
		if len(typed) != 2 {
			return point{}, false
		}
		x, xOK := number(typed[0])
		y, yOK := number(typed[1])
		if !xOK || !yOK {
			return point{}, false
		}

		return point{X: x, Y: y}, true
	case map[string]any:
		x, xOK := firstNumber(typed, "x")
		y, yOK := firstNumber(typed, "y")
		if !xOK || !yOK {
			return point{}, false
		}

		return point{X: x, Y: y}, true
	default:
		return point{}, false
	}
}

func firstNumber(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := number(values[key]); ok {
			return value, true
		}
	}

	return 0, false
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		value, err := typed.Float64()
		return value, err == nil
	default:
		return 0, false
	}
}

func pointsFromBox(box detectionmodel.Box) []point {
	return []point{
		{X: box.X1, Y: box.Y1},
		{X: box.X2, Y: box.Y1},
		{X: box.X2, Y: box.Y2},
		{X: box.X1, Y: box.Y2},
	}
}

func scaleNormalizedPoints(points []point, shape [2]int) []point {
	if shape[0] <= 1 || shape[1] <= 1 || !pointsLookNormalized(points) {
		return points
	}

	scaled := make([]point, 0, len(points))
	for index := range points {
		scaled = append(scaled, point{
			X: points[index].X * float64(shape[1]),
			Y: points[index].Y * float64(shape[0]),
		})
	}

	return scaled
}

func pointsLookNormalized(points []point) bool {
	for index := range points {
		if points[index].X < 0 || points[index].X > 1 || points[index].Y < 0 || points[index].Y > 1 {
			return false
		}
	}

	return true
}

func scaleNormalizedBox(box detectionmodel.Box, shape [2]int) detectionmodel.Box {
	if shape[0] <= 1 || shape[1] <= 1 {
		return box
	}
	if box.X1 < 0 || box.X1 > 1 || box.Y1 < 0 || box.Y1 > 1 ||
		box.X2 < 0 || box.X2 > 1 || box.Y2 < 0 || box.Y2 > 1 {
		return box
	}

	return detectionmodel.Box{
		X1: box.X1 * float64(shape[1]),
		Y1: box.Y1 * float64(shape[0]),
		X2: box.X2 * float64(shape[1]),
		Y2: box.Y2 * float64(shape[0]),
	}
}

func boxFromPoints(points []point) detectionmodel.Box {
	x1 := points[0].X
	y1 := points[0].Y
	x2 := points[0].X
	y2 := points[0].Y
	for index := 1; index < len(points); index++ {
		x1 = math.Min(x1, points[index].X)
		y1 = math.Min(y1, points[index].Y)
		x2 = math.Max(x2, points[index].X)
		y2 = math.Max(y2, points[index].Y)
	}

	return detectionmodel.Box{X1: x1, Y1: y1, X2: x2, Y2: y2}
}

func clampBox(box detectionmodel.Box, shape [2]int) detectionmodel.Box {
	if shape[0] <= 0 || shape[1] <= 0 {
		return box
	}

	height := float64(shape[0])
	width := float64(shape[1])

	return detectionmodel.Box{
		X1: math.Max(0, math.Min(width, box.X1)),
		Y1: math.Max(0, math.Min(height, box.Y1)),
		X2: math.Max(0, math.Min(width, box.X2)),
		Y2: math.Max(0, math.Min(height, box.Y2)),
	}
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

func extractOrientedCrop(img image.Image, points []point) (image.Image, error) {
	if len(points) != 4 {
		return nil, fmt.Errorf("oriented crop requires four points")
	}

	ordered := orderQuad(points)
	width := int(math.Round(math.Max(distance(ordered[0], ordered[1]), distance(ordered[3], ordered[2]))))
	height := int(math.Round(math.Max(distance(ordered[0], ordered[3]), distance(ordered[1], ordered[2]))))
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("oriented crop has no positive area")
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		v := (float64(y) + 0.5) / float64(height)
		for x := 0; x < width; x++ {
			u := (float64(x) + 0.5) / float64(width)
			source := interpolateQuad(ordered, u, v)
			dst.SetRGBA(x, y, sampleBilinear(img, source))
		}
	}

	return dst, nil
}

func orderQuad(points []point) []point {
	ordered := make([]point, 4)
	ordered[0] = minBy(points, func(p point) float64 { return p.X + p.Y })
	ordered[2] = maxBy(points, func(p point) float64 { return p.X + p.Y })
	ordered[1] = maxBy(points, func(p point) float64 { return p.X - p.Y })
	ordered[3] = maxBy(points, func(p point) float64 { return p.Y - p.X })
	if hasDuplicatePoints(ordered) {
		return orderQuadByAngle(points)
	}

	return ordered
}

func orderQuadByAngle(points []point) []point {
	ordered := append([]point(nil), points...)
	center := centroid(points)
	sort.Slice(ordered, func(i, j int) bool {
		return math.Atan2(ordered[i].Y-center.Y, ordered[i].X-center.X) <
			math.Atan2(ordered[j].Y-center.Y, ordered[j].X-center.X)
	})

	start := 0
	for index := 1; index < len(ordered); index++ {
		if ordered[index].X+ordered[index].Y < ordered[start].X+ordered[start].Y {
			start = index
		}
	}

	return []point{
		ordered[start],
		ordered[(start+1)%4],
		ordered[(start+2)%4],
		ordered[(start+3)%4],
	}
}

func centroid(points []point) point {
	var center point
	for index := range points {
		center.X += points[index].X
		center.Y += points[index].Y
	}

	return point{X: center.X / float64(len(points)), Y: center.Y / float64(len(points))}
}

func hasDuplicatePoints(points []point) bool {
	for i := range points {
		for j := i + 1; j < len(points); j++ {
			if points[i] == points[j] {
				return true
			}
		}
	}

	return false
}

func minBy(points []point, score func(point) float64) point {
	best := points[0]
	bestScore := score(best)
	for index := 1; index < len(points); index++ {
		if currentScore := score(points[index]); currentScore < bestScore {
			best = points[index]
			bestScore = currentScore
		}
	}

	return best
}

func maxBy(points []point, score func(point) float64) point {
	best := points[0]
	bestScore := score(best)
	for index := 1; index < len(points); index++ {
		if currentScore := score(points[index]); currentScore > bestScore {
			best = points[index]
			bestScore = currentScore
		}
	}

	return best
}

func distance(a, b point) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func interpolateQuad(points []point, u, v float64) point {
	top := interpolate(points[0], points[1], u)
	bottom := interpolate(points[3], points[2], u)

	return interpolate(top, bottom, v)
}

func interpolate(a, b point, ratio float64) point {
	return point{
		X: a.X + (b.X-a.X)*ratio,
		Y: a.Y + (b.Y-a.Y)*ratio,
	}
}

func sampleBilinear(img image.Image, source point) color.RGBA {
	bounds := img.Bounds()
	x := math.Max(float64(bounds.Min.X), math.Min(float64(bounds.Max.X-1), source.X))
	y := math.Max(float64(bounds.Min.Y), math.Min(float64(bounds.Max.Y-1), source.Y))
	x1 := int(math.Floor(x))
	y1 := int(math.Floor(y))
	x2 := min(x1+1, bounds.Max.X-1)
	y2 := min(y1+1, bounds.Max.Y-1)
	dx := x - float64(x1)
	dy := y - float64(y1)

	c11 := color.RGBAModel.Convert(img.At(x1, y1)).(color.RGBA)
	c21 := color.RGBAModel.Convert(img.At(x2, y1)).(color.RGBA)
	c12 := color.RGBAModel.Convert(img.At(x1, y2)).(color.RGBA)
	c22 := color.RGBAModel.Convert(img.At(x2, y2)).(color.RGBA)

	return color.RGBA{
		R: interpolateChannel(c11.R, c21.R, c12.R, c22.R, dx, dy),
		G: interpolateChannel(c11.G, c21.G, c12.G, c22.G, dx, dy),
		B: interpolateChannel(c11.B, c21.B, c12.B, c22.B, dx, dy),
		A: interpolateChannel(c11.A, c21.A, c12.A, c22.A, dx, dy),
	}
}

func interpolateChannel(c11, c21, c12, c22 uint8, dx, dy float64) uint8 {
	top := float64(c11) + (float64(c21)-float64(c11))*dx
	bottom := float64(c12) + (float64(c22)-float64(c12))*dx

	return uint8(math.Round(top + (bottom-top)*dy))
}

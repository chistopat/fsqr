package onnx

import (
	"image"
	"image/color"
	stddraw "image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

type letterboxMetadata struct {
	OriginalShape [2]int
	Scale         float64
	PadX          float64
	PadY          float64
}

func prepareInput(img image.Image, imageSize int) ([]float32, letterboxMetadata) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	scale := math.Min(float64(imageSize)/float64(width), float64(imageSize)/float64(height))
	resizedWidth := int(math.Round(float64(width) * scale))
	resizedHeight := int(math.Round(float64(height) * scale))
	padX := (imageSize - resizedWidth) / 2
	padY := (imageSize - resizedHeight) / 2

	canvas := image.NewRGBA(image.Rect(0, 0, imageSize, imageSize))
	background := &image.Uniform{C: color.RGBA{R: 114, G: 114, B: 114, A: 255}}
	stddraw.Draw(canvas, canvas.Bounds(), background, image.Point{}, stddraw.Src)
	xdraw.ApproxBiLinear.Scale(
		canvas,
		image.Rect(padX, padY, padX+resizedWidth, padY+resizedHeight),
		img,
		bounds,
		stddraw.Src,
		nil,
	)

	return imageToNCHW(canvas), letterboxMetadata{
		OriginalShape: [2]int{height, width},
		Scale:         scale,
		PadX:          float64(padX),
		PadY:          float64(padY),
	}
}

func imageToNCHW(img *image.RGBA) []float32 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	planeSize := width * height
	data := make([]float32, 3*planeSize)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixelOffset := img.PixOffset(x, y)
			tensorOffset := y*width + x
			data[tensorOffset] = float32(img.Pix[pixelOffset]) / 255
			data[planeSize+tensorOffset] = float32(img.Pix[pixelOffset+1]) / 255
			data[2*planeSize+tensorOffset] = float32(img.Pix[pixelOffset+2]) / 255
		}
	}

	return data
}

package imageutil

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/rwcarlsen/goexif/exif"
	_ "golang.org/x/image/webp"
)

const (
	orientationNormal     = 1
	orientationFlipH      = 2
	orientationRotate180  = 3
	orientationFlipV      = 4
	orientationTranspose  = 5
	orientationRotate90   = 6
	orientationTransverse = 7
	orientationRotate270  = 8
)

func DecodeOriented(body []byte) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}

	return ApplyOrientation(img, exifOrientation(body)), format, nil
}

func ApplyOrientation(img image.Image, orientation int) image.Image {
	if orientation < orientationFlipH || orientation > orientationRotate270 {
		return img
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	dstWidth, dstHeight := orientedDimensions(width, height, orientation)
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))

	for y := range height {
		for x := range width {
			dstX, dstY := orientedPoint(x, y, width, height, orientation)
			dst.Set(dstX, dstY, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}

	return dst
}

func exifOrientation(body []byte) int {
	metadata, err := exif.Decode(bytes.NewReader(body))
	if err != nil {
		return orientationNormal
	}
	tag, err := metadata.Get(exif.Orientation)
	if err != nil {
		return orientationNormal
	}
	orientation, err := tag.Int(0)
	if err != nil {
		return orientationNormal
	}

	return orientation
}

func orientedDimensions(width, height, orientation int) (orientedWidth, orientedHeight int) {
	switch orientation {
	case orientationTranspose, orientationRotate90, orientationTransverse, orientationRotate270:
		return height, width
	default:
		return width, height
	}
}

func orientedPoint(x, y, width, height, orientation int) (dstX, dstY int) {
	switch orientation {
	case orientationFlipH:
		return width - 1 - x, y
	case orientationRotate180:
		return width - 1 - x, height - 1 - y
	case orientationFlipV:
		return x, height - 1 - y
	case orientationTranspose:
		return y, x
	case orientationRotate90:
		return height - 1 - y, x
	case orientationTransverse:
		return height - 1 - y, width - 1 - x
	case orientationRotate270:
		return y, width - 1 - x
	default:
		return x, y
	}
}

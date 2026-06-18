package imageutil

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestApplyOrientationRotatesClockwise(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 10, A: 255})
	img.Set(1, 0, color.RGBA{R: 20, A: 255})
	img.Set(0, 1, color.RGBA{R: 30, A: 255})
	img.Set(1, 1, color.RGBA{R: 40, A: 255})
	img.Set(0, 2, color.RGBA{R: 50, A: 255})
	img.Set(1, 2, color.RGBA{R: 60, A: 255})

	oriented := ApplyOrientation(img, orientationRotate90)

	if oriented.Bounds().Dx() != 3 || oriented.Bounds().Dy() != 2 {
		t.Fatalf("expected oriented dimensions 3x2, got %dx%d", oriented.Bounds().Dx(), oriented.Bounds().Dy())
	}
	assertRed(t, oriented.At(0, 0), 50)
	assertRed(t, oriented.At(2, 1), 20)
}

func TestDecodeOrientedAppliesEXIFOrientation(t *testing.T) {
	t.Parallel()

	body := jpegWithOrientation(t, orientationRotate90)

	img, format, err := DecodeOriented(body)
	if err != nil {
		t.Fatalf("decode oriented image: %v", err)
	}

	if format != "jpeg" {
		t.Fatalf("expected jpeg format, got %q", format)
	}
	if img.Bounds().Dx() != 3 || img.Bounds().Dy() != 2 {
		t.Fatalf("expected EXIF-oriented dimensions 3x2, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func assertRed(t *testing.T, c color.Color, expected uint32) {
	t.Helper()

	red, _, _, _ := c.RGBA()
	if red>>8 != expected {
		t.Fatalf("expected red channel %d, got %d", expected, red>>8)
	}
}

func jpegWithOrientation(t *testing.T, orientation int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	var body bytes.Buffer
	if err := jpeg.Encode(&body, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	return injectEXIFOrientation(t, body.Bytes(), orientation)
}

func injectEXIFOrientation(t *testing.T, jpegBody []byte, orientation int) []byte {
	t.Helper()

	if len(jpegBody) < 2 || jpegBody[0] != 0xff || jpegBody[1] != 0xd8 {
		t.Fatalf("test jpeg does not start with SOI")
	}

	payload := append([]byte("Exif\x00\x00"), tiffOrientation(orientation)...)
	length := len(payload) + 2
	var app1 bytes.Buffer
	app1.Write([]byte{0xff, 0xe1})
	_ = binary.Write(&app1, binary.BigEndian, uint16(length))
	app1.Write(payload)

	out := append([]byte{}, jpegBody[:2]...)
	out = append(out, app1.Bytes()...)
	out = append(out, jpegBody[2:]...)

	return out
}

func tiffOrientation(orientation int) []byte {
	var body bytes.Buffer
	body.Write([]byte{'I', 'I'})
	_ = binary.Write(&body, binary.LittleEndian, uint16(42))
	_ = binary.Write(&body, binary.LittleEndian, uint32(8))
	_ = binary.Write(&body, binary.LittleEndian, uint16(1))
	_ = binary.Write(&body, binary.LittleEndian, uint16(0x0112))
	_ = binary.Write(&body, binary.LittleEndian, uint16(3))
	_ = binary.Write(&body, binary.LittleEndian, uint32(1))
	_ = binary.Write(&body, binary.LittleEndian, uint16(orientation))
	_ = binary.Write(&body, binary.LittleEndian, uint16(0))
	_ = binary.Write(&body, binary.LittleEndian, uint32(0))

	return body.Bytes()
}

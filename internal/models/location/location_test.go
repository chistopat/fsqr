package location

import (
	"math"
	"testing"
)

func TestNewAcceptsValidCoordinates(t *testing.T) {
	location, err := New(34.772013, 32.429736)
	if err != nil {
		t.Fatal(err)
	}

	if !location.Valid() {
		t.Fatal("expected location to be valid")
	}
	if location.Lat() != 34.772013 {
		t.Fatalf("expected lat 34.772013, got %f", location.Lat())
	}
	if location.Lon() != 32.429736 {
		t.Fatalf("expected lon 32.429736, got %f", location.Lon())
	}
}

func TestNewRejectsInvalidCoordinates(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{
			name: "latitude too small",
			lat:  -90.1,
			lon:  32.429736,
		},
		{
			name: "latitude too big",
			lat:  90.1,
			lon:  32.429736,
		},
		{
			name: "longitude too small",
			lat:  34.772013,
			lon:  -180.1,
		},
		{
			name: "longitude too big",
			lat:  34.772013,
			lon:  180.1,
		},
		{
			name: "nan latitude",
			lat:  math.NaN(),
			lon:  32.429736,
		},
		{
			name: "infinite longitude",
			lat:  34.772013,
			lon:  math.Inf(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.lat, tt.lon)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestZeroValueIsInvalid(t *testing.T) {
	var location Location

	if location.Valid() {
		t.Fatal("expected zero-value location to be invalid")
	}
}

func TestNewBBoxAcceptsValidBounds(t *testing.T) {
	bbox, err := NewBBox(
		mustLocation(t, 34, 32),
		mustLocation(t, 35, 33),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !bbox.Valid() {
		t.Fatal("expected bbox to be valid")
	}
	if bbox.Min().Lat() != 34 {
		t.Fatalf("expected min lat 34, got %f", bbox.Min().Lat())
	}
	if bbox.Max().Lon() != 33 {
		t.Fatalf("expected max lon 33, got %f", bbox.Max().Lon())
	}
}

func TestNewBBoxRejectsInvalidBounds(t *testing.T) {
	tests := []struct {
		name string
		min  Location
		max  Location
	}{
		{
			name: "missing min",
			max:  mustLocation(t, 35, 33),
		},
		{
			name: "missing max",
			min:  mustLocation(t, 34, 32),
		},
		{
			name: "inverted latitude",
			min:  mustLocation(t, 35, 32),
			max:  mustLocation(t, 34, 33),
		},
		{
			name: "inverted longitude",
			min:  mustLocation(t, 34, 33),
			max:  mustLocation(t, 35, 32),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBBox(tt.min, tt.max)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewBBoxAroundBuildsSquareAroundCenter(t *testing.T) {
	center := mustLocation(t, 0, 0)

	bbox, err := NewBBoxAround(center, 5000)
	if err != nil {
		t.Fatal(err)
	}

	assertApprox(t, bbox.Min().Lat(), -0.044966, 0.0001)
	assertApprox(t, bbox.Max().Lat(), 0.044966, 0.0001)
	assertApprox(t, bbox.Min().Lon(), -0.044966, 0.0001)
	assertApprox(t, bbox.Max().Lon(), 0.044966, 0.0001)
	if bbox.CrossesAntimeridian() {
		t.Fatal("expected bbox not to cross antimeridian")
	}
	assertApprox(t, bbox.LonDeltaDegrees(), 0.044966, 0.0001)
}

func TestNewBBoxAroundExpandsLongitudeAtAntimeridian(t *testing.T) {
	center := mustLocation(t, 0, 179.99)

	bbox, err := NewBBoxAround(center, 5000)
	if err != nil {
		t.Fatal(err)
	}

	if bbox.Min().Lon() != -180 {
		t.Fatalf("expected min lon -180, got %f", bbox.Min().Lon())
	}
	if bbox.Max().Lon() != 180 {
		t.Fatalf("expected max lon 180, got %f", bbox.Max().Lon())
	}
	if !bbox.CrossesAntimeridian() {
		t.Fatal("expected bbox to cross antimeridian")
	}
	assertApprox(t, bbox.LonDeltaDegrees(), 0.044966, 0.0001)
}

func TestNewBBoxAroundRejectsInvalidDistance(t *testing.T) {
	_, err := NewBBoxAround(mustLocation(t, 0, 0), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestZeroValueBBoxIsInvalid(t *testing.T) {
	var bbox BBox

	if bbox.Valid() {
		t.Fatal("expected zero-value bbox to be invalid")
	}
}

func assertApprox(t *testing.T, actual float64, expected float64, tolerance float64) {
	t.Helper()

	if math.Abs(actual-expected) > tolerance {
		t.Fatalf("expected %f within %f, got %f", expected, tolerance, actual)
	}
}

func mustLocation(t *testing.T, lat float64, lon float64) Location {
	t.Helper()

	location, err := New(lat, lon)
	if err != nil {
		t.Fatal(err)
	}

	return location
}

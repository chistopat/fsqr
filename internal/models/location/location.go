package location

import (
	"fmt"
	"math"
)

const earthRadiusMeters = 6371008.8

type Location struct {
	lat   float64
	lon   float64
	valid bool
}

func New(lat, lon float64) (Location, error) {
	if !validLatitude(lat) {
		return Location{}, fmt.Errorf("latitude must be between -90 and 90")
	}
	if !validLongitude(lon) {
		return Location{}, fmt.Errorf("longitude must be between -180 and 180")
	}

	return Location{
		lat:   lat,
		lon:   lon,
		valid: true,
	}, nil
}

func (location Location) Lat() float64 {
	return location.lat
}

func (location Location) Lon() float64 {
	return location.lon
}

func (location Location) Valid() bool {
	return location.valid
}

type BBox struct {
	min                 Location
	max                 Location
	crossesAntimeridian bool
	lonDeltaDegrees     float64
}

func NewBBox(minLocation, maxLocation Location) (BBox, error) {
	if !minLocation.Valid() {
		return BBox{}, fmt.Errorf("bbox min location is required")
	}
	if !maxLocation.Valid() {
		return BBox{}, fmt.Errorf("bbox max location is required")
	}
	if minLocation.Lat() > maxLocation.Lat() {
		return BBox{}, fmt.Errorf("bbox min latitude must be less than or equal to max latitude")
	}
	if minLocation.Lon() > maxLocation.Lon() {
		return BBox{}, fmt.Errorf("bbox min longitude must be less than or equal to max longitude")
	}

	return BBox{
		min:             minLocation,
		max:             maxLocation,
		lonDeltaDegrees: math.Abs(maxLocation.Lon()-minLocation.Lon()) / 2,
	}, nil
}

func NewBBoxAround(center Location, halfSideMeters float64) (BBox, error) {
	if !center.Valid() {
		return BBox{}, fmt.Errorf("bbox center location is required")
	}
	if math.IsNaN(halfSideMeters) || math.IsInf(halfSideMeters, 0) || halfSideMeters <= 0 {
		return BBox{}, fmt.Errorf("bbox half side must be positive")
	}

	angularDistance := halfSideMeters / earthRadiusMeters
	latDelta := radiansToDegrees(angularDistance)
	minLat := clamp(center.Lat()-latDelta, -90, 90)
	maxLat := clamp(center.Lat()+latDelta, -90, 90)

	minLon := -180.0
	maxLon := 180.0
	crossesAntimeridian := false
	lonDeltaDegrees := 180.0
	cosLat := math.Cos(degreesToRadians(center.Lat()))
	if math.Abs(cosLat) > 1e-12 {
		lonDelta := radiansToDegrees(angularDistance / cosLat)
		lonDeltaDegrees = math.Min(lonDelta, 180)
		if lonDelta < 180 {
			rawMinLon := center.Lon() - lonDelta
			rawMaxLon := center.Lon() + lonDelta
			if rawMinLon >= -180 && rawMaxLon <= 180 {
				minLon = rawMinLon
				maxLon = rawMaxLon
			} else {
				crossesAntimeridian = true
			}
		}
	}

	minLocation, err := New(minLat, minLon)
	if err != nil {
		return BBox{}, err
	}
	maxLocation, err := New(maxLat, maxLon)
	if err != nil {
		return BBox{}, err
	}

	bbox, err := NewBBox(minLocation, maxLocation)
	if err != nil {
		return BBox{}, err
	}
	bbox.crossesAntimeridian = crossesAntimeridian
	bbox.lonDeltaDegrees = lonDeltaDegrees

	return bbox, nil
}

func (bbox BBox) Min() Location {
	return bbox.min
}

func (bbox BBox) Max() Location {
	return bbox.max
}

func (bbox BBox) Valid() bool {
	return bbox.min.Valid() && bbox.max.Valid()
}

func (bbox BBox) CrossesAntimeridian() bool {
	return bbox.crossesAntimeridian
}

func (bbox BBox) LonDeltaDegrees() float64 {
	return bbox.lonDeltaDegrees
}

func validLatitude(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -90 && value <= 90
}

func validLongitude(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -180 && value <= 180
}

func degreesToRadians(value float64) float64 {
	return value * math.Pi / 180
}

func radiansToDegrees(value float64) float64 {
	return value * 180 / math.Pi
}

func clamp(value, minValue, maxValue float64) float64 {
	return math.Min(math.Max(value, minValue), maxValue)
}

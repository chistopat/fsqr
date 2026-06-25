package crops

import (
	"context"
	"fmt"
	"image"
)

type unavailableRefiner struct {
	err error
}

func NewUnavailableRefiner(err error) Refiner {
	return &unavailableRefiner{err: err}
}

func (refiner *unavailableRefiner) RefineCrop(
	_ context.Context,
	_ image.Image,
) (refined image.Image, metadata map[string]any, applied bool, err error) {
	if refiner.err == nil {
		return nil, nil, false, fmt.Errorf("crop refiner unavailable")
	}

	return nil, nil, false, fmt.Errorf("crop refiner unavailable: %w", refiner.err)
}

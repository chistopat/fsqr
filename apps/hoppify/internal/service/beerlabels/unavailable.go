package beerlabels

import (
	"context"
	"fmt"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
)

type unavailableRecognizer struct {
	model         string
	promptVersion string
	err           error
}

func NewUnavailableRecognizer(model, promptVersion string, err error) Recognizer {
	return &unavailableRecognizer{model: model, promptVersion: promptVersion, err: err}
}

func (recognizer *unavailableRecognizer) IdentifyBeerLabel(
	_ context.Context,
	_ []byte,
) (beerlabelmodel.Result, error) {
	if recognizer.err == nil {
		return beerlabelmodel.Result{}, ErrModelUnavailable
	}

	return beerlabelmodel.Result{}, fmt.Errorf("%w: %w", ErrModelUnavailable, recognizer.err)
}

func (recognizer *unavailableRecognizer) Model() string {
	return recognizer.model
}

func (recognizer *unavailableRecognizer) PromptVersion() string {
	return recognizer.promptVersion
}

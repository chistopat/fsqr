package scorer

import (
	"math"
	"testing"

	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	"github.com/chistopat/fsqr/internal/models/category/level"
	searchmodel "github.com/chistopat/fsqr/internal/models/search"
)

func TestRRFUsesReciprocalRanks(t *testing.T) {
	scorer := RRF{
		K:       10,
		Weights: []float64{2, 1},
	}

	scored := scorer.Score(
		[]searchmodel.Document{
			searchmodel.NewDocument(category(1, level.Min), 1, 0),
		},
		[]searchmodel.Document{
			searchmodel.NewDocument(category(1, level.Min), 2, 0),
		},
	)

	expected := 2.0/(10+1) + 1.0/(10+2)
	if math.Abs(scored[0].Score-expected) > 0.0000001 {
		t.Fatalf("expected RRF score %f, got %f", expected, scored[0].Score)
	}
}

func TestRRFAppliesCategoryLevelBoost(t *testing.T) {
	category := category(1, level.Max)
	document := searchmodel.NewDocument(
		category,
		1,
		0,
	)
	scorer := RRF{
		K:           10,
		LevelWeight: 1,
	}

	scored := scorer.Score([]searchmodel.Document{document})

	expected := (1.0 / (10 + 1)) * (1 + category.Level.Normalized())
	if math.Abs(scored[0].Score-expected) > 0.0000001 {
		t.Fatalf("expected boosted score %f, got %f", expected, scored[0].Score)
	}
}

func category(id int64, categoryLevel int) categorymodel.Category {
	decodedLevel, err := level.New(categoryLevel)
	if err != nil {
		panic(err)
	}

	return categorymodel.Category{
		ID:    id,
		Level: decodedLevel,
	}
}

package normalizer

import (
	"testing"

	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	searchmodel "github.com/chistopat/fsqr/internal/models/search"
)

func TestMinMaxNormalizesScores(t *testing.T) {
	documents := []searchmodel.Document{
		minmaxDocument(1, 2),
		minmaxDocument(2, 4),
		minmaxDocument(3, 6),
	}

	normalized := NewMinMax().Normalize(documents)

	assertScore(t, normalized[0], 0)
	assertScore(t, normalized[1], 0.5)
	assertScore(t, normalized[2], 1)
}

func TestMinMaxNormalizesFlatNonZeroScoresToOne(t *testing.T) {
	documents := []searchmodel.Document{
		minmaxDocument(1, 3),
		minmaxDocument(2, 3),
	}

	normalized := NewMinMax().Normalize(documents)

	assertScore(t, normalized[0], 1)
	assertScore(t, normalized[1], 1)
}

func TestMinMaxNormalizesEmptyScoresToZero(t *testing.T) {
	if got := normalizeScore(3, 0, 0, false); got != 0 {
		t.Fatalf("expected empty normalizer to return 0, got %f", got)
	}
}

func minmaxDocument(id int64, score float64) searchmodel.Document {
	return searchmodel.NewDocument(
		categorymodel.Category{
			ID: id,
		},
		int(id),
		score,
	)
}

func assertScore(
	t *testing.T,
	document searchmodel.Document,
	expected float64,
) {
	t.Helper()

	if document.Score != expected {
		t.Fatalf("expected score %f, got %f", expected, document.Score)
	}
}

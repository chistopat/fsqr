package ranker

import (
	"testing"

	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	searchmodel "github.com/chistopat/fsqr/internal/models/search"
)

func TestByScoreOrdersByScoreThenRankThenID(t *testing.T) {
	documents := []searchmodel.Document{
		rankerDocument(3, 0.5, 1),
		rankerDocument(2, 1, 3),
		rankerDocument(4, 1, 1),
		rankerDocument(1, 1, 1),
	}

	ranked := NewByScore().Rank(documents)

	assertOrder(t, ranked, []int64{1, 4, 2, 3})
}

func rankerDocument(id int64, score float64, rank int) searchmodel.Document {
	return searchmodel.NewDocument(
		categorymodel.Category{
			ID: id,
		},
		rank,
		score,
	)
}

func assertOrder(t *testing.T, documents []searchmodel.Document, expected []int64) {
	t.Helper()

	if len(documents) != len(expected) {
		t.Fatalf("expected %d documents, got %d", len(expected), len(documents))
	}
	for index, id := range expected {
		if documents[index].Category.ID != id {
			t.Fatalf("expected document %d at index %d, got %d", id, index, documents[index].Category.ID)
		}
	}
}

package search

import (
	"testing"

	categorymodel "github.com/chistopat/fsqr/internal/models/category"
)

func TestDocumentsFilterByRelativeScore(t *testing.T) {
	documents := Documents{
		testDocument(1, 1),
		testDocument(2, 0.5),
		testDocument(3, 0.49),
	}

	filtered := documents.FilterByRelativeScore(0.5)

	assertDocumentIDs(t, filtered, []int64{1, 2})
}

func TestDocumentsFilterByRelativeScoreKeepsDocumentsWhenTopScoreIsNotPositive(t *testing.T) {
	documents := Documents{
		testDocument(1, 0),
		testDocument(2, -1),
	}

	filtered := documents.FilterByRelativeScore(0.5)

	assertDocumentIDs(t, filtered, []int64{1, 2})
}

func TestDocumentsLimit(t *testing.T) {
	documents := Documents{
		testDocument(1, 1),
		testDocument(2, 0.5),
		testDocument(3, 0.25),
	}

	limited := documents.Limit(2)

	assertDocumentIDs(t, limited, []int64{1, 2})
}

func TestDocumentsCategories(t *testing.T) {
	documents := Documents{
		testDocument(1, 1),
		testDocument(2, 0.5),
	}

	categories := documents.Categories()

	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	if categories[0].ID != 1 || categories[1].ID != 2 {
		t.Fatalf("expected category ids [1 2], got [%d %d]", categories[0].ID, categories[1].ID)
	}
}

func testDocument(id int64, score float64) Document {
	return NewDocument(categorymodel.Category{ID: id}, int(id), score)
}

func assertDocumentIDs(t *testing.T, documents Documents, expected []int64) {
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

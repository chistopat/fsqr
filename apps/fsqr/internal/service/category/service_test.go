package category

import (
	"context"
	"testing"

	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	"github.com/chistopat/fsqr/internal/models/category/level"
	embeddingmodel "github.com/chistopat/fsqr/internal/models/embedding"
	searchmodel "github.com/chistopat/fsqr/internal/models/search"
	querymodel "github.com/chistopat/fsqr/internal/models/search/query"
)

func TestSearchCategoriesFusesTextAndVectorDocuments(t *testing.T) {
	categoryLevel, err := level.New(3)
	if err != nil {
		t.Fatal(err)
	}
	embedding, err := embeddingmodel.New(make([]float32, embeddingmodel.Dimensions))
	if err != nil {
		t.Fatal(err)
	}

	pharmacy := categorymodel.Category{ID: 10, Name: "Pharmacy", Level: categoryLevel}
	repo := &stubCategoryRepository{
		text: []searchmodel.Document{
			document(categorymodel.Category{ID: 1, Name: "Clinic", Level: categoryLevel}, 1, 0.9),
			document(pharmacy, 2, 0.8),
		},
		vector: []searchmodel.Document{
			document(pharmacy, 1, 0.9),
			document(categorymodel.Category{ID: 11, Name: "Drugstore", Level: categoryLevel}, 2, 0.7),
		},
	}
	service := New(repo, stubEmbedder{embedding: embedding})
	query, err := querymodel.New("pharmacy", 2)
	if err != nil {
		t.Fatal(err)
	}

	categories, err := service.SearchCategories(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}

	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	if categories[0].ID != pharmacy.ID {
		t.Fatalf("expected pharmacy first, got %d", categories[0].ID)
	}
}

func TestSearchCategoriesDropsLowConfidenceTail(t *testing.T) {
	categoryLevel, err := level.New(2)
	if err != nil {
		t.Fatal(err)
	}
	higherCategoryLevel, err := level.New(3)
	if err != nil {
		t.Fatal(err)
	}
	embedding, err := embeddingmodel.New(make([]float32, embeddingmodel.Dimensions))
	if err != nil {
		t.Fatal(err)
	}

	fuelStation := categorymodel.Category{ID: 443, Name: "Fuel Station", Level: categoryLevel}
	repo := &stubCategoryRepository{
		text: []searchmodel.Document{
			document(fuelStation, 1, 1),
		},
		vector: []searchmodel.Document{
			document(fuelStation, 1, 1),
			document(categorymodel.Category{ID: 498, Name: "Vehicle Inspection Station", Level: higherCategoryLevel}, 3, 0.8),
		},
	}
	service := New(repo, stubEmbedder{embedding: embedding})
	query, err := querymodel.New("gas station", 3)
	if err != nil {
		t.Fatal(err)
	}

	categories, err := service.SearchCategories(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}

	if len(categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(categories))
	}
	if categories[0].ID != fuelStation.ID {
		t.Fatalf("expected fuel station, got %d", categories[0].ID)
	}
}

type stubCategoryRepository struct {
	text   []searchmodel.Document
	vector []searchmodel.Document
}

func (repo *stubCategoryRepository) SearchText(
	_ context.Context,
	_ string,
	_ int,
) ([]searchmodel.Document, error) {
	return repo.text, nil
}

func (repo *stubCategoryRepository) SearchVector(
	_ context.Context,
	_ embeddingmodel.Embedding,
	_ int,
) ([]searchmodel.Document, error) {
	return repo.vector, nil
}

type stubEmbedder struct {
	embedding embeddingmodel.Embedding
}

func (embedder stubEmbedder) EmbedQuery(
	_ context.Context,
	_ querymodel.Query,
) (embeddingmodel.Embedding, error) {
	return embedder.embedding, nil
}

func document(
	category categorymodel.Category,
	rank int,
	score float64,
) searchmodel.Document {
	return searchmodel.NewDocument(
		category,
		rank,
		score,
	)
}

package categories

import "testing"

func TestMapVectorDocumentComputesRankAndScore(t *testing.T) {
	document, err := mapVectorDocument(row{
		ID:            441,
		CategoryID:    "5032872391d4c4b30a586d64",
		CategoryName:  "Electric Vehicle Charging Station",
		CategoryLabel: "Travel and Transportation > Electric Vehicle Charging Station",
		CategoryLevel: 2,
		Distance:      0.25,
	}, 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if document.Rank != 3 {
		t.Fatalf("expected rank 3, got %d", document.Rank)
	}
	if document.Score != 0.75 {
		t.Fatalf("expected score 0.75, got %f", document.Score)
	}
	if document.Category.ID != 441 {
		t.Fatalf("expected category id 441, got %d", document.Category.ID)
	}
}

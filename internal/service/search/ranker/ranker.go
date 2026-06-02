package ranker

import (
	"cmp"
	"slices"

	searchmodel "github.com/chistopat/fsqr/internal/models/search"
)

type ByScore struct{}

func NewByScore() ByScore {
	return ByScore{}
}

func (ranker ByScore) Rank(documents []searchmodel.Document) []searchmodel.Document {
	ranked := make([]searchmodel.Document, len(documents))
	copy(ranked, documents)

	slices.SortFunc(ranked, compareDocuments)

	return ranked
}

func compareDocuments(left, right searchmodel.Document) int {
	if byScore := cmp.Compare(right.Score, left.Score); byScore != 0 {
		return byScore
	}

	if byRank := compareRank(left.Rank, right.Rank); byRank != 0 {
		return byRank
	}

	return cmp.Compare(left.Category.ID, right.Category.ID)
}

func compareRank(left, right int) int {
	switch {
	case left <= 0 && right <= 0:
		return 0
	case left <= 0:
		return 1
	case right <= 0:
		return -1
	default:
		return cmp.Compare(left, right)
	}
}

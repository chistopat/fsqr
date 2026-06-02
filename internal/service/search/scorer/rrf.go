package scorer

import searchmodel "github.com/chistopat/fsqr/internal/models/search"

const (
	DefaultRRFK        = 10
	DefaultLevelWeight = 1
)

type RRF struct {
	K           float64
	Weights     []float64
	LevelWeight float64
}

func NewRRF() RRF {
	return RRF{
		K:           DefaultRRFK,
		LevelWeight: DefaultLevelWeight,
	}
}

func (scorer RRF) Score(groups ...[]searchmodel.Document) []searchmodel.Document {
	byID := make(map[int64]searchmodel.Document)
	for groupIndex, group := range groups {
		for _, document := range group {
			scored, found := byID[document.Category.ID]
			if !found {
				scored = searchmodel.Document{
					Category: document.Category,
				}
			}

			scored.Rank = bestRank(scored.Rank, document.Rank)
			scored.Score += scorer.reciprocalRankScore(groupIndex, document)
			byID[document.Category.ID] = scored
		}
	}

	documents := make([]searchmodel.Document, 0, len(byID))
	for _, document := range byID {
		document.Score *= scorer.categoryLevelBoost(document)
		documents = append(documents, document)
	}

	return documents
}

func (scorer RRF) reciprocalRankScore(groupIndex int, document searchmodel.Document) float64 {
	if document.Rank <= 0 {
		return 0
	}

	return scorer.weight(groupIndex) / (scorer.k() + float64(document.Rank))
}

func (scorer RRF) categoryLevelBoost(document searchmodel.Document) float64 {
	return 1 + scorer.LevelWeight*document.Category.Level.Normalized()
}

func (scorer RRF) k() float64 {
	if scorer.K <= 0 {
		return DefaultRRFK
	}

	return scorer.K
}

func (scorer RRF) weight(groupIndex int) float64 {
	if len(scorer.Weights) <= groupIndex {
		return 1
	}

	return scorer.Weights[groupIndex]
}

func bestRank(left, right int) int {
	switch {
	case left <= 0:
		return right
	case right <= 0:
		return left
	case right < left:
		return right
	default:
		return left
	}
}

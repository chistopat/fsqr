package normalizer

import searchmodel "github.com/chistopat/fsqr/internal/models/search"

type MinMax struct{}

func NewMinMax() MinMax {
	return MinMax{}
}

func (normalizer MinMax) Normalize(documents []searchmodel.Document) []searchmodel.Document {
	normalized := make([]searchmodel.Document, len(documents))
	copy(normalized, documents)

	minScore, maxScore, ok := normalizer.bounds(documents)
	for index := range normalized {
		normalized[index].Score = normalizeScore(normalized[index].Score, minScore, maxScore, ok)
	}

	return normalized
}

func (normalizer MinMax) bounds(documents []searchmodel.Document) (minScore, maxScore float64, ok bool) {
	for _, document := range documents {
		if !ok {
			minScore = document.Score
			maxScore = document.Score
			ok = true
			continue
		}

		if document.Score < minScore {
			minScore = document.Score
		}
		if document.Score > maxScore {
			maxScore = document.Score
		}
	}

	return minScore, maxScore, ok
}

func normalizeScore(score, minScore, maxScore float64, ok bool) float64 {
	if !ok {
		return 0
	}
	if maxScore == minScore {
		if maxScore == 0 {
			return 0
		}

		return 1
	}

	return (score - minScore) / (maxScore - minScore)
}

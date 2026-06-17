package search

import categorymodel "github.com/chistopat/fsqr/internal/models/category"

type Document struct {
	Category categorymodel.Category
	Rank     int
	Score    float64
}

type Documents []Document

func NewDocument(category categorymodel.Category, rank int, score float64) Document {
	return Document{
		Category: category,
		Rank:     rank,
		Score:    score,
	}
}

func (documents Documents) FilterByRelativeScore(minScoreRatio float64) Documents {
	if len(documents) == 0 || minScoreRatio <= 0 {
		return documents
	}

	topScore := documents[0].Score
	if topScore <= 0 {
		return documents
	}

	cutoff := topScore * minScoreRatio
	filtered := documents[:0]
	for _, document := range documents {
		if document.Score < cutoff {
			continue
		}

		filtered = append(filtered, document)
	}

	return filtered
}

func (documents Documents) Limit(limit int) Documents {
	if len(documents) <= limit {
		return documents
	}

	return documents[:limit]
}

func (documents Documents) Categories() []categorymodel.Category {
	categories := make([]categorymodel.Category, 0, len(documents))
	for _, document := range documents {
		categories = append(categories, document.Category)
	}

	return categories
}

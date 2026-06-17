package category

import "github.com/chistopat/fsqr/internal/models/category/level"

type Category struct {
	ID            int64       `json:"id"`
	FSQCategoryID string      `json:"fsq_category_id"`
	Name          string      `json:"name"`
	Label         string      `json:"label"`
	Level         level.Level `json:"level"`
}

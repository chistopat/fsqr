package categories

type row struct {
	ID            int64   `db:"id"`
	CategoryID    string  `db:"category_id"`
	CategoryName  string  `db:"category_name"`
	CategoryLabel string  `db:"category_label"`
	CategoryLevel int     `db:"category_level"`
	Rank          int     `db:"rank"`
	Score         float64 `db:"score"`
	Distance      float64 `db:"distance"`
}

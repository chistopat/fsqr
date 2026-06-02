package places

import "database/sql"

type row struct {
	FSQPlaceID string         `db:"fsq_place_id"`
	Name       sql.NullString `db:"name"`
	CategoryID int64          `db:"category_id"`
	Lat        float64        `db:"lat"`
	Lon        float64        `db:"lon"`
	Distance   float64        `db:"dist"`
}

type detailRow struct {
	UUID                  string         `db:"uuid"`
	Name                  sql.NullString `db:"name"`
	Lat                   float64        `db:"lat"`
	Lon                   float64        `db:"lon"`
	CategoryFSQCategoryID string         `db:"category_fsq_category_id"`
	CategoryName          string         `db:"category_name"`
	CategoryPath          string         `db:"category_path"`
	Address               sql.NullString `db:"address"`
	Locality              sql.NullString `db:"locality"`
	Region                sql.NullString `db:"region"`
	Country               sql.NullString `db:"country"`
	Tel                   sql.NullString `db:"tel"`
	Website               sql.NullString `db:"website"`
	Email                 sql.NullString `db:"email"`
	FacebookID            sql.NullInt64  `db:"facebook_id"`
	Instagram             sql.NullString `db:"instagram"`
	Twitter               sql.NullString `db:"twitter"`
}

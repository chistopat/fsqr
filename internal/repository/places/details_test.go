package places

import (
	"database/sql"
	"testing"
)

func TestMapPlaceDetailsMapsOptionalFields(t *testing.T) {
	facebookID := int64(123)
	row := detailRow{
		UUID:                  "fsq-1",
		Name:                  sql.NullString{String: "Example Coffee", Valid: true},
		Lat:                   34.772013,
		Lon:                   32.429736,
		CategoryFSQCategoryID: "cat-1",
		CategoryName:          "Coffee Shop",
		CategoryPath:          "Dining and Drinking > Coffee Shop",
		Address:               sql.NullString{String: "12 Example Street", Valid: true},
		Locality:              sql.NullString{String: "Paphos", Valid: true},
		Country:               sql.NullString{String: "CY", Valid: true},
		Tel:                   sql.NullString{String: "+35726000000", Valid: true},
		FacebookID:            sql.NullInt64{Int64: facebookID, Valid: true},
	}

	place := mapPlaceDetails(&row)

	if place.UUID != "fsq-1" {
		t.Fatalf("expected fsq-1, got %q", place.UUID)
	}
	if place.Category == nil || place.Category.FSQCategoryID != "cat-1" {
		t.Fatalf("expected category cat-1, got %#v", place.Category)
	}
	if place.Address == nil || place.Address.Line == nil || *place.Address.Line != "12 Example Street" {
		t.Fatalf("expected mapped address, got %#v", place.Address)
	}
	if place.Contacts == nil || place.Contacts.FacebookID == nil || *place.Contacts.FacebookID != facebookID {
		t.Fatalf("expected mapped contacts, got %#v", place.Contacts)
	}
}

func TestMapPlaceDetailsOmitsEmptyOptionalGroups(t *testing.T) {
	row := detailRow{
		UUID:                  "fsq-1",
		Name:                  sql.NullString{String: "Example Coffee", Valid: true},
		Lat:                   34.772013,
		Lon:                   32.429736,
		CategoryFSQCategoryID: "cat-1",
		CategoryName:          "Coffee Shop",
		CategoryPath:          "Dining and Drinking > Coffee Shop",
	}

	place := mapPlaceDetails(&row)

	if place.Address != nil {
		t.Fatalf("expected nil address, got %#v", place.Address)
	}
	if place.Contacts != nil {
		t.Fatalf("expected nil contacts, got %#v", place.Contacts)
	}
}

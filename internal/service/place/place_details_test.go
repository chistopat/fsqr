package place

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chistopat/fsqr/internal/models"
	placesrepo "github.com/chistopat/fsqr/internal/repository/places"
)

func TestPlaceDetailsReturnsPlace(t *testing.T) {
	repo := &stubPlaceDetailsRepository{
		place: models.PlaceDetails{
			UUID: "fsq-1",
			Name: "Example Coffee",
			Lat:  34.772013,
			Lon:  32.429736,
		},
	}
	service := NewPlaceDetails(repo)

	place, err := service.GetPlace(context.Background(), " fsq-1 ")
	if err != nil {
		t.Fatal(err)
	}

	if repo.uuid != "fsq-1" {
		t.Fatalf("expected normalized uuid fsq-1, got %q", repo.uuid)
	}
	if place.UUID != "fsq-1" {
		t.Fatalf("expected fsq-1, got %q", place.UUID)
	}
}

func TestPlaceDetailsRejectsInvalidUUID(t *testing.T) {
	service := NewPlaceDetails(&stubPlaceDetailsRepository{})

	_, err := service.GetPlace(context.Background(), "")
	if !IsInvalidPlaceInput(err) {
		t.Fatalf("expected invalid place input error, got %v", err)
	}

	_, err = service.GetPlace(context.Background(), strings.Repeat("x", MaxPlaceUUIDRunes+1))
	if !IsInvalidPlaceInput(err) {
		t.Fatalf("expected invalid place input error, got %v", err)
	}
}

func TestPlaceDetailsMapsNotFound(t *testing.T) {
	service := NewPlaceDetails(&stubPlaceDetailsRepository{err: placesrepo.ErrPlaceNotFound})

	_, err := service.GetPlace(context.Background(), "missing")
	if !IsPlaceNotFound(err) {
		t.Fatalf("expected place not found error, got %v", err)
	}
}

func TestPlaceDetailsReturnsRepositoryError(t *testing.T) {
	expected := errors.New("db failed")
	service := NewPlaceDetails(&stubPlaceDetailsRepository{err: expected})

	_, err := service.GetPlace(context.Background(), "fsq-1")
	if !errors.Is(err, expected) {
		t.Fatalf("expected repository error, got %v", err)
	}
}

type stubPlaceDetailsRepository struct {
	place models.PlaceDetails
	uuid  string
	err   error
}

func (repo *stubPlaceDetailsRepository) GetByUUID(
	_ context.Context,
	uuid string,
) (models.PlaceDetails, error) {
	repo.uuid = uuid
	if repo.err != nil {
		return models.PlaceDetails{}, repo.err
	}

	return repo.place, nil
}

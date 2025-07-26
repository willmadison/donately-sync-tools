package donately

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/tursodatabase/libsql-client-go/libsql"

	"github.com/willmadison/donately-sync-tools/donately/internal/sqlite/donors"
)

type Adjustment struct {
	DisplayName string  `json:"name"`
	Slug        string  `json:"slug"`
	Amount      float64 `json:"amount"`
}

type Donor struct {
	Person      Person       `json:"person"`
	Donations   []Donation   `json:"donations"`
	Adjustments []Adjustment `json:"adjustments"`
}

type AdjustmentStore interface {
	GetAdustmentsByPerson(context.Context, Person) ([]Adjustment, error)
	SaveAdjustments(context.Context, Person, []Adjustment) error
}

type defaultAdjustmentStore struct {
	queries *donors.Queries
}

func (d defaultAdjustmentStore) GetAdustmentsByPerson(ctx context.Context, person Person) ([]Adjustment, error) {
	var adjustments []Adjustment

	rawAdjustments, err := d.queries.GetDonorAdjustmentsByPerson(ctx, sql.NullString{String: person.ID, Valid: true})
	if err != nil {
		return adjustments, fmt.Errorf("encountered an error fetching adjustments: %s", err)
	}

	for _, adjustment := range rawAdjustments {
		adjustments = append(adjustments, asAdjustment(adjustment))
	}

	return adjustments, nil
}

func asAdjustment(adjustment donors.DonorAdjustment) Adjustment {
	return Adjustment{
		DisplayName: adjustment.DisplayName.String,
		Slug:        adjustment.Slug.String,
		Amount:      adjustment.Amount.Float64,
	}
}

func (d defaultAdjustmentStore) SaveAdjustments(ctx context.Context, person Person, adjustments []Adjustment) error {
	for _, adjustment := range adjustments {
		_, err := d.queries.SaveDonorAdjustment(ctx, donors.SaveDonorAdjustmentParams{
			PersonID:    sql.NullString{String: person.ID, Valid: true},
			Slug:        sql.NullString{String: adjustment.Slug, Valid: true},
			DisplayName: sql.NullString{String: adjustment.DisplayName, Valid: true},
			Amount:      sql.NullFloat64{Float64: adjustment.Amount, Valid: true},
		})

		if err != nil {
			return fmt.Errorf("encountered an error persisting a donor adjustment: %s", err)
		}
	}

	return nil
}

func NewAdjustmentStore() (AdjustmentStore, error) {
	databaseURL := os.Getenv("DATABASE_URL")

	var driver string

	switch {
	case strings.HasPrefix(databaseURL, "libsql://"):
		driver = "libsql"
	case strings.HasPrefix(databaseURL, "file:"):
		driver = "sqlite3"
	default:
		return nil, fmt.Errorf("unsupported DATABASE_URL: %s", databaseURL)
	}

	db, err := sql.Open(driver, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("encountered an error connecting to the database: %s", err)
	}

	queries := donors.New(db)

	return defaultAdjustmentStore{queries}, nil
}

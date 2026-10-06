package tree

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Distributor assignment management.
//
// Distributor is NOT a column on a brand or a model: it is an effective-dated
// assignment on (brand_id, car_type), and analytics resolves it per fact period
// through the LATERAL join in analytics.joinBlock. Nothing is stored on a fact
// (§2.5), so an assignment written here is visible across all history for the
// periods it covers the moment it commits.
//
// Until now these rows existed only in the seed, so a brand created after the
// seed had no way to get a distributor short of raw SQL.
//
// A missing assignment is a valid permanent state — distributor is optional and
// modelled as the absence of a row, never a sentinel (§2.4).

// Assignment is one effective-dated (brand, car_type) → distributor row.
type Assignment struct {
	ID              int64      `json:"id"`
	CarType         string     `json:"car_type"`
	DistributorID   int64      `json:"distributor_id"`
	DistributorName string     `json:"distributor_name"`
	ValidFrom       time.Time  `json:"valid_from"`
	ValidTo         *time.Time `json:"valid_to"`
	// Units is the volume of the brand's facts in this car_type that fall inside
	// this row's date range — what the assignment actually governs.
	Units int64 `json:"units"`
}

// CreateDistributor adds a distributor (an importer / agent company). The §0.1
// casing rule is deliberately NOT applied: it governs brand and model names, not
// company names, and would mangle "GB Auto" or "MTI".
func CreateDistributor(ctx context.Context, pool *pgxpool.Pool, name string, actorID int64) (int64, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, "", fmt.Errorf("distributor name required")
	}
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO distributors (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, "", fmt.Errorf("a distributor named %q already exists", name)
		}
		return 0, "", fmt.Errorf("create distributor: %w", err)
	}
	_ = logChange(ctx, pool, "distributor", id, "create", actorID, 0)
	return id, name, nil
}

// BrandAssignments lists a brand's distributor assignments, newest range first
// within each car type, with the volume each one governs.
func BrandAssignments(ctx context.Context, pool *pgxpool.Pool, brandID int64) ([]Assignment, error) {
	rows, err := pool.Query(ctx, `
		SELECT da.id, da.car_type, da.distributor_id, d.name, da.valid_from, da.valid_to,
		       COALESCE((
		         SELECT sum(f.volume) FROM facts f
		           JOIN models m ON m.id = f.model_id
		          WHERE f.brand_id = da.brand_id
		            AND m.car_type = da.car_type
		            AND make_date(f.period_year, f.period_month, 1) >= da.valid_from
		            AND (da.valid_to IS NULL
		                 OR make_date(f.period_year, f.period_month, 1) < da.valid_to)
		       ),0)::bigint
		  FROM distributor_assignments da
		  JOIN distributors d ON d.id = da.distributor_id
		 WHERE da.brand_id = $1
		 ORDER BY da.car_type, da.valid_from DESC`, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Assignment{}
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.ID, &a.CarType, &a.DistributorID, &a.DistributorName,
			&a.ValidFrom, &a.ValidTo, &a.Units); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FirstFactMonth returns the first month in which the brand has any facts — the
// natural default valid_from, so a first assignment covers existing history
// instead of only the future. Falls back to today when the brand has no facts.
func FirstFactMonth(ctx context.Context, pool *pgxpool.Pool, brandID int64) time.Time {
	var t *time.Time
	_ = pool.QueryRow(ctx, `
		SELECT MIN(make_date(period_year, period_month, 1))
		  FROM facts WHERE brand_id=$1`, brandID).Scan(&t)
	if t == nil {
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return *t
}

// SetAssignment points (brand, car_type) at a distributor from validFrom on.
//
// distributor_assignments carries an EXCLUDE USING gist that forbids overlapping
// date ranges for the same (brand_id, car_type), so a plain INSERT would fail
// almost every time. Instead:
//
//  1. an existing row with the same valid_from is UPDATEd in place (a correction);
//  2. otherwise the currently-open range is closed at validFrom first, then the
//     new row is inserted — "the distributor changed hands on this date", which
//     is what the effective dating is for and never trips the constraint.
func SetAssignment(ctx context.Context, pool *pgxpool.Pool, brandID int64, carType string,
	distributorID int64, validFrom time.Time, validTo *time.Time, actorID int64) error {
	carType = strings.TrimSpace(carType)
	if carType == "" {
		return fmt.Errorf("car type required — a distributor is assigned per (brand, car type)")
	}
	if distributorID == 0 {
		return fmt.Errorf("distributor required")
	}
	if validTo != nil && !validTo.After(validFrom) {
		return fmt.Errorf("valid_to must be after valid_from")
	}
	validFrom = validFrom.UTC().Truncate(24 * time.Hour)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM distributor_assignments
		 WHERE brand_id=$1 AND car_type=$2 AND valid_from=$3`, brandID, carType, validFrom).Scan(&id)
	switch {
	case err == nil:
		if _, err := tx.Exec(ctx, `
			UPDATE distributor_assignments SET distributor_id=$2, valid_to=$3
			 WHERE id=$1`, id, distributorID, validTo); err != nil {
			return assignErr(err)
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	default:
		// Close whatever range is currently open-ended and started earlier.
		if _, err := tx.Exec(ctx, `
			UPDATE distributor_assignments SET valid_to=$3
			 WHERE brand_id=$1 AND car_type=$2 AND valid_to IS NULL AND valid_from < $3`,
			brandID, carType, validFrom); err != nil {
			return assignErr(err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO distributor_assignments (brand_id, car_type, distributor_id, valid_from, valid_to)
			VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			brandID, carType, distributorID, validFrom, validTo).Scan(&id); err != nil {
			return assignErr(err)
		}
	}

	units := assignedUnitsTx(ctx, tx, brandID, carType, validFrom, validTo)
	if err := logChangeTx(ctx, tx, "distributor_assignment", id, "set", actorID, int(units)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteAssignment removes an assignment. The brand's facts in that car type then
// fall back to 'No distributor' — a valid permanent state (§2.4).
func DeleteAssignment(ctx context.Context, pool *pgxpool.Pool, id int64, actorID int64) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var brandID int64
	var carType string
	var validFrom time.Time
	var validTo *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT brand_id, car_type, valid_from, valid_to
		  FROM distributor_assignments WHERE id=$1`, id).
		Scan(&brandID, &carType, &validFrom, &validTo); err != nil {
		return 0, fmt.Errorf("assignment %d: %w", id, err)
	}
	units := assignedUnitsTx(ctx, tx, brandID, carType, validFrom, validTo)
	if _, err := tx.Exec(ctx, `DELETE FROM distributor_assignments WHERE id=$1`, id); err != nil {
		return 0, err
	}
	if err := logChangeTx(ctx, tx, "distributor_assignment", id, "delete", actorID, int(units)); err != nil {
		return 0, err
	}
	return units, tx.Commit(ctx)
}

// assignedUnitsTx is the volume a (brand, car_type, range) assignment governs —
// the impact figure for change_log.
func assignedUnitsTx(ctx context.Context, tx pgx.Tx, brandID int64, carType string, from time.Time, to *time.Time) int64 {
	var units int64
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(sum(f.volume),0)::bigint FROM facts f
		  JOIN models m ON m.id = f.model_id
		 WHERE f.brand_id=$1 AND m.car_type=$2
		   AND make_date(f.period_year, f.period_month, 1) >= $3
		   AND ($4::date IS NULL OR make_date(f.period_year, f.period_month, 1) < $4)`,
		brandID, carType, from, to).Scan(&units)
	return units
}

// assignErr turns the GIST exclusion violation into something a user can act on.
func assignErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return fmt.Errorf("that date range overlaps an existing assignment for this brand and car type — " +
			"end the existing one first, or reuse its start date to correct it")
	}
	return err
}

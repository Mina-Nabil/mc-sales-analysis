package analytics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RefreshModelDefaults recomputes each model's default engine_type and supply
// from its highest-volume historical fact value. These defaults are the
// fallback the engine/supply dimensions COALESCE onto for facts whose source
// (the live monthly feed) carried no engine/supply column. Idempotent; run it
// after any fact load. col names below are fixed literals, not user input.
func RefreshModelDefaults(ctx context.Context, pool *pgxpool.Pool) error {
	for _, col := range []string{"engine_type", "supply"} {
		q := fmt.Sprintf(`
			WITH ranked AS (
				SELECT model_id, %[1]s AS val,
				       row_number() OVER (PARTITION BY model_id ORDER BY sum(volume) DESC) AS rn
				  FROM facts
				 WHERE model_id IS NOT NULL AND %[1]s IS NOT NULL
				 GROUP BY model_id, %[1]s
			)
			UPDATE models m SET %[1]s = r.val
			  FROM ranked r
			 WHERE r.model_id = m.id AND r.rn = 1 AND m.%[1]s IS DISTINCT FROM r.val`, col)
		if _, err := pool.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

package api

import (
	"net/http"
	"time"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/analytics"
)

// Period maintenance: list the loaded periods and delete a period's facts.
// Deleting only ever touches `facts` (and marks the period's batches
// rolled_back, mirroring the supersede path in ingest.Commit §4.4). The car
// tree — brands, models, aliases, segments — is never modified.

type periodDTO struct {
	Year      int        `json:"year"`
	Month     int        `json:"month"`
	Rows      int64      `json:"rows"`
	Volume    int64      `json:"volume"`
	BatchID   *int64     `json:"batch_id"`
	BatchName *string    `json:"batch_filename"`
	Committed *time.Time `json:"committed_at"`
}

func (s *Server) listPeriods(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT f.period_year, f.period_month, count(*)::bigint, COALESCE(SUM(f.volume),0)::bigint,
		       b.id, b.source_filename, b.committed_at
		  FROM facts f
		  LEFT JOIN LATERAL (
		    SELECT id, source_filename, committed_at FROM import_batches
		     WHERE period_year = f.period_year AND period_month = f.period_month
		       AND state = 'committed'
		     ORDER BY id DESC LIMIT 1
		  ) b ON true
		 GROUP BY f.period_year, f.period_month, b.id, b.source_filename, b.committed_at
		 ORDER BY f.period_year DESC, f.period_month DESC`)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []periodDTO{}
	for rows.Next() {
		var p periodDTO
		if err := rows.Scan(&p.Year, &p.Month, &p.Rows, &p.Volume, &p.BatchID, &p.BatchName, &p.Committed); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, out)
}

// deletePeriods removes every fact in the given periods. Irreversible; the only
// way back is to re-import the month.
func (s *Server) deletePeriods(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Periods []struct {
			Year  int `json:"year"`
			Month int `json:"month"`
		} `json:"periods"`
		Confirm string `json:"confirm"`
	}
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Periods) == 0 {
		httpErr(w, http.StatusBadRequest, "select at least one period")
		return
	}
	if req.Confirm != "DELETE" {
		httpErr(w, http.StatusBadRequest, `confirmation required: send confirm:"DELETE"`)
		return
	}
	for _, p := range req.Periods {
		if p.Year < 2000 || p.Year > 2100 || p.Month < 1 || p.Month > 12 {
			httpErr(w, http.StatusBadRequest, "invalid period")
			return
		}
	}

	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	var totalRows, totalVol int64
	for _, p := range req.Periods {
		var rows, vol int64
		if err := tx.QueryRow(ctx,
			`SELECT count(*)::bigint, COALESCE(SUM(volume),0)::bigint
			   FROM facts WHERE period_year=$1 AND period_month=$2`, p.Year, p.Month).
			Scan(&rows, &vol); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM facts WHERE period_year=$1 AND period_month=$2`, p.Year, p.Month); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		// The period is no longer loaded, so its batches are no longer committed.
		if _, err := tx.Exec(ctx,
			`UPDATE import_batches SET state='rolled_back'
			  WHERE period_year=$1 AND period_month=$2 AND state='committed'`, p.Year, p.Month); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO change_log (entity_type, action, actor_id, actor_kind, volume_impact, after)
			VALUES ('period','delete_facts',$1,'human',$2,
			        jsonb_build_object('period', to_char(make_date($3,$4,1),'YYYY-MM'), 'rows', $5::bigint))`,
			s.user(r).ID, vol, p.Year, p.Month, rows); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		totalRows += rows
		totalVol += vol
	}
	if err := tx.Commit(ctx); err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Model engine/supply defaults are derived from facts — keep them current.
	_ = analytics.RefreshModelDefaults(ctx, s.pool)

	writeJSON(w, http.StatusOK, map[string]any{
		"periods":        len(req.Periods),
		"deleted_rows":   totalRows,
		"deleted_volume": totalVol,
	})
}

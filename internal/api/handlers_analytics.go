package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/analytics"
	"github.com/xuri/excelize/v2"
)

func parseMatrixParams(r *http.Request) analytics.Params {
	q := r.URL.Query()
	p := analytics.Params{
		Dimension:    or(q.Get("dimension"), "brand"),
		Year:         atoiOr(q.Get("year"), 2026),
		Month:        atoiOr(q.Get("month"), 0),
		CompareYear:  atoiOr(q.Get("compare_year"), 0),
		CompareMonth: atoiOr(q.Get("compare_month"), 0),
		Filters:      map[string][]string{},
		Limit:        atoiOr(q.Get("limit"), 100),
	}
	if p.CompareYear == 0 {
		p.CompareYear = p.Year - 1
	}
	for key, vals := range q {
		if strings.HasPrefix(key, "filter.") && len(vals) > 0 && vals[0] != "" {
			p.Filters[strings.TrimPrefix(key, "filter.")] = splitCSV(vals[0])
		}
	}
	return p
}

func (s *Server) analyticsMatrix(w http.ResponseWriter, r *http.Request) {
	res, err := analytics.Matrix(r.Context(), s.pool, parseMatrixParams(r))
	if err != nil {
		httpErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) analyticsDimensions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, analytics.AvailableDimensions())
}

var monthNames = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func (s *Server) analyticsExport(w http.ResponseWriter, r *http.Request) {
	p := parseMatrixParams(r)
	p.Limit = 0 // export everything
	res, err := analytics.Matrix(r.Context(), s.pool, p)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err.Error())
		return
	}
	f := excelize.NewFile()
	sheet := "Matrix"
	f.SetSheetName("Sheet1", sheet)

	header := []any{cases(res.Dimension)}
	header = append(header, toAny(monthNames)...)
	header = append(header, "Total", "Share %", "Compare total", "Growth %", "Δ Share pts", "Rank", "Rank prior")
	setRow(f, sheet, 1, header)

	for i, row := range res.Rows {
		rec := []any{row.Key}
		for _, m := range row.Months {
			rec = append(rec, m)
		}
		growth := "new"
		if row.GrowthPct != nil {
			growth = fmt.Sprintf("%.1f%%", *row.GrowthPct)
		}
		rec = append(rec, row.Total,
			fmt.Sprintf("%.1f%%", row.SharePct),
			row.YTDPrior, growth,
			fmt.Sprintf("%+.1f", row.SharePointDelta),
			row.Rank, row.RankPrior)
		setRow(f, sheet, i+2, rec)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="matrix-%s-%d.xlsx"`, res.Dimension, res.Year))
	if err := f.Write(w); err != nil {
		httpErr(w, http.StatusInternalServerError, "export failed")
	}
}

// ── small helpers ───────────────────────────────────────────────────────────

func setRow(f *excelize.File, sheet string, rowNum int, vals []any) {
	for c, v := range vals {
		cell, _ := excelize.CoordinatesToCellName(c+1, rowNum)
		f.SetCellValue(sheet, cell, v)
	}
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func or(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func atoiOr(v string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return def
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func cases(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ReplaceAll(s[1:], "_", " ")
}

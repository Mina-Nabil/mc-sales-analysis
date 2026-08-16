package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/auth"
	"github.com/Mina-Nabil/mc-sales-analysis/internal/ingest"
	"github.com/Mina-Nabil/mc-sales-analysis/internal/review"
)

// ── auth ────────────────────────────────────────────────────────────────────

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email, Password string }
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	token, u, err := auth.Login(r.Context(), s.pool, req.Email, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		httpErr(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "login failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: token, Path: "/",
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(30 * 24 * time.Hour),
	})
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		_ = auth.Logout(r.Context(), s.pool, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.user(r))
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email, Password string }
	if err := readJSON(r, &req); err != nil || req.Email == "" || len(req.Password) < 8 {
		httpErr(w, http.StatusBadRequest, "email and password (≥8 chars) required")
		return
	}
	u, err := auth.CreateUser(r.Context(), s.pool, req.Email, req.Password)
	if err != nil {
		httpErr(w, http.StatusConflict, "could not create user (email may already exist)")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// ── review queue ────────────────────────────────────────────────────────────

func (s *Server) reviewList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseIntQ(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := review.List(r.Context(), s.pool, limit)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []review.Item{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) reviewConfirm(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		ModelID *int64 `json:"model_id"`
	}
	_ = readJSON(r, &req) // body optional
	moved, err := review.Confirm(r.Context(), s.pool, id, req.ModelID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"facts_rederived": moved})
}

func (s *Server) reviewReassign(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		ModelID int64 `json:"model_id"`
	}
	if err := readJSON(r, &req); err != nil || req.ModelID == 0 {
		httpErr(w, http.StatusBadRequest, "model_id required")
		return
	}
	moved, err := review.Confirm(r.Context(), s.pool, id, &req.ModelID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"facts_rederived": moved})
}

func (s *Server) reviewReject(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := review.Reject(r.Context(), s.pool, id); err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

func (s *Server) reviewBulkConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MinConfidence float64 `json:"min_confidence"`
	}
	if err := readJSON(r, &req); err != nil || req.MinConfidence <= 0 {
		httpErr(w, http.StatusBadRequest, "min_confidence (0..1) required")
		return
	}
	n, vol, err := review.BulkConfirm(r.Context(), s.pool, req.MinConfidence)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"confirmed": n, "facts_rederived": vol})
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	thr := s.floatSetting(r, "fuzzy_threshold", 0.92)
	floor := s.floatSetting(r, "fuzzy_review_floor", 0.75)
	res, err := review.Resolve(r.Context(), s.pool, thr, floor)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ── car tree (read) ─────────────────────────────────────────────────────────

func (s *Server) brands(w http.ResponseWriter, r *http.Request) {
	yearWhere, args := yearFilter(r, "period_year")
	rows, err := s.pool.Query(r.Context(), `
		SELECT b.id, b.name, COALESCE(b.origin,''), b.parent_brand_id, COALESCE(v.vol,0)
		  FROM brands b
		  LEFT JOIN (SELECT brand_id, sum(volume) vol FROM facts `+yearWhere+` GROUP BY brand_id) v
		    ON v.brand_id = b.id
		 ORDER BY COALESCE(v.vol,0) DESC`, args...)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type brand struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Origin   string `json:"origin"`
		ParentID *int64 `json:"parent_id"`
		Volume   int64  `json:"volume"`
	}
	out := []brand{}
	for rows.Next() {
		var b brand
		if err := rows.Scan(&b.ID, &b.Name, &b.Origin, &b.ParentID, &b.Volume); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, b)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) brandModels(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT m.id, m.name, COALESCE(m.car_type,''), COALESCE(seg.name,''),
		       COALESCE(m.tier,''), m.status::text, COALESCE(v.vol,0)
		  FROM models m
		  LEFT JOIN segments seg ON seg.id = m.segment_id
		  LEFT JOIN (SELECT model_id, sum(volume) vol FROM facts GROUP BY model_id) v
		    ON v.model_id = m.id
		 WHERE m.brand_id = $1
		 ORDER BY COALESCE(v.vol,0) DESC`, id)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type model struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		CarType string `json:"car_type"`
		Segment string `json:"segment"`
		Tier    string `json:"tier"`
		Status  string `json:"status"`
		Volume  int64  `json:"volume"`
	}
	out := []model{}
	for rows.Next() {
		var m model
		if err := rows.Scan(&m.ID, &m.Name, &m.CarType, &m.Segment, &m.Tier, &m.Status, &m.Volume); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) modelAliases(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, raw, status::text, method, COALESCE(confidence,0), COALESCE(reasoning,'')
		  FROM model_aliases WHERE model_id = $1 ORDER BY raw`, id)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type alias struct {
		ID         int64   `json:"id"`
		Raw        string  `json:"raw"`
		Status     string  `json:"status"`
		Method     string  `json:"method"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}
	out := []alias{}
	for rows.Next() {
		var a alias
		if err := rows.Scan(&a.ID, &a.Raw, &a.Status, &a.Method, &a.Confidence, &a.Reasoning); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) segments(w http.ResponseWriter, r *http.Request) {
	yearWhere, args := yearFilter(r, "f.period_year")
	s.simpleListArgs(w, r, `
		SELECT seg.id, seg.name, COALESCE(v.vol,0)
		  FROM segments seg
		  LEFT JOIN (SELECT segment_id, sum(volume) vol FROM facts f
		             JOIN models m ON m.id=f.model_id `+yearWhere+` GROUP BY segment_id) v
		    ON v.segment_id = seg.id
		 ORDER BY COALESCE(v.vol,0) DESC`, args...)
}

func (s *Server) distributors(w http.ResponseWriter, r *http.Request) {
	s.simpleList(w, r, `SELECT id, name, 0 FROM distributors ORDER BY name`)
}

func (s *Server) simpleList(w http.ResponseWriter, r *http.Request, q string) {
	s.simpleListArgs(w, r, q)
}

func (s *Server) simpleListArgs(w http.ResponseWriter, r *http.Request, q string, args ...any) {
	rows, err := s.pool.Query(r.Context(), q, args...)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type row struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Volume int64  `json:"volume"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Name, &x.Volume); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, x)
	}
	writeJSON(w, http.StatusOK, out)
}

// ── imports ─────────────────────────────────────────────────────────────────

func (s *Server) importUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(80 << 20); err != nil {
		httpErr(w, http.StatusBadRequest, "expected multipart form with a 'file' field")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "missing 'file'")
		return
	}
	defer f.Close()

	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		httpErr(w, http.StatusInternalServerError, "cannot store upload")
		return
	}
	token := newUploadToken()
	dst := filepath.Join(s.uploadDir, token+".xlsx")
	out, err := os.Create(dst)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "cannot store upload")
		return
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		httpErr(w, http.StatusInternalServerError, "upload write failed")
		return
	}
	out.Close()

	pf, err := ingest.DetectAndParse(dst)
	if err != nil {
		_ = os.Remove(dst)
		httpErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"upload_id":    token,
		"filename":     hdr.Filename,
		"signature":    pf.Signature,
		"role":         pf.Role,
		"period_year":  pf.Year,
		"period_month": pf.Month,
		"rows":         len(pf.Rows),
		"volume":       pf.TotalVolume,
	})
}

func (s *Server) importDryRun(w http.ResponseWriter, r *http.Request) {
	pf, ok := s.loadUpload(w, r)
	if !ok {
		return
	}
	rep, err := ingest.DryRun(r.Context(), s.pool, pf)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dryRunDTO(rep))
}

func (s *Server) importCommit(w http.ResponseWriter, r *http.Request) {
	pf, ok := s.loadUpload(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = readJSON(r, &req)
	res, err := ingest.Commit(r.Context(), s.pool, pf, req.Reason)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) importList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, COALESCE(source_filename,''), COALESCE(feed_role,''),
		       period_year, period_month, state,
		       COALESCE(row_count,0), COALESCE(total_volume,0),
		       COALESCE(dropped_row_count,0), COALESCE(dropped_volume,0), created_at
		  FROM import_batches ORDER BY id DESC LIMIT 200`)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type batch struct {
		ID          int64     `json:"id"`
		Filename    string    `json:"filename"`
		Role        string    `json:"role"`
		Year        int       `json:"period_year"`
		Month       int       `json:"period_month"`
		State       string    `json:"state"`
		Rows        int       `json:"rows"`
		Volume      int64     `json:"volume"`
		DroppedRows int       `json:"dropped_rows"`
		DroppedVol  int64     `json:"dropped_volume"`
		CreatedAt   time.Time `json:"created_at"`
	}
	out := []batch{}
	for rows.Next() {
		var b batch
		if err := rows.Scan(&b.ID, &b.Filename, &b.Role, &b.Year, &b.Month, &b.State,
			&b.Rows, &b.Volume, &b.DroppedRows, &b.DroppedVol, &b.CreatedAt); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, b)
	}
	writeJSON(w, http.StatusOK, out)
}

// loadUpload re-parses a previously uploaded file by its token.
func (s *Server) loadUpload(w http.ResponseWriter, r *http.Request) (*ingest.ParsedFeed, bool) {
	token := r.PathValue("id")
	if !safeToken(token) {
		httpErr(w, http.StatusBadRequest, "bad upload id")
		return nil, false
	}
	path := filepath.Join(s.uploadDir, token+".xlsx")
	if _, err := os.Stat(path); err != nil {
		httpErr(w, http.StatusNotFound, "upload not found (re-upload the file)")
		return nil, false
	}
	pf, err := ingest.DetectAndParse(path)
	if err != nil {
		httpErr(w, http.StatusUnprocessableEntity, err.Error())
		return nil, false
	}
	return pf, true
}

// dryRunDTO shapes a DryRunReport for JSON, omitting the huge per-row slice.
func dryRunDTO(rep *ingest.DryRunReport) map[string]any {
	type ni struct {
		Raw    string `json:"raw"`
		Brand  string `json:"brand,omitempty"`
		Volume int    `json:"volume"`
	}
	conv := func(in []ingest.NewItem) []ni {
		out := make([]ni, 0, len(in))
		for _, x := range in {
			out = append(out, ni{Raw: x.Raw, Brand: x.Brand, Volume: x.Volume})
		}
		return out
	}
	return map[string]any{
		"signature":            rep.Feed.Signature,
		"period_year":          rep.Feed.Year,
		"period_month":         rep.Feed.Month,
		"parsed_rows":          len(rep.Feed.Rows),
		"parsed_volume":        rep.Feed.TotalVolume,
		"exclude_motorcycles":  rep.ExcludeMotorcycles,
		"dropped_moto_rows":    rep.DroppedMotoRows,
		"dropped_moto_volume":  rep.DroppedMotoVol,
		"car_rows":             rep.CarRows,
		"car_volume":           rep.CarVolume,
		"tier_rows":            rep.TierRows,
		"tier_volume":          rep.TierVol,
		"brand_resolved":       rep.BrandResolved,
		"brand_model_resolved": rep.BrandModelResolved,
		"prev_period":          rep.PrevPeriodLabel,
		"prev_period_volume":   rep.PrevPeriodVolume,
		"swing_pct":            rep.SwingPct,
		"existing_batch":       rep.ExistingBatchForPeriod,
		"new_brands":           conv(rep.NewBrands),
		"new_models":           conv(rep.NewModels),
	}
}

// ── misc ────────────────────────────────────────────────────────────────────

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out[k] = v
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := readJSON(r, &req); err != nil || len(req) == 0 {
		httpErr(w, http.StatusBadRequest, "expected {key: value} object")
		return
	}
	u := s.user(r)
	for k, v := range req {
		if _, err := s.pool.Exec(r.Context(), `
			INSERT INTO settings (key, value, updated_at) VALUES ($1,$2,now())
			ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, k, v); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		_, _ = s.pool.Exec(r.Context(),
			`INSERT INTO change_log (entity_type, action, actor_id, actor_kind, after)
			 VALUES ('setting',$1,$2,'human',$3)`, "update", u.ID, `{"`+k+`":"`+v+`"}`)
	}
	s.getSettings(w, r)
}

func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, entity_type, COALESCE(entity_id,0), action, actor_kind,
		       COALESCE(volume_impact,0), created_at
		  FROM change_log ORDER BY id DESC LIMIT 200`)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type change struct {
		ID         int64     `json:"id"`
		EntityType string    `json:"entity_type"`
		EntityID   int64     `json:"entity_id"`
		Action     string    `json:"action"`
		ActorKind  string    `json:"actor_kind"`
		Volume     int64     `json:"volume_impact"`
		CreatedAt  time.Time `json:"created_at"`
	}
	out := []change{}
	for rows.Next() {
		var c change
		if err := rows.Scan(&c.ID, &c.EntityType, &c.EntityID, &c.Action, &c.ActorKind, &c.Volume, &c.CreatedAt); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

// confidence returns the data-composition badge (§5.5) for an optional period.
func (s *Server) confidence(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	args := []any{}
	where := ""
	if y := q.Get("year"); y != "" {
		args = append(args, y)
		where = " WHERE period_year = $1"
		if m := q.Get("month"); m != "" {
			args = append(args, m)
			where += " AND period_month = $2"
		}
	}
	rows, err := s.pool.Query(r.Context(),
		`SELECT status::text, count(*), COALESCE(sum(volume),0) FROM facts`+where+` GROUP BY status`, args...)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type comp struct {
		Status string `json:"status"`
		Rows   int64  `json:"rows"`
		Volume int64  `json:"volume"`
	}
	out := []comp{}
	for rows.Next() {
		var c comp
		if err := rows.Scan(&c.Status, &c.Rows, &c.Volume); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	var st struct {
		Facts       int64 `json:"facts"`
		Volume      int64 `json:"volume"`
		Periods     int64 `json:"periods"`
		Brands      int64 `json:"brands"`
		Models      int64 `json:"models"`
		QueueDepth  int64 `json:"queue_depth"`
		QueueVolume int64 `json:"queue_volume"`
	}
	ctx := r.Context()
	_ = s.pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(volume),0) FROM facts`).Scan(&st.Facts, &st.Volume)
	_ = s.pool.QueryRow(ctx, `SELECT count(DISTINCT (period_year,period_month)) FROM facts`).Scan(&st.Periods)
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM brands`).Scan(&st.Brands)
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM models`).Scan(&st.Models)
	_ = s.pool.QueryRow(ctx,
		`SELECT count(*), COALESCE(sum(volume),0) FROM facts WHERE status IN ('needs_review','unresolved')`).
		Scan(&st.QueueDepth, &st.QueueVolume)
	writeJSON(w, http.StatusOK, st)
}

// ── small helpers ───────────────────────────────────────────────────────────

func (s *Server) floatSetting(r *http.Request, key string, def float64) float64 {
	var v string
	if err := s.pool.QueryRow(r.Context(), `SELECT value FROM settings WHERE key=$1`, key).Scan(&v); err != nil {
		return def
	}
	f, err := parseFloatQ(v)
	if err != nil {
		return def
	}
	return f
}

func newUploadToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func safeToken(t string) bool {
	if len(t) != 32 {
		return false
	}
	for _, c := range t {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

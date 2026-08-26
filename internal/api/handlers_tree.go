package api

import (
	"net/http"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/tree"
)

func (s *Server) createBrand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Origin   string `json:"origin"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		httpErr(w, http.StatusBadRequest, "name required")
		return
	}
	id, name, err := tree.CreateBrand(r.Context(), s.pool, req.Name, req.Origin, req.ParentID, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name})
}

func (s *Server) createModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BrandID   int64  `json:"brand_id"`
		Name      string `json:"name"`
		CarType   string `json:"car_type"`
		SegmentID *int64 `json:"segment_id"`
	}
	if err := readJSON(r, &req); err != nil || req.BrandID == 0 || req.Name == "" {
		httpErr(w, http.StatusBadRequest, "brand_id and name required")
		return
	}
	id, name, err := tree.CreateModel(r.Context(), s.pool, req.BrandID, req.Name, req.CarType, req.SegmentID, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name})
}

// modelDetail returns one model's full spec set for the Model Analytics page:
// model-level specs, the brand-level origin, and the distributor resolved from
// the effective-dated (brand, car_type) assignment at the model's latest fact
// period (mirrors the LATERAL in analytics.joinBlock).
func (s *Server) modelDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var d struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Brand       string  `json:"brand"`
		BrandID     int64   `json:"brand_id"`
		Origin      *string `json:"origin"`
		CarType     *string `json:"car_type"`
		Segment     *string `json:"segment"`
		EngineType  *string `json:"engine_type"`
		Supply      *string `json:"supply"`
		Distributor *string `json:"distributor"`
		TotalVolume int64   `json:"total_volume"`
		FirstPeriod *string `json:"first_period"`
		LastPeriod  *string `json:"last_period"`
	}
	err = s.pool.QueryRow(r.Context(), `
		WITH span AS (
			SELECT COALESCE(SUM(volume),0)::bigint AS total,
			       -- Only periods with actual sales: the monthly feed emits a row
			       -- per gov/unit even when the Zero count is 0, so plain MIN/MAX
			       -- would report a model as "active" long after it stopped selling.
			       MIN(make_date(period_year, period_month, 1)) FILTER (WHERE volume > 0) AS first_p,
			       MAX(make_date(period_year, period_month, 1)) FILTER (WHERE volume > 0) AS last_p
			  FROM facts WHERE model_id = $1
		)
		SELECT m.id, m.name, b.name, b.id, b.origin,
		       m.car_type, seg.name, m.engine_type, m.supply,
		       (SELECT dd.name FROM distributor_assignments da
		          JOIN distributors dd ON dd.id = da.distributor_id
		         WHERE da.brand_id = b.id AND da.car_type = m.car_type
		           AND da.valid_from <= COALESCE((SELECT last_p FROM span), CURRENT_DATE)
		           AND (da.valid_to IS NULL
		                OR da.valid_to > COALESCE((SELECT last_p FROM span), CURRENT_DATE))
		         LIMIT 1),
		       (SELECT total FROM span),
		       to_char((SELECT first_p FROM span), 'YYYY-MM'),
		       to_char((SELECT last_p FROM span), 'YYYY-MM')
		  FROM models m
		  JOIN brands b ON b.id = m.brand_id
		  LEFT JOIN segments seg ON seg.id = m.segment_id
		 WHERE m.id = $1`, id).
		Scan(&d.ID, &d.Name, &d.Brand, &d.BrandID, &d.Origin, &d.CarType, &d.Segment,
			&d.EngineType, &d.Supply, &d.Distributor, &d.TotalVolume,
			&d.FirstPeriod, &d.LastPeriod)
	if err != nil {
		httpErr(w, http.StatusNotFound, "model not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) editModel(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Name       string `json:"name"`
		CarType    string `json:"car_type"`
		EngineType string `json:"engine_type"`
		Supply     string `json:"supply"`
		SegmentID  *int64 `json:"segment_id"`
	}
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := tree.EditModel(r.Context(), s.pool, id, req.Name, req.CarType, req.EngineType, req.Supply, req.SegmentID, s.user(r).ID); err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) mergePreview(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	into, err := parseIntQ(r.URL.Query().Get("into_id"))
	if err != nil {
		httpErr(w, http.StatusBadRequest, "into_id required")
		return
	}
	mi, err := tree.MergePreview(r.Context(), s.pool, id, int64(into))
	if err != nil {
		httpErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mi)
}

func (s *Server) mergeModels(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		IntoID int64 `json:"into_id"`
	}
	if err := readJSON(r, &req); err != nil || req.IntoID == 0 {
		httpErr(w, http.StatusBadRequest, "into_id required")
		return
	}
	mi, err := tree.MergeModels(r.Context(), s.pool, id, req.IntoID, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mi)
}

func (s *Server) deleteAlias(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	units, err := tree.DeleteAlias(r.Context(), s.pool, id, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"facts_rederived": units})
}

func (s *Server) brandQueue(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseIntQ(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := tree.BrandQueue(r.Context(), s.pool, limit)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) resolveBrand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RawBrand string `json:"raw_brand"`
		BrandID  int64  `json:"brand_id"`
		NewBrand *struct {
			Name     string `json:"name"`
			Origin   string `json:"origin"`
			ParentID *int64 `json:"parent_id"`
		} `json:"new_brand"`
	}
	if err := readJSON(r, &req); err != nil || req.RawBrand == "" {
		httpErr(w, http.StatusBadRequest, "raw_brand required")
		return
	}
	ctx := r.Context()
	brandID := req.BrandID
	if req.NewBrand != nil && req.NewBrand.Name != "" {
		id, _, err := tree.CreateBrand(ctx, s.pool, req.NewBrand.Name, req.NewBrand.Origin, req.NewBrand.ParentID, s.user(r).ID)
		if err != nil {
			httpErr(w, http.StatusConflict, err.Error())
			return
		}
		brandID = id
	}
	if brandID == 0 {
		httpErr(w, http.StatusBadRequest, "provide brand_id or new_brand")
		return
	}
	units, err := tree.ResolveBrand(ctx, s.pool, req.RawBrand, brandID, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"brand_id": brandID, "facts_rederived": units})
}

func (s *Server) reviewNewModel(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Name      string `json:"name"`
		CarType   string `json:"car_type"`
		SegmentID *int64 `json:"segment_id"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		httpErr(w, http.StatusBadRequest, "name required")
		return
	}
	modelID, units, err := tree.CreateModelForReview(r.Context(), s.pool, id, req.Name, req.CarType, req.SegmentID, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"model_id": modelID, "facts_rederived": units})
}

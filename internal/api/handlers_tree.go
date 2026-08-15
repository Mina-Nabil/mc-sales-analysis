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
		Tier      string `json:"tier"`
		SegmentID *int64 `json:"segment_id"`
	}
	if err := readJSON(r, &req); err != nil || req.BrandID == 0 || req.Name == "" {
		httpErr(w, http.StatusBadRequest, "brand_id and name required")
		return
	}
	id, name, err := tree.CreateModel(r.Context(), s.pool, req.BrandID, req.Name, req.CarType, req.SegmentID, req.Tier, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name})
}

func (s *Server) editModel(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Name      string `json:"name"`
		CarType   string `json:"car_type"`
		Tier      string `json:"tier"`
		SegmentID *int64 `json:"segment_id"`
	}
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := tree.EditModel(r.Context(), s.pool, id, req.Name, req.CarType, req.Tier, req.SegmentID, s.user(r).ID); err != nil {
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
		Tier      string `json:"tier"`
		SegmentID *int64 `json:"segment_id"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		httpErr(w, http.StatusBadRequest, "name required")
		return
	}
	modelID, units, err := tree.CreateModelForReview(r.Context(), s.pool, id, req.Name, req.CarType, req.SegmentID, req.Tier, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"model_id": modelID, "facts_rederived": units})
}

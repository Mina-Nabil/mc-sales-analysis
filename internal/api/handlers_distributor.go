package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/tree"
)

// brandDetail returns one brand's own record for the Tree page's brand form.
// The brand list endpoint omits notes, and the form needs the parent's name.
func (s *Server) brandDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var d struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Origin      *string `json:"origin"`
		Notes       *string `json:"notes"`
		Status      string  `json:"status"`
		ParentID    *int64  `json:"parent_id"`
		ParentName  *string `json:"parent_name"`
		ModelCount  int     `json:"model_count"`
		TotalVolume int64   `json:"total_volume"`
	}
	err = s.pool.QueryRow(r.Context(), `
		SELECT b.id, b.name, b.origin, b.notes, b.status::text, b.parent_brand_id, p.name,
		       (SELECT count(*) FROM models m WHERE m.brand_id = b.id),
		       COALESCE((SELECT sum(volume) FROM facts f WHERE f.brand_id = b.id),0)::bigint
		  FROM brands b
		  LEFT JOIN brands p ON p.id = b.parent_brand_id
		 WHERE b.id = $1`, id).
		Scan(&d.ID, &d.Name, &d.Origin, &d.Notes, &d.Status, &d.ParentID, &d.ParentName,
			&d.ModelCount, &d.TotalVolume)
	if err != nil {
		httpErr(w, http.StatusNotFound, "brand not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) editBrand(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Origin   string `json:"origin"`
		Notes    string `json:"notes"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := tree.EditBrand(r.Context(), s.pool, id, req.Name, req.Origin, req.Notes, req.ParentID, s.user(r).ID); err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) createDistributor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		httpErr(w, http.StatusBadRequest, "name required")
		return
	}
	id, name, err := tree.CreateDistributor(r.Context(), s.pool, req.Name, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name})
}

func (s *Server) brandAssignments(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	items, err := tree.BrandAssignments(r.Context(), s.pool, id)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// assignmentReq is the shared body shape for setting an assignment. valid_from
// defaults to the brand's first fact month so a first assignment covers existing
// history rather than only the future.
type assignmentReq struct {
	CarType       string `json:"car_type"`
	DistributorID int64  `json:"distributor_id"`
	ValidFrom     string `json:"valid_from"`
	ValidTo       string `json:"valid_to"`
}

func (s *Server) setBrandAssignment(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req assignmentReq
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.applyAssignment(r, int64(id), req); err != nil {
		status := http.StatusConflict
		if errors.Is(err, errBadInput) {
			status = http.StatusBadRequest // a malformed date is the client's fault
		}
		httpErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// errBadInput marks a request the caller got wrong, so the handler can answer 400
// instead of 409.
var errBadInput = errors.New("bad input")

// applyAssignment parses the dates and calls tree.SetAssignment. Shared with the
// brand-creation paths, which can carry an optional first assignment.
func (s *Server) applyAssignment(r *http.Request, brandID int64, req assignmentReq) error {
	ctx := r.Context()
	from := tree.FirstFactMonth(ctx, s.pool, brandID)
	if req.ValidFrom != "" {
		t, err := time.Parse("2006-01-02", req.ValidFrom)
		if err != nil {
			return fmt.Errorf("%w: valid_from must be a date like 2026-01-01", errBadInput)
		}
		from = t
	}
	var to *time.Time
	if req.ValidTo != "" {
		t, err := time.Parse("2006-01-02", req.ValidTo)
		if err != nil {
			return fmt.Errorf("%w: valid_to must be a date like 2026-01-01", errBadInput)
		}
		to = &t
	}
	return tree.SetAssignment(ctx, s.pool, brandID, req.CarType, req.DistributorID, from, to, s.user(r).ID)
}

func (s *Server) deleteBrandAssignment(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	units, err := tree.DeleteAssignment(r.Context(), s.pool, id, s.user(r).ID)
	if err != nil {
		httpErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"units": units})
}

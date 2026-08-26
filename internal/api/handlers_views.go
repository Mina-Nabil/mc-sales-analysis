package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Saved analytics views (§5) — named dashboard snapshots shared across users.
// `config` is opaque JSON owned by the frontend (chart set + filters + year).

type viewDTO struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func (s *Server) listViews(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(),
		`SELECT id, name, config, created_at, updated_at FROM saved_views ORDER BY created_at`)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []viewDTO{}
	for rows.Next() {
		var v viewDTO
		if err := rows.Scan(&v.ID, &v.Name, &v.Config, &v.CreatedAt, &v.UpdatedAt); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createView(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		httpErr(w, http.StatusBadRequest, "expected {name, config}")
		return
	}
	if len(req.Config) == 0 {
		req.Config = json.RawMessage(`{}`)
	}
	var v viewDTO
	err := s.pool.QueryRow(r.Context(),
		`INSERT INTO saved_views (name, config, created_by) VALUES ($1, $2::jsonb, $3)
		 RETURNING id, name, config, created_at, updated_at`,
		req.Name, []byte(req.Config), s.user(r).ID).
		Scan(&v.ID, &v.Name, &v.Config, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) updateView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad view id")
		return
	}
	var req struct {
		Name   *string          `json:"name"`
		Config *json.RawMessage `json:"config"`
	}
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var cfg []byte
	if req.Config != nil {
		cfg = []byte(*req.Config)
	}
	var v viewDTO
	err = s.pool.QueryRow(r.Context(),
		`UPDATE saved_views
		    SET name = COALESCE($2, name),
		        config = COALESCE($3::jsonb, config),
		        updated_at = now()
		  WHERE id = $1
		  RETURNING id, name, config, created_at, updated_at`,
		id, req.Name, cfg).
		Scan(&v.ID, &v.Name, &v.Config, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		httpErr(w, http.StatusNotFound, "view not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad view id")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM saved_views WHERE id = $1`, id); err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

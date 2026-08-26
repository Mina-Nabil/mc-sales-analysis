package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/auth"
)

// User management (§11.5). Every user is an admin — there are no roles — but a
// user can be deactivated (blocked from every request) or deleted. The seed
// admin is protected from both so the system can always be logged into.

type userDTO struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	IsActive  bool      `json:"is_active"`
	IsSeed    bool      `json:"is_seed"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(),
		`SELECT id, email, is_active, is_seed, created_at FROM users ORDER BY id`)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []userDTO{}
	for rows.Next() {
		var u userDTO
		if err := rows.Scan(&u.ID, &u.Email, &u.IsActive, &u.IsSeed, &u.CreatedAt); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, u)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email, Password string }
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || len(req.Password) < 8 {
		httpErr(w, http.StatusBadRequest, "email and a password of at least 8 characters are required")
		return
	}
	u, err := auth.CreateUser(r.Context(), s.pool, req.Email, req.Password)
	if err != nil {
		httpErr(w, http.StatusConflict, "could not create user (is the email already in use?)")
		return
	}
	s.logUserChange(r, "create", u.ID, req.Email)
	writeJSON(w, http.StatusCreated, map[string]any{"id": u.ID, "email": u.Email})
}

// updateUser changes the email and/or password. Allowed for every user,
// including the seed admin (only delete/deactivate are blocked for it).
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email != "" {
		if _, err := s.pool.Exec(r.Context(),
			`UPDATE users SET email=$2, updated_at=now() WHERE id=$1`, id, email); err != nil {
			httpErr(w, http.StatusConflict, "could not update email (is it already in use?)")
			return
		}
	}
	if req.Password != "" {
		if len(req.Password) < 8 {
			httpErr(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := s.pool.Exec(r.Context(),
			`UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, id, hash); err != nil {
			httpErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Changing a password invalidates that user's existing sessions.
		_, _ = s.pool.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	}
	s.logUserChange(r, "update", int64(id), email)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// setUserActive activates or deactivates a user. A deactivated user fails
// authentication, so every request they make is rejected immediately.
func (s *Server) setUserActive(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if err := readJSON(r, &req); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !req.Active {
		if msg, ok := s.guardProtected(r, int64(id), "deactivate"); !ok {
			httpErr(w, http.StatusForbidden, msg)
			return
		}
	}
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE users SET is_active=$2, updated_at=now() WHERE id=$1`, id, req.Active); err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !req.Active {
		// Drop their sessions so the block is immediate, not just on next login.
		_, _ = s.pool.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	}
	action := "deactivate"
	if req.Active {
		action = "activate"
	}
	s.logUserChange(r, action, int64(id), "")
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "is_active": req.Active})
}

// deleteUser removes the account. Their attribution on aliases, imports and the
// change log is set to NULL by the FK rules (migration 0006) — history is kept,
// only the actor reference is dropped.
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if msg, ok := s.guardProtected(r, int64(id), "delete"); !ok {
		httpErr(w, http.StatusForbidden, msg)
		return
	}
	s.logUserChange(r, "delete", int64(id), "") // log before the row disappears
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM users WHERE id=$1`, id); err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// guardProtected blocks destructive actions on the seed admin, and on the
// caller's own account (deactivating or deleting yourself would lock you out).
func (s *Server) guardProtected(r *http.Request, id int64, action string) (string, bool) {
	if id == s.user(r).ID {
		return "you cannot " + action + " your own account", false
	}
	var isSeed bool
	if err := s.pool.QueryRow(r.Context(), `SELECT is_seed FROM users WHERE id=$1`, id).Scan(&isSeed); err != nil {
		return "user not found", false
	}
	if isSeed {
		return "the seed admin cannot be " + action + "d", false
	}
	return "", true
}

func (s *Server) logUserChange(r *http.Request, action string, id int64, email string) {
	after := `{}`
	if email != "" {
		after = `{"email":"` + strings.ReplaceAll(email, `"`, "") + `"}`
	}
	_, _ = s.pool.Exec(r.Context(),
		`INSERT INTO change_log (entity_type, entity_id, action, actor_id, actor_kind, after)
		 VALUES ('user',$1,$2,$3,'human',$4)`, id, action, s.user(r).ID, after)
}

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	CookieName = "mc_session"
	sessionTTL = 30 * 24 * time.Hour
)

// User is the authenticated principal.
type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

var ErrInvalidCredentials = errors.New("invalid email or password")

// SeedAdmin creates (or updates) the first user from env. It refuses to run when
// any user already exists unless force is set, so it cannot become a backdoor
// (TECH §11.5).
func SeedAdmin(ctx context.Context, pool *pgxpool.Pool, email, password string, force bool) error {
	if email == "" || password == "" {
		return errors.New("ADMIN_EMAIL and ADMIN_PASSWORD must both be set")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	// Allow updating the same admin; block creating alongside other users.
	if n > 0 && !force {
		var sameExists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, email).Scan(&sameExists); err != nil {
			return err
		}
		if !(n == 1 && sameExists) {
			return errors.New("users already exist; refusing to seed admin without --force")
		}
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (email, password_hash) VALUES ($1,$2)
		ON CONFLICT (email) DO UPDATE SET password_hash=EXCLUDED.password_hash, updated_at=now()`,
		email, hash)
	return err
}

// CreateUser adds a new user (any authenticated user may do this; no public
// registration).
func CreateUser(ctx context.Context, pool *pgxpool.Pool, email, password string) (User, error) {
	var u User
	hash, err := HashPassword(password)
	if err != nil {
		return u, err
	}
	err = pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1,$2) RETURNING id, email`,
		email, hash).Scan(&u.ID, &u.Email)
	return u, err
}

// Login verifies credentials and creates a session, returning the opaque token.
func Login(ctx context.Context, pool *pgxpool.Pool, email, password string) (string, User, error) {
	var u User
	var hash string
	err := pool.QueryRow(ctx,
		`SELECT id, email, password_hash FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", u, ErrInvalidCredentials
	}
	if err != nil {
		return "", u, err
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil || !ok {
		return "", u, ErrInvalidCredentials
	}
	token, err := newToken()
	if err != nil {
		return "", u, err
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1,$2,$3)`,
		token, u.ID, time.Now().Add(sessionTTL),
	); err != nil {
		return "", u, err
	}
	return token, u, nil
}

// Logout revokes a session.
func Logout(ctx context.Context, pool *pgxpool.Pool, token string) error {
	_, err := pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, token)
	return err
}

// Authenticate resolves a session token to its (unexpired) user.
func Authenticate(ctx context.Context, pool *pgxpool.Pool, token string) (User, error) {
	var u User
	err := pool.QueryRow(ctx, `
		SELECT u.id, u.email FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.id=$1 AND s.expires_at > now()`, token,
	).Scan(&u.ID, &u.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, fmt.Errorf("no valid session")
	}
	return u, err
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

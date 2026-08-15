// Package mcsales holds embedded assets that ship inside the single binary:
// the SQL migrations and the Phase 0 seed CSVs (the seed state per TECH §11.3).
package mcsales

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS

//go:embed seed/*.csv
var Seeds embed.FS

// WebDist is the built React SPA, served by the API (TECH §1: React embedded
// via embed.FS). Currently a placeholder page until the front-end is built.
//
//go:embed web/dist
var WebDist embed.FS

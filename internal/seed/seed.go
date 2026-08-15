// Package seed loads the Phase 0 classification (the 15 committed CSVs) into an
// empty database. It is idempotent-by-truncate: it refuses to run if the tree
// tables already hold data, so it can never double-seed.
package seed

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"html"
	"io/fs"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bom = "\uFEFF"

// Counts is what a seed run produced, for acceptance checks.
type Counts struct {
	Brands, Models, Segments               int
	BrandAliases, ModelAliases, GeoAliases int
	Regions, Governorates, TrafficUnits    int
	Distributors, DistributorAssignments   int
}

// Load reads every seed CSV from seeds (rooted at "seed/") and inserts the
// classification inside a single transaction.
func Load(ctx context.Context, pool *pgxpool.Pool, seeds fs.FS) (Counts, error) {
	var c Counts

	var already int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM brands`).Scan(&already); err != nil {
		return c, fmt.Errorf("precheck brands: %w", err)
	}
	if already > 0 {
		return c, fmt.Errorf("refusing to seed: brands table already has %d rows (drop & re-migrate first)", already)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)

	// ── segments ────────────────────────────────────────────────────────────
	segRows, err := readCSV(seeds, "segments.csv")
	if err != nil {
		return c, err
	}
	segID := map[string]int64{}
	for _, r := range segRows {
		var id int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO segments (name) VALUES ($1) RETURNING id`, r["name"],
		).Scan(&id); err != nil {
			return c, fmt.Errorf("segment %q: %w", r["name"], err)
		}
		segID[r["name"]] = id
		c.Segments++
	}

	// ── regions (derived) → governorates → traffic units ─────────────────────
	govRows, err := readCSV(seeds, "governorates.csv")
	if err != nil {
		return c, err
	}
	regID := map[string]int64{}
	govID := map[string]int64{}
	for _, r := range govRows {
		region := r["region"]
		if _, ok := regID[region]; !ok {
			var id int64
			if err := tx.QueryRow(ctx,
				`INSERT INTO regions (name) VALUES ($1) RETURNING id`, region,
			).Scan(&id); err != nil {
				return c, fmt.Errorf("region %q: %w", region, err)
			}
			regID[region] = id
			c.Regions++
		}
		var id int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO governorates (name, region_id) VALUES ($1,$2) RETURNING id`,
			r["name"], regID[region],
		).Scan(&id); err != nil {
			return c, fmt.Errorf("governorate %q: %w", r["name"], err)
		}
		govID[r["name"]] = id
		c.Governorates++
	}

	unitRows, err := readCSV(seeds, "traffic_units.csv")
	if err != nil {
		return c, err
	}
	for _, r := range unitRows {
		gid, ok := govID[r["governorate"]]
		if !ok {
			return c, fmt.Errorf("traffic unit %q references unknown governorate %q", r["name"], r["governorate"])
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO traffic_units (name, governorate_id) VALUES ($1,$2)`, r["name"], gid,
		); err != nil {
			return c, fmt.Errorf("traffic unit %q: %w", r["name"], err)
		}
		c.TrafficUnits++
	}

	// ── brands (two passes: insert, then link parents) ───────────────────────
	brandRows, err := readCSV(seeds, "brands.csv")
	if err != nil {
		return c, err
	}
	brandID := map[string]int64{}
	for _, r := range brandRows {
		var id int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO brands (name, origin, status) VALUES ($1,$2,$3) RETURNING id`,
			r["name"], nullIf(r["origin"], ""), status(r["status"]),
		).Scan(&id); err != nil {
			return c, fmt.Errorf("brand %q: %w", r["name"], err)
		}
		brandID[r["name"]] = id
		c.Brands++
	}
	for _, r := range brandRows {
		parent := r["parent_brand"]
		if parent == "" {
			continue
		}
		pid, ok := brandID[parent]
		if !ok {
			return c, fmt.Errorf("brand %q references unknown parent %q", r["name"], parent)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE brands SET parent_brand_id=$1 WHERE id=$2`, pid, brandID[r["name"]],
		); err != nil {
			return c, err
		}
	}

	// ── models ───────────────────────────────────────────────────────────────
	modelRows, err := readCSV(seeds, "models.csv")
	if err != nil {
		return c, err
	}
	modelID := map[string]int64{}
	for _, r := range modelRows {
		bid, ok := brandID[r["brand"]]
		if !ok {
			return c, fmt.Errorf("model %q references unknown brand %q", r["model"], r["brand"])
		}
		var segIDVal any
		if s := r["segment"]; s != "" {
			id, ok := segID[s]
			if !ok {
				return c, fmt.Errorf("model %q/%q references unknown segment %q", r["brand"], r["model"], s)
			}
			segIDVal = id
		}
		var id int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO models (brand_id, name, car_type, segment_id, tier, status)
			 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			bid, r["model"], nullIf(r["car_type"], ""), segIDVal, nullIf(r["tier"], ""), status(r["status"]),
		).Scan(&id); err != nil {
			return c, fmt.Errorf("model %q/%q: %w", r["brand"], r["model"], err)
		}
		modelID[key(r["brand"], r["model"])] = id
		c.Models++
	}

	// ── brand aliases (one row per raw spelling) ─────────────────────────────
	baRows, err := readCSV(seeds, "brand_aliases.csv")
	if err != nil {
		return c, err
	}
	for _, r := range baRows {
		bid, ok := brandID[r["brand"]]
		if !ok {
			return c, fmt.Errorf("brand alias references unknown brand %q", r["brand"])
		}
		for _, raw := range spellings(r["raw_spellings"], r["norm_key"]) {
			ct, err := tx.Exec(ctx,
				`INSERT INTO brand_aliases (raw, raw_normalized, raw_normalized_nospace, brand_id, status, method)
				 VALUES ($1,$2,$3,$4,$5,'seed') ON CONFLICT (raw) DO NOTHING`,
				raw, r["norm_key"], nospace(r["norm_key"]), bid, status(r["status"]),
			)
			if err != nil {
				return c, fmt.Errorf("brand alias %q: %w", raw, err)
			}
			c.BrandAliases += int(ct.RowsAffected())
		}
	}

	// ── model aliases (per-brand scope) ──────────────────────────────────────
	maRows, err := readCSV(seeds, "model_aliases.csv")
	if err != nil {
		return c, err
	}
	for _, r := range maRows {
		bid, ok := brandID[r["brand"]]
		if !ok {
			return c, fmt.Errorf("model alias references unknown brand %q", r["brand"])
		}
		mid, ok := modelID[key(r["brand"], r["model"])]
		if !ok {
			return c, fmt.Errorf("model alias references unknown model %q/%q", r["brand"], r["model"])
		}
		for _, raw := range spellings(r["raw_spellings"], r["norm_key"]) {
			ct, err := tx.Exec(ctx,
				`INSERT INTO model_aliases (brand_id, raw, raw_normalized, raw_normalized_nospace, model_id, status, method)
				 VALUES ($1,$2,$3,$4,$5,$6,'seed') ON CONFLICT (brand_id, raw) DO NOTHING`,
				bid, raw, r["norm_key"], nospace(r["norm_key"]), mid, status(r["status"]),
			)
			if err != nil {
				return c, fmt.Errorf("model alias %q/%q: %w", r["brand"], raw, err)
			}
			c.ModelAliases += int(ct.RowsAffected())
		}
	}

	// ── geo aliases ──────────────────────────────────────────────────────────
	// Resolve traffic-unit ids from inside the tx (read-your-writes).
	unitID := map[string]int64{}
	if err := loadUnitIDs(ctx, tx, unitID); err != nil {
		return c, err
	}
	gaRows, err := readCSV(seeds, "geo_aliases.csv")
	if err != nil {
		return c, err
	}
	for _, r := range gaRows {
		var targetID any
		switch r["kind"] {
		case "governorate":
			if id, ok := govID[r["target"]]; ok {
				targetID = id
			}
		case "traffic_unit":
			if id, ok := unitID[r["target"]]; ok {
				targetID = id
			}
		}
		ct, err := tx.Exec(ctx,
			`INSERT INTO geo_aliases (raw, raw_normalized, raw_normalized_nospace, kind, target_id, status, method)
			 VALUES ($1,$2,$3,$4,$5,$6,'seed') ON CONFLICT (kind, raw) DO NOTHING`,
			r["norm_key"], r["norm_key"], nospace(r["norm_key"]), r["kind"], targetID, status(r["status"]),
		)
		if err != nil {
			return c, fmt.Errorf("geo alias %q: %w", r["norm_key"], err)
		}
		c.GeoAliases += int(ct.RowsAffected())
	}

	// ── distributors (derived) + assignments ─────────────────────────────────
	// A few assignment rows still carry a pre-merge / shorthand brand name
	// (SsangYong was merged into KGM; "LI" is shorthand for "Li Auto"). Resolve
	// them through a canonicalization map — data-driven from research_decisions
	// where possible — rather than editing the authoritative seed files.
	rename, err := brandRenames(seeds)
	if err != nil {
		return c, err
	}
	canon := func(name string) string {
		if _, ok := brandID[name]; ok {
			return name
		}
		if to, ok := rename[name]; ok {
			return to
		}
		return name
	}

	daRows, err := readCSV(seeds, "distributor_assignments.csv")
	if err != nil {
		return c, err
	}
	distID := map[string]int64{}
	for _, r := range daRows {
		name := r["distributor"]
		if _, ok := distID[name]; !ok {
			var id int64
			if err := tx.QueryRow(ctx,
				`INSERT INTO distributors (name) VALUES ($1) RETURNING id`, name,
			).Scan(&id); err != nil {
				return c, fmt.Errorf("distributor %q: %w", name, err)
			}
			distID[name] = id
			c.Distributors++
		}
	}
	// After merges, two source brands can collapse onto one (brand, car_type).
	// The seed dates are all identical open ranges, so guard against the EXCLUDE
	// constraint: duplicate distributor → skip; different distributor → warn.
	seenAssign := map[string]string{} // "brandID\x00carType" -> distributor name
	for _, r := range daRows {
		cn := canon(r["brand"])
		if cn != r["brand"] {
			fmt.Printf("  note: assignment brand %q → %q\n", r["brand"], cn)
		}
		bid, ok := brandID[cn]
		if !ok {
			return c, fmt.Errorf("assignment references unknown brand %q", r["brand"])
		}
		ak := fmt.Sprintf("%d\x00%s", bid, r["car_type"])
		if prev, dup := seenAssign[ak]; dup {
			if prev != r["distributor"] {
				fmt.Printf("  warn: %s/%s already assigned to %q; ignoring conflicting %q\n",
					cn, r["car_type"], prev, r["distributor"])
			}
			continue
		}
		seenAssign[ak] = r["distributor"]
		vf, err := time.Parse("2006-01-02", r["valid_from"])
		if err != nil {
			return c, fmt.Errorf("assignment %q bad valid_from %q: %w", r["brand"], r["valid_from"], err)
		}
		var vt any
		if r["valid_to"] != "" {
			t, err := time.Parse("2006-01-02", r["valid_to"])
			if err != nil {
				return c, fmt.Errorf("assignment %q bad valid_to %q: %w", r["brand"], r["valid_to"], err)
			}
			vt = t
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO distributor_assignments (brand_id, car_type, distributor_id, valid_from, valid_to)
			 VALUES ($1,$2,$3,$4,$5)`,
			bid, r["car_type"], distID[r["distributor"]], vf, vt,
		); err != nil {
			return c, fmt.Errorf("assignment %q/%q: %w", r["brand"], r["car_type"], err)
		}
		c.DistributorAssignments++
	}

	// ── motorcycle recognition keys (§0.1, §11.6) ────────────────────────────
	mkRows, err := readCSV(seeds, "motorcycle_keys.csv")
	if err != nil {
		return c, err
	}
	for _, r := range mkRows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO motorcycle_keys (kind, brand_key, model_key) VALUES ($1,$2,$3)
			 ON CONFLICT (kind, brand_key, model_key) DO NOTHING`,
			r["kind"], r["brand_key"], r["model_key"],
		); err != nil {
			return c, fmt.Errorf("motorcycle key %q/%q: %w", r["brand_key"], r["model_key"], err)
		}
	}

	// One summary audit entry for the whole seed load (§2.6: migration actions logged).
	if _, err := tx.Exec(ctx,
		`INSERT INTO change_log (entity_type, entity_id, action, after, actor_kind)
		 VALUES ('seed', NULL, 'load', $1, 'migration')`,
		fmt.Sprintf(`{"brands":%d,"models":%d,"brand_aliases":%d,"model_aliases":%d}`,
			c.Brands, c.Models, c.BrandAliases, c.ModelAliases),
	); err != nil {
		return c, err
	}

	if err := tx.Commit(ctx); err != nil {
		return c, err
	}
	return c, nil
}

func loadUnitIDs(ctx context.Context, tx pgx.Tx, out map[string]int64) error {
	rows, err := tx.Query(ctx, `SELECT name, id FROM traffic_units`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			return err
		}
		out[name] = id
	}
	return rows.Err()
}

// ── helpers ────────────────────────────────────────────────────────────────

// readCSV reads a BOM-tolerant CSV into a slice of header→value maps.
func readCSV(fsys fs.FS, name string) ([]map[string]string, error) {
	raw, err := fs.ReadFile(fsys, "seed/"+name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	raw = bytes.TrimPrefix(raw, []byte(bom))
	rd := csv.NewReader(bytes.NewReader(raw))
	records, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	hdr := records[0]
	out := make([]map[string]string, 0, len(records)-1)
	for _, rec := range records[1:] {
		m := make(map[string]string, len(hdr))
		for i, h := range hdr {
			if i < len(rec) {
				// The Phase 0 export HTML-over-escaped some values (e.g.
				// "Lynk &amp; Co"); unescape uniformly so names, aliases and
				// normalized keys all agree and resolve against raw feed strings.
				m[strings.TrimPrefix(h, bom)] = html.UnescapeString(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// spellings splits the " | "-separated raw spellings; falls back to norm_key.
func spellings(field, fallback string) []string {
	field = strings.TrimSpace(field)
	if field == "" {
		return []string{fallback}
	}
	parts := strings.Split(field, "|")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{fallback}
	}
	return out
}

func nospace(s string) string { return strings.Join(strings.Fields(s), "") }

func nullIf(s, empty string) any {
	if s == empty {
		return nil
	}
	return s
}

// status passes through valid resolution_status values; anything else → confirmed.
func status(s string) string {
	switch s {
	case "confirmed", "auto_resolved", "needs_review", "unresolved", "rejected":
		return s
	default:
		return "confirmed"
	}
}

func key(brand, model string) string { return brand + "\x00" + model }

// brandRenames builds an old-name → canonical-name map from research_decisions
// (brand merges/renames), plus a couple of Latin shorthands the source uses in
// distributor assignments but not in the brands list.
func brandRenames(seeds fs.FS) (map[string]string, error) {
	m := map[string]string{
		"LI": "Li Auto", // shorthand used only in distributor_assignments
	}
	rows, err := readCSV(seeds, "research_decisions.csv")
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		act := strings.ToLower(r["action"])
		from, to := strings.TrimSpace(r["from_name"]), strings.TrimSpace(r["to_name"])
		if from == "" || to == "" {
			continue
		}
		if strings.Contains(act, "brand") && (strings.Contains(act, "merged") || strings.Contains(act, "renamed")) {
			m[from] = to
		}
	}
	return m, nil
}

// Package ingest holds the import pipeline. factmigrate.go implements the
// one-shot historical migration (TECH §8): it reads the source workbook's
// "Raw Data" sheet, applies the §0.1 scope rules, resolves each car row against
// the seeded aliases (deterministic tiers only), writes facts as one batch per
// period, and asserts the §8.1 acceptance tests.
package ingest

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// Raw Data column indices (0-based), per BUSINESS_LOGIC_PLAN §1.1.
const (
	colRawGov   = 0
	colRawUnit  = 1
	colRawBrand = 2
	colRawModel = 3
	colCarType  = 8
	colBrandEN  = 9
	colSegment  = 11
	colEngine   = 12
	colSupply   = 13
	colVol      = 14
	colMonth    = 15
	colYear     = 17
	minCols     = 18
)

var monthNum = map[string]int{
	"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4, "May": 5, "Jun": 6,
	"Jul": 7, "Aug": 8, "Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12,
}

type fact struct {
	rawGov, rawUnit, rawBrand, rawModel string
	year, month                         int
	volume                              int
	brandID, modelID, govID, unitID     int64 // 0 = unresolved
	engine, supply                      string
	confirmed                           bool
}

type period struct{ year, month int }

// MigrateFacts runs the full historical migration and prints the §8.1 report.
func MigrateFacts(ctx context.Context, pool *pgxpool.Pool, workbookPath string) error {
	var have int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM facts`).Scan(&have); err != nil {
		return fmt.Errorf("precheck facts: %w", err)
	}
	if have > 0 {
		return fmt.Errorf("refusing to migrate: facts already has %d rows", have)
	}
	res, err := loadResolver(ctx, pool)
	if err != nil {
		return err
	}

	f, err := excelize.OpenFile(workbookPath)
	if err != nil {
		return fmt.Errorf("open workbook: %w", err)
	}
	defer f.Close()
	rows, err := f.Rows("Raw Data")
	if err != nil {
		return fmt.Errorf("open Raw Data: %w", err)
	}

	fmt.Println("parsing Raw Data + resolving…")
	start := time.Now()

	// accumulators
	var rawRows, rawVol, motoRows, motoVol int
	facts := make([]fact, 0, 400000)
	dropRows := map[period]int{}
	dropVol := map[period]int{}
	// in-memory acceptance aggregates
	var jf2026PCB, jf2026All, car2026 int
	ct2026 := map[string]int{}
	keys2026 := map[string]struct{}{}
	var brandResRows, brandModelResRows, govResRows, unitResRows int

	first := true
	for rows.Next() {
		cols, err := rows.Columns()
		if err != nil {
			return fmt.Errorf("read row: %w", err)
		}
		if first { // header
			first = false
			continue
		}
		if len(cols) < minCols {
			continue // trailing/blank row
		}
		get := func(i int) string {
			if i < len(cols) {
				return cols[i]
			}
			return ""
		}
		rawGov, rawUnit := get(colRawGov), get(colRawUnit)
		rawBrand, rawModel := get(colRawBrand), get(colRawModel)
		if rawGov == "" && rawBrand == "" && get(colVol) == "" {
			continue
		}
		vol := parseInt(get(colVol))
		yr := parseInt(get(colYear))
		mo := monthNum[strings.TrimSpace(get(colMonth))]

		rawRows++
		rawVol += vol

		carType := strings.TrimSpace(get(colCarType))
		segment := strings.TrimSpace(get(colSegment))
		engine := strings.TrimSpace(get(colEngine))
		brandEN := strings.TrimSpace(get(colBrandEN))
		if isMotorcycle(carType, segment, engine, brandEN) {
			motoRows++
			motoVol += vol
			dropRows[period{yr, mo}]++
			dropVol[period{yr, mo}] += vol
			continue
		}

		fc := fact{
			rawGov: rawGov, rawUnit: rawUnit, rawBrand: rawBrand, rawModel: rawModel,
			year: yr, month: mo, volume: vol,
			engine: canonEngine(engine), supply: canonSupply(get(colSupply)),
		}
		fc.brandID = res.brand(rawBrand)
		if fc.brandID != 0 {
			fc.modelID = res.model(fc.brandID, rawModel)
		}
		fc.govID = res.geo(res.govNorm, rawGov)
		fc.unitID = res.geo(res.unitNorm, rawUnit)
		fc.confirmed = fc.modelID != 0
		facts = append(facts, fc)

		// resolution coverage
		if fc.brandID != 0 {
			brandResRows++
			if fc.modelID != 0 {
				brandModelResRows++
			}
		}
		if fc.govID != 0 {
			govResRows++
		}
		if fc.unitID != 0 {
			unitResRows++
		}
		// acceptance aggregates
		if yr == 2026 {
			car2026++
			keys2026[fc.rawGov+"\x00"+fc.rawUnit+"\x00"+fc.rawBrand+"\x00"+fc.rawModel+"\x00"+get(colMonth)] = struct{}{}
			ctKey := carType
			if ctKey == "" {
				ctKey = "Undefined"
			}
			ct2026[ctKey] += vol
			if mo == 1 || mo == 2 {
				jf2026All += vol
				if carType == "Passenger" || carType == "Commercial" || carType == "Bus" {
					jf2026PCB += vol
				}
			}
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close rows: %w", err)
	}
	carRows := len(facts)
	var carVol int
	for i := range facts {
		carVol += facts[i].volume
	}
	fmt.Printf("parsed %d rows in %s\n", rawRows, time.Since(start).Round(time.Second))

	// ── write batches (one per period) + COPY facts ──────────────────────────
	if err := writeBatches(ctx, pool, facts, dropRows, dropVol); err != nil {
		return err
	}

	// ── §8.1 acceptance report ───────────────────────────────────────────────
	fmt.Println("\n=== §8.1 migration acceptance ===")
	pass := true
	chk := func(label string, actual, expected int) {
		ok := actual == expected
		mark := "✓"
		if !ok {
			mark, pass = "✗", false
		}
		fmt.Printf("  %s %-46s %12d  (expect %d)\n", mark, label, actual, expected)
	}
	chk("raw rows read", rawRows, 526346)
	chk("raw volume", rawVol, 2246287)
	chk("motorcycle rows dropped", motoRows, 148921)
	chk("motorcycle units dropped", motoVol, 1138473)
	chk("car fact rows", carRows, 377425)
	chk("car volume", carVol, 1107814)
	chk("reconciliation (car+moto units)", carVol+motoVol, 2246287)
	chk("2026 car rows", car2026, 17600)
	// distinct periods from batches
	var nPeriods int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM import_batches`).Scan(&nPeriods); err != nil {
		return err
	}
	chk("import batches (periods)", nPeriods, 60)
	chk("2026 natural-key distinct", len(keys2026), 17600)
	chk("Jan+Feb 2026 Passenger+Commercial+Bus", jf2026PCB, 51252)
	chk("Jan+Feb 2026 all car types", jf2026All, 53494)
	chk("2026 car_type Passenger", ct2026["Passenger"], 40701)
	chk("2026 car_type Commercial", ct2026["Commercial"], 7567)
	chk("2026 car_type Bus", ct2026["Bus"], 2984)
	chk("2026 car_type Undefined", ct2026["Undefined"], 2220)
	chk("2026 car_type Construction", ct2026["Construction"], 22)

	// DB-side confirmation
	var dbFacts, dbVol, dbKeys2026 int
	pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(volume),0) FROM facts`).Scan(&dbFacts, &dbVol)
	pool.QueryRow(ctx, `SELECT count(DISTINCT (raw_governorate,raw_unit,raw_brand,raw_model,period_month)) FROM facts WHERE period_year=2026`).Scan(&dbKeys2026)
	chk("DB facts row count", dbFacts, 377425)
	chk("DB facts total volume", dbVol, 1107814)
	chk("DB 2026 distinct natural keys", dbKeys2026, 17600)

	fmt.Println("\n=== resolution coverage (informational — not a §8.1 gate) ===")
	pct := func(n int) float64 { return 100 * float64(n) / float64(carRows) }
	fmt.Printf("  brand resolved            %7d / %d  (%.2f%%)  [Phase 0: 98.04%%]\n", brandResRows, carRows, pct(brandResRows))
	fmt.Printf("  brand+model resolved      %7d / %d  (%.2f%%)  [Phase 0: 96.80%%]\n", brandModelResRows, carRows, pct(brandModelResRows))
	fmt.Printf("  governorate resolved      %7d / %d  (%.2f%%)\n", govResRows, carRows, pct(govResRows))
	fmt.Printf("  traffic unit resolved     %7d / %d  (%.2f%%)\n", unitResRows, carRows, pct(unitResRows))

	if !pass {
		return fmt.Errorf("§8.1 acceptance FAILED — see ✗ rows above")
	}
	fmt.Println("\nAll §8.1 acceptance tests passed. ✅")
	return nil
}

func writeBatches(ctx context.Context, pool *pgxpool.Pool, facts []fact, dropRows, dropVol map[period]int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// group fact indices by period
	byPeriod := map[period][]int{}
	for i := range facts {
		p := period{facts[i].year, facts[i].month}
		byPeriod[p] = append(byPeriod[p], i)
	}
	// also include drop-only periods (none expected, but be safe)
	for p := range dropRows {
		if _, ok := byPeriod[p]; !ok {
			byPeriod[p] = nil
		}
	}
	periods := make([]period, 0, len(byPeriod))
	for p := range byPeriod {
		periods = append(periods, p)
	}
	sort.Slice(periods, func(i, j int) bool {
		if periods[i].year != periods[j].year {
			return periods[i].year < periods[j].year
		}
		return periods[i].month < periods[j].month
	})

	cols := []string{
		"raw_governorate", "raw_unit", "raw_brand", "raw_model",
		"period_year", "period_month", "volume",
		"brand_id", "model_id", "governorate_id", "traffic_unit_id",
		"import_batch_id", "status", "engine_type", "supply",
	}
	for _, p := range periods {
		idx := byPeriod[p]
		var vol int
		for _, i := range idx {
			vol += facts[i].volume
		}
		var batchID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO import_batches
			  (source_filename, feed_role, detected_grain, period_year, period_month,
			   state, row_count, total_volume, dropped_row_count, dropped_volume,
			   committed_at, created_at)
			VALUES ($1,'primary','brands_models_by_status',$2,$3,'committed',$4,$5,$6,$7,now(),now())
			RETURNING id`,
			"260308_Registration Report Dashboard Inc Distributor.xlsx",
			p.year, p.month, len(idx), vol, dropRows[p], dropVol[p],
		).Scan(&batchID); err != nil {
			return fmt.Errorf("batch %d-%02d: %w", p.year, p.month, err)
		}

		src := pgx.CopyFromSlice(len(idx), func(k int) ([]any, error) {
			fc := facts[idx[k]]
			status := "unresolved"
			if fc.confirmed {
				status = "confirmed"
			}
			return []any{
				fc.rawGov, fc.rawUnit, fc.rawBrand, fc.rawModel,
				int16(fc.year), int16(fc.month), fc.volume,
				nz(fc.brandID), nz(fc.modelID), nz(fc.govID), nz(fc.unitID),
				batchID, status, nzs(fc.engine), nzs(fc.supply),
			}, nil
		})
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"facts"}, cols, src); err != nil {
			return fmt.Errorf("copy facts %d-%02d: %w", p.year, p.month, err)
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO change_log (entity_type, action, actor_kind) VALUES ('facts','migrate','migration')`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isMotorcycle(carType, segment, engine, brand string) bool {
	return carType == "Motorcycle" || segment == "Motorcycle" || segment == "Scooter" ||
		engine == "Motorcycle" || brand == "Motorcycle"
}

func canonEngine(s string) string {
	switch {
	case strings.EqualFold(s, "ice"):
		return "ICE"
	case strings.EqualFold(s, "hybrid"):
		return "HYBRID"
	default:
		return s
	}
}

func canonSupply(s string) string { return strings.TrimSpace(s) }

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}

func nz(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
func nzs(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

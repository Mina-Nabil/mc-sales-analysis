package ingest

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// FeedRow is one parsed fact from a monthly primary feed.
type FeedRow struct {
	RawGov, RawUnit, RawBrand, RawModel string
	// ModelYear is the vehicle's year of manufacture (سنة الصنع). 0 means the
	// feed did not carry one — stored as NULL, surfaced as 'Unknown'.
	ModelYear int
	Volume    int
}

// ParsedFeed is the result of detecting + parsing an uploaded file.
type ParsedFeed struct {
	Path        string
	Signature   string
	Role        string
	Year, Month int
	Rows        []FeedRow
	TotalVolume int
	// ModelYears lists the year columns the sheet carried, in sheet order.
	// Empty for the by-status feed, which has no manufacture year at all.
	ModelYears []int
	// PrivatePlate marks the الملاكي variant of the report, which covers only
	// private plates (~a third of the market). Parsing it at all requires
	// ParseOptions.AllowPrivatePlate; the flag then travels with the feed so
	// the dry run, the UI and the batch row all say the month is partial.
	PrivatePlate bool
}

// ParseOptions carries the caller's choices for DetectAndParseWithOptions.
type ParseOptions struct {
	// Year and Month, when both non-zero, replace the period detected from the
	// sheet title.
	Year, Month int
	// AllowPrivatePlate accepts the الملاكي report instead of refusing it. It
	// is deliberately opt-in per import: the sheet is NOT the full market, so a
	// month loaded from it is not comparable with the all-vehicles months.
	AllowPrivatePlate bool
}

// PrivatePlateNote is prefixed to the batch's revision_reason when the month
// was loaded from the الملاكي report, so the partial coverage is on the record.
const PrivatePlateNote = "[PRIVATE PLATES ONLY — الملاكي report, not full market coverage]"

var periodRe = regexp.MustCompile(`من\s*(\d{4})/(\d{1,2})/(\d{1,2})`)

// DetectAndParse identifies the file's signature/role/period and parses it,
// taking the period from the sheet title (the traffic-authority `من YYYY/MM/DD`
// header). Used by the CLI import path.
// Two signatures are recognised — brands_models_by_year (preferred: it carries
// the manufacture year) and brands_models_by_status. Anything else returns a
// clear "unsupported" error rather than mis-parsing (§4.1).
func DetectAndParse(path string) (*ParsedFeed, error) {
	return DetectAndParseWithOptions(path, ParseOptions{})
}

// DetectAndParseWithPeriod is DetectAndParse with an explicit period override:
// when yearOverride/monthOverride are both non-zero they replace the period
// detected from the sheet title. This lets the web UI let the user pick the
// month/year (or derive it from the file name) rather than depending on the
// title cell. When no override is given and the title carries no detectable
// period, it errors as before.
func DetectAndParseWithPeriod(path string, yearOverride, monthOverride int) (*ParsedFeed, error) {
	return DetectAndParseWithOptions(path, ParseOptions{Year: yearOverride, Month: monthOverride})
}

// DetectAndParseWithOptions is the full form: period override plus the
// opt-in that accepts the private-plate (الملاكي) variant.
func DetectAndParseWithOptions(path string, opts ParseOptions) (*ParsedFeed, error) {
	yearOverride, monthOverride := opts.Year, opts.Month
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	sheet := f.GetSheetList()[0]
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read sheet: %w", err)
	}
	if len(rows) < 4 {
		return nil, fmt.Errorf("file has too few rows to be a known feed")
	}

	title := cell(rows, 0, 0)
	header := rows[1] // row 2: dimension labels
	sub := rows[2]    // row 3: measure sub-columns

	// Two signatures are supported. The by-model-year report is preferred — it
	// is an exact decomposition of the by-status feed's Zero column and carries
	// the manufacture year — but a month delivered only in the older shape still
	// imports, with model_year left NULL.
	privatePlate := isPrivatePlateReport(title)
	var parse func(*ParsedFeed, [][]string) error
	switch {
	case isModelYearFeed(header, sub):
		if privatePlate && !opts.AllowPrivatePlate {
			return nil, errPrivatePlate
		}
		parse = parseByModelYear
	case isPrimaryStatusFeed(header, sub):
		parse = parseByStatus
	default:
		return nil, fmt.Errorf("unrecognised header signature — refusing to guess.\n  row2=%v\n  row3=%v", header, sub)
	}

	yr, mo, err := detectPeriod(title)
	if yearOverride > 0 && monthOverride > 0 {
		yr, mo = yearOverride, monthOverride // caller-supplied period wins
	} else if err != nil {
		return nil, err
	}
	if yr < 2000 || yr > 2100 || mo < 1 || mo > 12 {
		return nil, fmt.Errorf("invalid period %04d-%02d", yr, mo)
	}

	pf := &ParsedFeed{Path: path, Role: "primary", Year: yr, Month: mo, PrivatePlate: privatePlate}
	if err := parse(pf, rows); err != nil {
		return nil, err
	}
	return pf, nil
}

// parseByStatus reads the older brands × models × vehicle-status feed. Volume is
// the Zero column (col 6); the sheet carries no manufacture year.
func parseByStatus(pf *ParsedFeed, rows [][]string) error {
	pf.Signature = "brands_models_by_status"
	// Forward-fill governorate (col 1) and unit (col 2).
	var lastGov, lastUnit string
	for i := 3; i < len(rows); i++ {
		r := rows[i]
		if gov := col(r, 1); strings.TrimSpace(gov) != "" {
			lastGov = gov
		}
		if unit := col(r, 2); strings.TrimSpace(unit) != "" {
			lastUnit = unit
		}
		brand, model := col(r, 3), col(r, 4)
		if strings.TrimSpace(brand) == "" && strings.TrimSpace(model) == "" {
			continue // subtotal / blank
		}
		vol := parseInt(col(r, 6)) // Zero column
		pf.Rows = append(pf.Rows, FeedRow{
			RawGov: lastGov, RawUnit: lastUnit, RawBrand: brand, RawModel: model, Volume: vol})
		pf.TotalVolume += vol
	}
	return nil
}

// parseByModelYear reads the "zero vehicles by year of manufacture" report and
// unpivots it: one source row becomes one fact per non-empty year cell.
//
// The year block is read from row 3 every time — the authority slides the window
// (Jan/Feb 2026 report 2022–2026, Mar onward 2023–2027), so column offsets must
// never be hard-coded. If a row's year cells do not add up to its own grand
// total, the difference is emitted as a single model-year-unknown row, so the
// period still reconciles exactly (§8.1) rather than silently losing units.
func parseByModelYear(pf *ParsedFeed, rows [][]string) error {
	pf.Signature = "brands_models_by_year"

	yearCols := map[int]int{} // column index → model year
	for i, v := range rows[2] {
		if y := parseInt(v); y >= 1980 && y <= 2100 && len(strings.TrimSpace(v)) == 4 {
			yearCols[i] = y
			pf.ModelYears = append(pf.ModelYears, y)
		}
	}
	if len(yearCols) == 0 {
		return fmt.Errorf("model-year feed has no year columns in row 3: %v", rows[2])
	}
	sort.Ints(pf.ModelYears)

	totalCol := -1
	for i, h := range rows[1] {
		if strings.Contains(h, "الإجمالي العام") {
			totalCol = i
		}
	}

	var lastGov, lastUnit string
	for i := 3; i < len(rows); i++ {
		r := rows[i]
		// the trailing grand-total row carries the label in place of a governorate
		if strings.Contains(col(r, 0), "الإجمالي العام") || strings.Contains(col(r, 1), "الإجمالي العام") {
			continue
		}
		if gov := col(r, 1); strings.TrimSpace(gov) != "" {
			lastGov = gov
		}
		if unit := col(r, 2); strings.TrimSpace(unit) != "" {
			lastUnit = unit
		}
		brand, model := col(r, 3), col(r, 4)
		if strings.TrimSpace(brand) == "" && strings.TrimSpace(model) == "" {
			continue // subtotal / blank
		}

		base := FeedRow{RawGov: lastGov, RawUnit: lastUnit, RawBrand: brand, RawModel: model}
		var spread int
		for ci, year := range yearCols {
			v := strings.TrimSpace(col(r, ci))
			if v == "" {
				continue
			}
			vol := parseInt(v)
			if vol == 0 {
				continue
			}
			row := base
			row.ModelYear, row.Volume = year, vol
			pf.Rows = append(pf.Rows, row)
			pf.TotalVolume += vol
			spread += vol
		}
		// Residual guard: never let a truncated year block drop units.
		if totalCol >= 0 {
			if grand := parseInt(col(r, totalCol)); grand > spread {
				row := base
				row.ModelYear, row.Volume = 0, grand-spread
				pf.Rows = append(pf.Rows, row)
				pf.TotalVolume += row.Volume
			}
		}
	}
	return nil
}

func isPrimaryStatusFeed(header, sub []string) bool {
	h := strings.Join(header, "|")
	need := []string{"محافظة الإصدار", "المنفذ", "الماركة", "الطراز"}
	for _, n := range need {
		if !strings.Contains(h, n) {
			return false
		}
	}
	s := strings.Join(sub, "|")
	return strings.Contains(s, "Zero") && strings.Contains(s, "Used")
}

// isModelYearFeed recognises the by-year-of-manufacture report: same first five
// columns as the by-status feed, but the measure block is سنة الصنع with 4-digit
// year sub-columns (and the model column is الموديل, not الطراز).
func isModelYearFeed(header, sub []string) bool {
	h := strings.Join(header, "|")
	for _, n := range []string{"محافظة الإصدار", "المنفذ", "الماركة", "سنة الصنع"} {
		if !strings.Contains(h, n) {
			return false
		}
	}
	for _, v := range sub {
		if y := parseInt(v); y >= 1980 && y <= 2100 && len(strings.TrimSpace(v)) == 4 {
			return true
		}
	}
	return false
}

// isPrivatePlateReport spots the look-alike that ships in the same archive:
// تقرير … المركبات الملاكي … is private plates only and has an identical header
// signature, but roughly a third of the units (Jul-2026: 22,378 vs 63,219).
// Matching on the NORMALIZED title matters — these sheets carry tatweel
// elongation throughout, so a raw substring test is unreliable.
func isPrivatePlateReport(title string) bool {
	return strings.Contains(domain.Normalize(title), domain.Normalize("المركبات الملاكي"))
}

// errPrivatePlate is what an unflagged private-plate upload gets. It stays the
// default: loading it silently would under-count the month by ~two thirds.
var errPrivatePlate = fmt.Errorf("this is the private-plate (الملاكي) report, which covers only part of the market — " +
	"upload the all-vehicles report (تقرير بماركات وطرازات المركبات المرخصة لأول مرة) instead, " +
	"or tick “private-plate report” / pass --allow-private-plate to load it as a partial-coverage month")

// rejectPrivatePlateVariant is the default-path guard kept as a named check.
func rejectPrivatePlateVariant(title string) error {
	if isPrivatePlateReport(title) {
		return errPrivatePlate
	}
	return nil
}

func detectPeriod(title string) (int, int, error) {
	m := periodRe.FindStringSubmatch(title)
	if m == nil {
		return 0, 0, fmt.Errorf("could not detect period from title %q (implement filename/prompt fallback)", title)
	}
	return atoi(m[1]), atoi(m[2]), nil
}

// ── Dry run (§4.3) ──────────────────────────────────────────────────────────

type NewItem struct {
	Raw    string
	Brand  string // for models: the resolved brand name
	Volume int
}

// DryRunReport is everything §4.3 requires the dry run to display.
type DryRunReport struct {
	Feed                              *ParsedFeed
	ExcludeMotorcycles                bool
	DroppedMotoRows, DroppedMotoVol   int
	CarRows, CarVolume                int            // after motorcycle exclusion
	TierRows                          map[string]int // exact|normalized|nospace|unresolved
	TierVol                           map[string]int
	BrandResolved, BrandModelResolved int
	NewBrands, NewModels              []NewItem
	// YearMix is car units by model year (0 = the feed carried none for that row).
	YearMix                map[int]int
	UnknownYearVol         int
	PrevPeriodLabel        string
	PrevPeriodVolume       int
	SwingPct               float64
	ExistingBatchForPeriod bool
}

func settingBool(ctx context.Context, pool *pgxpool.Pool, key string, def bool) bool {
	var v string
	if err := pool.QueryRow(ctx, `SELECT value FROM settings WHERE key=$1`, key).Scan(&v); err != nil {
		return def
	}
	return v == "true" || v == "1"
}

func DryRun(ctx context.Context, pool *pgxpool.Pool, pf *ParsedFeed) (*DryRunReport, error) {
	res, err := loadResolver(ctx, pool)
	if err != nil {
		return nil, err
	}
	rep := &DryRunReport{
		Feed:               pf,
		ExcludeMotorcycles: settingBool(ctx, pool, "ingest.exclude_motorcycles", true),
		TierRows:           map[string]int{}, TierVol: map[string]int{},
		YearMix: map[int]int{},
	}
	newBrandVol := map[string]int{}
	newModelVol := map[string]int{}
	newModelBrand := map[string]string{}
	brandName := map[int64]string{}

	for _, row := range pf.Rows {
		if rep.ExcludeMotorcycles && res.isMotorcycle(row.RawBrand, row.RawModel) {
			rep.DroppedMotoRows++
			rep.DroppedMotoVol += row.Volume
			continue
		}
		rep.CarRows++
		rep.CarVolume += row.Volume
		rep.YearMix[row.ModelYear] += row.Volume
		if row.ModelYear == 0 {
			rep.UnknownYearVol += row.Volume
		}
		bid, btier := res.brandT(row.RawBrand)
		if bid == 0 {
			rep.TierRows["unresolved"]++
			rep.TierVol["unresolved"] += row.Volume
			newBrandVol[row.RawBrand] += row.Volume
			continue
		}
		rep.BrandResolved++
		mid, mtier := res.modelT(bid, row.RawModel)
		if mid == 0 {
			rep.TierRows["unresolved"]++
			rep.TierVol["unresolved"] += row.Volume
			bn := brandName[bid]
			if bn == "" {
				bn = lookupBrandName(ctx, pool, bid)
				brandName[bid] = bn
			}
			k := bn + " / " + row.RawModel
			newModelVol[k] += row.Volume
			newModelBrand[k] = bn
			continue
		}
		rep.BrandModelResolved++
		// tier is the weaker of brand/model match
		rep.TierRows[weaker(btier, mtier)]++
		rep.TierVol[weaker(btier, mtier)] += row.Volume
	}

	rep.NewBrands = topItems(newBrandVol, nil, 15)
	rep.NewModels = topItems(newModelVol, newModelBrand, 15)

	// compare to the most recent committed period strictly before this one
	var py, pm, pv int
	err = pool.QueryRow(ctx, `
		SELECT period_year, period_month, total_volume
		  FROM import_batches
		 WHERE state='committed' AND (period_year*12+period_month) < $1
		 ORDER BY period_year*12+period_month DESC LIMIT 1`,
		pf.Year*12+pf.Month,
	).Scan(&py, &pm, &pv)
	if err == nil {
		rep.PrevPeriodLabel = fmt.Sprintf("%04d-%02d", py, pm)
		rep.PrevPeriodVolume = pv
		if pv > 0 {
			rep.SwingPct = 100 * float64(rep.CarVolume-pv) / float64(pv)
		}
	}

	// is there already a committed batch for THIS period? (revision)
	_ = pool.QueryRow(ctx, `
		SELECT true FROM import_batches
		 WHERE state='committed' AND period_year=$1 AND period_month=$2 LIMIT 1`,
		pf.Year, pf.Month,
	).Scan(&rep.ExistingBatchForPeriod)

	return rep, nil
}

// CommitResult summarises what a commit wrote.
type CommitResult struct {
	BatchID         int64 `json:"batch_id"`
	CarRows         int   `json:"car_rows"`
	CarVolume       int   `json:"car_volume"`
	DroppedMotoRows int   `json:"dropped_moto_rows"`
	DroppedMotoVol  int   `json:"dropped_moto_volume"`
	Revised         bool  `json:"revised"`
}

// Commit writes the parsed feed as a committed batch of facts. If a committed
// batch already exists for the period, it is superseded (§4.4).
func Commit(ctx context.Context, pool *pgxpool.Pool, pf *ParsedFeed, reason string) (CommitResult, error) {
	var out CommitResult
	// A partial-coverage month must say so permanently, not just in the dry run.
	if pf.PrivatePlate {
		reason = strings.TrimSpace(PrivatePlateNote + " " + reason)
	}
	res, err := loadResolver(ctx, pool)
	if err != nil {
		return out, err
	}
	excludeMotos := settingBool(ctx, pool, "ingest.exclude_motorcycles", true)

	// Partition rows into cars (stored) and motorcycles (dropped but counted).
	carRows := make([]FeedRow, 0, len(pf.Rows))
	var carVol, dropRows, dropVol int
	for _, row := range pf.Rows {
		if excludeMotos && res.isMotorcycle(row.RawBrand, row.RawModel) {
			dropRows++
			dropVol += row.Volume
			continue
		}
		carRows = append(carRows, row)
		carVol += row.Volume
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)

	var superseded *int64
	rows, _ := tx.Query(ctx,
		`SELECT id FROM import_batches WHERE state='committed' AND period_year=$1 AND period_month=$2`,
		pf.Year, pf.Month)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		superseded = &id
	}
	rows.Close()
	if superseded != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM facts WHERE import_batch_id=$1`, *superseded); err != nil {
			return out, err
		}
		if _, err := tx.Exec(ctx, `UPDATE import_batches SET state='rolled_back' WHERE id=$1`, *superseded); err != nil {
			return out, err
		}
	}

	var batchID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO import_batches
		  (source_filename, feed_role, detected_grain, period_year, period_month,
		   state, row_count, total_volume, dropped_row_count, dropped_volume,
		   supersedes_batch_id, revision_reason, committed_at, created_at)
		VALUES ($1,'primary',$2,$3,$4,'committed',$5,$6,$7,$8,$9,$10,now(),now())
		RETURNING id`,
		baseName(pf.Path), pf.Signature, pf.Year, pf.Month,
		len(carRows), carVol, dropRows, dropVol, superseded, nzStr(reason),
	).Scan(&batchID); err != nil {
		return out, err
	}

	cols := []string{
		"raw_governorate", "raw_unit", "raw_brand", "raw_model",
		"period_year", "period_month", "volume", "model_year",
		"brand_id", "model_id", "governorate_id", "traffic_unit_id",
		"import_batch_id", "status",
	}
	src := pgx.CopyFromSlice(len(carRows), func(k int) ([]any, error) {
		row := carRows[k]
		bid := res.brand(row.RawBrand)
		var mid int64
		if bid != 0 {
			mid = res.model(bid, row.RawModel)
		}
		gid := res.geo(res.govNorm, row.RawGov)
		uid := res.geo(res.unitNorm, row.RawUnit)
		status := "unresolved"
		if mid != 0 {
			status = "confirmed"
		}
		return []any{
			row.RawGov, row.RawUnit, row.RawBrand, row.RawModel,
			int16(pf.Year), int16(pf.Month), row.Volume, nzYear(row.ModelYear),
			nz(bid), nz(mid), nz(gid), nz(uid), batchID, status,
		}, nil
	})
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"facts"}, cols, src); err != nil {
		return out, err
	}

	action := "import"
	if superseded != nil {
		action = "revise"
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO change_log (entity_type, entity_id, action, actor_kind, batch_id)
		 VALUES ('import_batch',$1,$2,'human',$1)`, batchID, action); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	out = CommitResult{
		BatchID: batchID, CarRows: len(carRows), CarVolume: carVol,
		DroppedMotoRows: dropRows, DroppedMotoVol: dropVol, Revised: superseded != nil,
	}
	return out, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func lookupBrandName(ctx context.Context, pool *pgxpool.Pool, id int64) string {
	var n string
	pool.QueryRow(ctx, `SELECT name FROM brands WHERE id=$1`, id).Scan(&n)
	return n
}

// weaker returns the less-certain of two tier labels.
func weaker(a, b string) string {
	rank := map[string]int{"exact": 3, "normalized": 2, "nospace": 1}
	if rank[a] <= rank[b] {
		return a
	}
	return b
}

func topItems(vol map[string]int, brand map[string]string, n int) []NewItem {
	items := make([]NewItem, 0, len(vol))
	for k, v := range vol {
		it := NewItem{Raw: k, Volume: v}
		if brand != nil {
			it.Brand = brand[k]
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Volume > items[j].Volume })
	if len(items) > n {
		items = items[:n]
	}
	return items
}

func cell(rows [][]string, r, c int) string {
	if r < len(rows) && c < len(rows[r]) {
		return rows[r][c]
	}
	return ""
}

func col(r []string, c int) string {
	if c < len(r) {
		return r[c]
	}
	return ""
}

func atoi(s string) int {
	n := 0
	for _, ch := range strings.TrimSpace(s) {
		if ch < '0' || ch > '9' {
			return n
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// nzYear stores an unknown model year as NULL rather than 0.
func nzYear(y int) any {
	if y == 0 {
		return nil
	}
	return int16(y)
}

func nzStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

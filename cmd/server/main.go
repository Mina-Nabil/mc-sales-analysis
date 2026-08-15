// Command server is the single binary for the whole system (TECH §1).
// It dispatches on a subcommand:
//
//	server migrate      apply SQL migrations
//	server seed         load the Phase 0 classification CSVs
//	server serve        run the API + import worker  (not yet implemented)
//	server seed-admin   create/update the first admin user  (not yet implemented)
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mcsales "github.com/Mina-Nabil/mc-sales-analysis"
	"github.com/Mina-Nabil/mc-sales-analysis/internal/api"
	"github.com/Mina-Nabil/mc-sales-analysis/internal/auth"
	"github.com/Mina-Nabil/mc-sales-analysis/internal/ingest"
	"github.com/Mina-Nabil/mc-sales-analysis/internal/review"
	"github.com/Mina-Nabil/mc-sales-analysis/internal/seed"
	"github.com/Mina-Nabil/mc-sales-analysis/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := run(ctx, os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string) error {
	switch cmd {
	case "migrate":
		return cmdMigrate(ctx)
	case "seed":
		return cmdSeed(ctx)
	case "migrate-facts":
		return cmdMigrateFacts(ctx)
	case "import":
		return cmdImport(ctx, false, "")
	case "import-commit":
		reason := ""
		if len(os.Args) > 3 {
			reason = os.Args[3]
		}
		return cmdImport(ctx, true, reason)
	case "resolve":
		return cmdResolve(ctx)
	case "review":
		return cmdReview(ctx)
	case "serve":
		return cmdServe()
	case "seed-admin":
		return cmdSeedAdmin(ctx)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func cmdMigrate(ctx context.Context) error {
	pool, err := store.Connect(ctx, dbURL())
	if err != nil {
		return err
	}
	defer pool.Close()
	fmt.Println("applying migrations…")
	if err := store.Migrate(ctx, pool, mcsales.Migrations); err != nil {
		return err
	}
	fmt.Println("migrations up to date.")
	return nil
}

func cmdSeed(ctx context.Context) error {
	pool, err := store.Connect(ctx, dbURL())
	if err != nil {
		return err
	}
	defer pool.Close()
	fmt.Println("loading Phase 0 seed…")
	c, err := seed.Load(ctx, pool, mcsales.Seeds)
	if err != nil {
		return err
	}
	fmt.Printf(`seeded:
  brands                    %4d
  models                    %4d
  segments                  %4d
  brand_aliases             %4d
  model_aliases             %4d
  geo_aliases               %4d
  regions                   %4d
  governorates              %4d
  traffic_units             %4d
  distributors              %4d
  distributor_assignments   %4d
`,
		c.Brands, c.Models, c.Segments, c.BrandAliases, c.ModelAliases, c.GeoAliases,
		c.Regions, c.Governorates, c.TrafficUnits, c.Distributors, c.DistributorAssignments)
	return nil
}

func cmdMigrateFacts(ctx context.Context) error {
	wb := os.Getenv("SOURCE_WORKBOOK")
	if wb == "" {
		wb = "./260308_Registration Report Dashboard Inc Distributor.xlsx"
	}
	pool, err := store.Connect(ctx, dbURL())
	if err != nil {
		return err
	}
	defer pool.Close()
	return ingest.MigrateFacts(ctx, pool, wb)
}

func cmdImport(ctx context.Context, commit bool, reason string) error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: server import <file.xlsx>   |   server import-commit <file.xlsx> [reason]")
	}
	path := os.Args[2]
	pool, err := store.Connect(ctx, dbURL())
	if err != nil {
		return err
	}
	defer pool.Close()

	fmt.Println("detecting + parsing…")
	pf, err := ingest.DetectAndParse(path)
	if err != nil {
		return err
	}
	rep, err := ingest.DryRun(ctx, pool, pf)
	if err != nil {
		return err
	}
	printDryRun(rep)

	if !commit {
		fmt.Println("\n(dry run only — re-run with `import-commit` to write facts)")
		return nil
	}
	fmt.Println("\ncommitting…")
	r, err := ingest.Commit(ctx, pool, pf, reason)
	if err != nil {
		return err
	}
	verb := "committed"
	if r.Revised {
		verb = "revised"
	}
	fmt.Printf("%s batch #%d for %04d-%02d — %d car facts / %d units (dropped %d motorcycle rows / %d units)\n",
		verb, r.BatchID, pf.Year, pf.Month, r.CarRows, r.CarVolume, r.DroppedMotoRows, r.DroppedMotoVol)
	return nil
}

func printDryRun(r *ingest.DryRunReport) {
	pf := r.Feed
	total := r.CarRows
	fmt.Printf("\n=== dry-run report ===\n")
	fmt.Printf("  signature   %s (%s)\n", pf.Signature, pf.Role)
	fmt.Printf("  period      %04d-%02d\n", pf.Year, pf.Month)
	fmt.Printf("  parsed      %d rows / %d units\n", len(pf.Rows), pf.TotalVolume)
	if r.ExcludeMotorcycles {
		fmt.Printf("  dropped     %d motorcycle rows / %d units (excluded, still reported)\n", r.DroppedMotoRows, r.DroppedMotoVol)
	}
	fmt.Printf("  car facts   %d rows / %d units\n", r.CarRows, r.CarVolume)
	if r.ExistingBatchForPeriod {
		fmt.Printf("  ⚠ a committed batch already exists for this period — commit will REVISE it\n")
	}
	fmt.Printf("\n  resolution by tier (row / volume):\n")
	for _, t := range []string{"exact", "normalized", "nospace", "unresolved"} {
		fmt.Printf("    %-11s %7d  %10d\n", t, r.TierRows[t], r.TierVol[t])
	}
	pct := func(n int) float64 {
		if total == 0 {
			return 0
		}
		return 100 * float64(n) / float64(total)
	}
	fmt.Printf("  brand resolved        %.1f%%   brand+model resolved  %.1f%%\n",
		pct(r.BrandResolved), pct(r.BrandModelResolved))

	if r.PrevPeriodLabel != "" {
		flag := ""
		if r.SwingPct > 30 || r.SwingPct < -30 {
			flag = "  ⚠ >30% swing — check this is the right file"
		}
		fmt.Printf("\n  vs %s: %d → %d  (%+.1f%%)%s\n",
			r.PrevPeriodLabel, r.PrevPeriodVolume, pf.TotalVolume, r.SwingPct, flag)
	}
	if len(r.NewBrands) > 0 {
		fmt.Printf("\n  top unresolved brands (→ review as new brands):\n")
		for _, it := range r.NewBrands {
			fmt.Printf("    %6d  %s\n", it.Volume, it.Raw)
		}
	}
	if len(r.NewModels) > 0 {
		fmt.Printf("\n  top unresolved models under known brands (→ review):\n")
		for _, it := range r.NewModels {
			fmt.Printf("    %6d  %s\n", it.Volume, it.Raw)
		}
	}
}

func cmdResolve(ctx context.Context) error {
	pool, err := store.Connect(ctx, dbURL())
	if err != nil {
		return err
	}
	defer pool.Close()
	thr := floatSetting(ctx, pool, "fuzzy_threshold", 0.92)
	floor := floatSetting(ctx, pool, "fuzzy_review_floor", 0.75)
	fmt.Printf("fuzzy resolve pass (threshold %.2f, review floor %.2f)…\n", thr, floor)
	r, err := review.Resolve(ctx, pool, thr, floor)
	if err != nil {
		return err
	}
	fmt.Printf(`  distinct unresolved (brand,model)   %d
  auto-linked (tier 3)                %d  (%d units)
  proposed → review                   %d
  new-model candidates → review       %d
`, r.Distinct, r.AutoResolved, r.AutoVolume, r.Proposed, r.NewModelCandidate)
	return nil
}

func cmdReview(ctx context.Context) error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: server review <list|confirm|reject|reassign|bulk-confirm> …")
	}
	pool, err := store.Connect(ctx, dbURL())
	if err != nil {
		return err
	}
	defer pool.Close()

	switch os.Args[2] {
	case "list":
		limit := 25
		if len(os.Args) > 3 {
			limit = atoiDefault(os.Args[3], 25)
		}
		items, err := review.List(ctx, pool, limit)
		if err != nil {
			return err
		}
		fmt.Printf("%-8s %-12s %-22s %-22s %-7s %8s\n", "ALIAS", "BRAND", "RAW MODEL", "PROPOSAL", "CONF", "VOLUME")
		for _, it := range items {
			conf := ""
			if it.Confidence != nil {
				conf = fmt.Sprintf("%.2f", *it.Confidence)
			}
			prop := it.Proposal
			if prop == "" {
				prop = "— (new model)"
			}
			fmt.Printf("%-8d %-12.12s %-22.22s %-22.22s %-7s %8d\n",
				it.AliasID, it.Brand, it.RawModel, prop, conf, it.Volume)
		}
		fmt.Printf("(%d items shown)\n", len(items))
		return nil

	case "confirm":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: server review confirm <aliasID> [modelID]")
		}
		id := int64(atoiDefault(os.Args[3], 0))
		var modelID *int64
		if len(os.Args) > 4 {
			m := int64(atoiDefault(os.Args[4], 0))
			modelID = &m
		}
		moved, err := review.Confirm(ctx, pool, id, modelID)
		if err != nil {
			return err
		}
		fmt.Printf("confirmed — %d facts re-derived to the confirmed model\n", moved)
		return nil

	case "reassign":
		if len(os.Args) < 5 {
			return fmt.Errorf("usage: server review reassign <aliasID> <modelID>")
		}
		id := int64(atoiDefault(os.Args[3], 0))
		m := int64(atoiDefault(os.Args[4], 0))
		moved, err := review.Confirm(ctx, pool, id, &m)
		if err != nil {
			return err
		}
		fmt.Printf("reassigned — %d facts re-derived\n", moved)
		return nil

	case "reject":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: server review reject <aliasID>")
		}
		id := int64(atoiDefault(os.Args[3], 0))
		if err := review.Reject(ctx, pool, id); err != nil {
			return err
		}
		fmt.Println("rejected — facts returned to unresolved, alias kept as a negative example")
		return nil

	case "bulk-confirm":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: server review bulk-confirm <minConfidence>")
		}
		minc := floatDefault(os.Args[3], 0.95)
		n, vol, err := review.BulkConfirm(ctx, pool, minc)
		if err != nil {
			return err
		}
		fmt.Printf("bulk-confirmed %d items ≥ %.2f — %d facts re-derived\n", n, minc, vol)
		return nil

	default:
		return fmt.Errorf("unknown review subcommand %q", os.Args[2])
	}
}

func cmdServe() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.Connect(ctx, dbURL())
	if err != nil {
		return err
	}
	defer pool.Close()

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	secure := os.Getenv("SECURE_COOKIES") == "true"
	srv, err := api.New(pool, uploadDir, mcsales.WebDist, secure)
	if err != nil {
		return err
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	httpSrv := &http.Server{
		Addr:              ":" + port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(sctx)
	}()

	fmt.Printf("listening on http://localhost:%s (secure cookies: %v)\n", port, secure)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	fmt.Println("shut down cleanly.")
	return nil
}

func cmdSeedAdmin(ctx context.Context) error {
	pool, err := store.Connect(ctx, dbURL())
	if err != nil {
		return err
	}
	defer pool.Close()
	force := false
	for _, a := range os.Args[2:] {
		if a == "--force" {
			force = true
		}
	}
	email := os.Getenv("ADMIN_EMAIL")
	password := os.Getenv("ADMIN_PASSWORD")
	if err := auth.SeedAdmin(ctx, pool, email, password, force); err != nil {
		return err
	}
	fmt.Printf("admin user %q is ready.\n", email)
	return nil
}

func floatSetting(ctx context.Context, pool *pgxpool.Pool, key string, def float64) float64 {
	var v string
	if err := pool.QueryRow(ctx, `SELECT value FROM settings WHERE key=$1`, key).Scan(&v); err != nil {
		return def
	}
	return floatDefault(v, def)
}

func dbURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://mc:mc@localhost:5433/mcsales?sslmode=disable"
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}

func floatDefault(s string, def float64) float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		return f
	}
	return def
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: server <migrate|seed|migrate-facts|import|import-commit|resolve|review|serve|seed-admin>")
}

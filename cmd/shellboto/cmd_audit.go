package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"go.uber.org/zap"

	"github.com/amiwrpremium/shellboto/internal/db/repo"
)

// cmdAudit dispatches "audit <verb>".
func cmdAudit(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: shellboto audit <verify|search|export|replay> [flags]")
		return exitUsage
	}
	switch args[0] {
	case "verify":
		return cmdAuditVerify(args[1:])
	case "search":
		return cmdAuditSearch(args[1:])
	case "export":
		return cmdAuditExport(args[1:])
	case "replay":
		return cmdAuditReplay(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown audit subcommand %q\n", args[0])
		return exitUsage
	}
}

// openAuditRepo is the common setup for every audit subcommand: load
// config, resolve the seed, open the DB, build the repo.
func openAuditRepo(configPath string) (*repo.AuditRepo, func(), error) {
	cfg, err := loadConfigForCLI(configPath)
	if err != nil {
		return nil, func() {}, err
	}
	seed, _, err := auditSeedDecode()
	if err != nil {
		return nil, func() {}, fmt.Errorf("SHELLBOTO_AUDIT_SEED: %w", err)
	}
	gormDB, cleanup, err := openDBForCLI(cfg.DBPath)
	if err != nil {
		return nil, func() {}, err
	}
	// Journal: drop zap output for CLI callers; the bot is the only
	// writer that mirrors events to journald.
	r := repo.NewAuditRepo(gormDB, seed, zap.NewNop(), repo.AuditOutputMode(cfg.AuditOutputMode), cfg.AuditMaxBlobBytes)
	return r, cleanup, nil
}

// cmdAuditVerify walks the chain and reports the result.
func cmdAuditVerify(args []string) int {
	fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	auditRepo, cleanup, err := openAuditRepo(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	defer cleanup()

	result, err := auditRepo.Verify(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		return exitErr
	}
	if result.OK {
		suffix := ""
		if result.PostPrune {
			suffix = " (post-prune; genesis seed binding skipped)"
		}
		fmt.Printf("✅ audit chain OK — %d rows verified%s.\n", result.VerifiedRows, suffix)
		return exitOK
	}
	fmt.Fprintf(os.Stderr, "❌ audit chain BROKEN at row %d: %s\n", result.FirstBadID, result.Reason)
	return exitCheckFail
}

// cmdAuditSearch prints recent events filtered by user/kind/since.
func cmdAuditSearch(args []string) int {
	fs := flag.NewFlagSet("audit search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	userID := fs.Int64("user", 0, "filter by Telegram user ID (0 = all)")
	kind := fs.String("kind", "", "filter by event kind (e.g. command_run, auth_reject)")
	since := fs.Duration("since", 0, "only events newer than this duration (e.g. 24h)")
	limit := fs.Int("limit", 50, "max rows to return before filtering")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	rows, cleanup, code := loadAuditRows(*configPath, *userID, *limit)
	if code != exitOK {
		return code
	}
	defer cleanup()

	rows = filterRows(rows, *kind, *since)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTS\tUSER\tKIND\tEXIT\tBYTES\tCMD")
	for _, r := range rows {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ID,
			r.TS.UTC().Format("2006-01-02T15:04:05Z"),
			formatUserID(r.UserID),
			r.Kind,
			formatIntPtr(r.ExitCode),
			formatIntPtr(r.BytesOut),
			truncateForCol(r.Cmd, 60),
		)
	}
	_ = w.Flush()
	fmt.Printf("\n%d row(s).\n", len(rows))
	return exitOK
}

// cmdAuditExport streams events as JSONL or CSV.
func cmdAuditExport(args []string) int {
	fs := flag.NewFlagSet("audit export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	userID := fs.Int64("user", 0, "filter by Telegram user ID (0 = all)")
	kind := fs.String("kind", "", "filter by event kind")
	since := fs.Duration("since", 0, "only events newer than this duration")
	limit := fs.Int("limit", 10000, "max rows to fetch before filtering")
	format := fs.String("format", "json", "output format: json | csv")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	rows, cleanup, code := loadAuditRows(*configPath, *userID, *limit)
	if code != exitOK {
		return code
	}
	defer cleanup()

	rows = filterRows(rows, *kind, *since)

	switch *format {
	case "json":
		return writeJSONL(os.Stdout, rows)
	case "csv":
		return writeCSV(os.Stdout, rows)
	default:
		fmt.Fprintf(os.Stderr, "unknown format %q (want json or csv)\n", *format)
		return exitUsage
	}
}

// loadAuditRows is the shared fetch path for search + export.
func loadAuditRows(configPath string, userID int64, limit int) ([]*repo.Row, func(), int) {
	auditRepo, cleanup, err := openAuditRepo(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, cleanup, exitErr
	}
	var userFilter *int64
	if userID != 0 {
		userFilter = &userID
	}
	rows, err := auditRepo.Recent(context.Background(), userFilter, limit)
	if err != nil {
		cleanup()
		fmt.Fprintf(os.Stderr, "query: %v\n", err)
		return nil, func() {}, exitErr
	}
	return rows, cleanup, exitOK
}

// filterRows applies in-memory kind + since filtering. Repo's Recent has
// no built-in filter for either, so we post-filter.
func filterRows(rows []*repo.Row, kind string, since time.Duration) []*repo.Row {
	if kind == "" && since == 0 {
		return rows
	}
	var cutoff time.Time
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}
	out := rows[:0]
	for _, r := range rows {
		if kind != "" && r.Kind != kind {
			continue
		}
		if since > 0 && r.TS.Before(cutoff) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// writeJSONL emits one JSON object per line.
func writeJSONL(w io.Writer, rows []*repo.Row) int {
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(rowToMap(r)); err != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", err)
			return exitErr
		}
	}
	return exitOK
}

// writeCSV emits a header + one row per event.
func writeCSV(w io.Writer, rows []*repo.Row) int {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"id", "ts", "user_id", "kind", "cmd", "exit_code", "bytes_out", "duration_ms", "termination", "danger_pattern", "detail", "has_output"}); err != nil {
		fmt.Fprintf(os.Stderr, "csv header: %v\n", err)
		return exitErr
	}
	for _, r := range rows {
		err := cw.Write([]string{
			strconv.FormatInt(r.ID, 10),
			r.TS.UTC().Format(time.RFC3339Nano),
			formatUserID(r.UserID),
			r.Kind,
			r.Cmd,
			formatIntPtr(r.ExitCode),
			formatIntPtr(r.BytesOut),
			formatInt64Ptr(r.DurationMS),
			r.Termination,
			r.DangerPattern,
			r.Detail,
			strconv.FormatBool(r.HasOutput),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "csv row: %v\n", err)
			return exitErr
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "csv flush: %v\n", err)
		return exitErr
	}
	return exitOK
}

func rowToMap(r *repo.Row) map[string]any {
	m := map[string]any{
		"id":         r.ID,
		"ts":         r.TS.UTC().Format(time.RFC3339Nano),
		"kind":       r.Kind,
		"cmd":        r.Cmd,
		"has_output": r.HasOutput,
	}
	if r.UserID != nil {
		m["user_id"] = *r.UserID
	}
	if r.ExitCode != nil {
		m["exit_code"] = *r.ExitCode
	}
	if r.BytesOut != nil {
		m["bytes_out"] = *r.BytesOut
	}
	if r.DurationMS != nil {
		m["duration_ms"] = *r.DurationMS
	}
	if r.Termination != "" {
		m["termination"] = r.Termination
	}
	if r.DangerPattern != "" {
		m["danger_pattern"] = r.DangerPattern
	}
	if r.Detail != "" {
		m["detail"] = r.Detail
	}
	return m
}

func formatUserID(id *int64) string {
	if id == nil {
		return "-"
	}
	return strconv.FormatInt(*id, 10)
}

func formatIntPtr(i *int) string {
	if i == nil {
		return "-"
	}
	return strconv.Itoa(*i)
}

func formatInt64Ptr(i *int64) string {
	if i == nil {
		return "-"
	}
	return strconv.FormatInt(*i, 10)
}

func truncateForCol(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

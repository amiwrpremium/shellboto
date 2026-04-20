package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/amiwrpremium/shellboto/internal/config"
	"github.com/amiwrpremium/shellboto/internal/db"
	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
)

// writeValidConfig creates a minimal TOML config + sets required env
// vars so config.Load succeeds. Returns the config path.
func writeValidConfig(t *testing.T, dir string) string {
	t.Helper()
	dbPath := filepath.Join(dir, "state.db")
	configPath := filepath.Join(dir, "config.toml")
	body := "db_path = \"" + dbPath + "\"\n"
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("SHELLBOTO_TOKEN", "test-token")
	t.Setenv("SHELLBOTO_SUPERADMIN_ID", "42")
	t.Setenv("SHELLBOTO_AUDIT_SEED", hex.EncodeToString(make([]byte, 32)))
	return configPath
}

func TestDispatch_UnknownCommand(t *testing.T) {
	code := dispatchSubcommand("nope", nil)
	if code != exitUsage {
		t.Fatalf("want exitUsage, got %d", code)
	}
}

func TestDispatch_Help(t *testing.T) {
	if code := dispatchSubcommand("help", nil); code != exitOK {
		t.Fatalf("help returned %d", code)
	}
}

func TestDoctor_AllGreen(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	var buf bytes.Buffer
	code := runDoctor(configPath, &buf)
	if code != exitOK {
		t.Fatalf("doctor returned %d, output:\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "All checks passed") {
		t.Fatalf("doctor output missing success line:\n%s", buf.String())
	}
}

func TestDoctor_MissingSeed(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)
	// Override seed after helper — helper sets a valid one.
	t.Setenv("SHELLBOTO_AUDIT_SEED", "")

	var buf bytes.Buffer
	code := runDoctor(configPath, &buf)
	// Missing seed is a WARN, not a fail — checks still pass, but with
	// the "dev mode" warning.
	if code != exitOK {
		t.Fatalf("doctor returned %d (expected exitOK with warn), output:\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "all-zeros fallback") {
		t.Fatalf("doctor output missing warn line:\n%s", buf.String())
	}
}

func TestDoctor_BadSeed(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)
	t.Setenv("SHELLBOTO_AUDIT_SEED", "not-hex")

	var buf bytes.Buffer
	code := runDoctor(configPath, &buf)
	if code == exitOK {
		t.Fatalf("doctor should have failed, got exitOK. output:\n%s", buf.String())
	}
}

func TestConfigCheck_Positional(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	code := cmdConfig([]string{"check", configPath})
	if code != exitOK {
		t.Fatalf("config check returned %d", code)
	}
}

func TestConfigCheck_BadPath(t *testing.T) {
	// Env vars still set so only the file-not-found path fails.
	t.Setenv("SHELLBOTO_TOKEN", "x")
	t.Setenv("SHELLBOTO_SUPERADMIN_ID", "1")
	code := cmdConfig([]string{"check", "/no/such/file.toml"})
	if code != exitErr {
		t.Fatalf("want exitErr, got %d", code)
	}
}

func TestAuditVerify_EmptyDB(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	code := cmdAuditVerify([]string{"-config", configPath})
	// Empty chain verifies OK with 0 rows.
	if code != exitOK {
		t.Fatalf("audit verify returned %d on empty DB", code)
	}
}

func TestAuditSearch_NoRows(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	// Redirect stdout to capture table output.
	stdout := redirectStdout(t)
	defer stdout.restore()

	code := cmdAuditSearch([]string{"-config", configPath, "--limit", "10"})
	if code != exitOK {
		t.Fatalf("audit search returned %d", code)
	}
	if !strings.Contains(stdout.read(), "0 row(s)") {
		t.Fatalf("expected '0 row(s)' in output, got:\n%s", stdout.read())
	}
}

// synthJournalLines builds zap-shaped journal JSON lines for every row
// currently in audit_events. Used by the audit-replay tests so we can
// feed "journal" input that matches the DB's canonical form exactly.
func synthJournalLines(t *testing.T, gormDB *gorm.DB) []string {
	t.Helper()
	var rows []dbm.AuditEvent
	if err := gormDB.Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		obj := map[string]any{
			"msg":            "audit",
			"id":             r.ID,
			"ts":             r.TS.UTC().Format(time.RFC3339Nano),
			"kind":           r.Kind,
			"cmd":            r.Cmd,
			"termination":    r.Termination,
			"danger_pattern": r.DangerPattern,
			"detail":         r.Detail,
			"output_sha256":  r.OutputSHA256,
			"prev_hash":      hex.EncodeToString(r.PrevHash),
			"row_hash":       hex.EncodeToString(r.RowHash),
		}
		if r.UserID != nil {
			obj["user_id"] = *r.UserID
		}
		if r.ExitCode != nil {
			obj["exit_code"] = *r.ExitCode
		}
		if r.BytesOut != nil {
			obj["bytes_out"] = *r.BytesOut
		}
		if r.DurationMS != nil {
			obj["duration_ms"] = *r.DurationMS
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		lines = append(lines, string(raw))
	}
	return lines
}

func TestAuditReplay_AllClean(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	cfg, _ := config.Load(configPath)
	gormDB, _ := db.Open(cfg.DBPath)
	auditRepo := repo.NewAuditRepo(gormDB, nil, zap.NewNop(), repo.OutputAlways, 0)
	uid := int64(1)
	_, _ = auditRepo.Log(context.Background(), repo.Event{Kind: dbm.KindStartup})
	_, _ = auditRepo.Log(context.Background(), repo.Event{UserID: &uid, Kind: dbm.KindCommandRun, Cmd: "echo hi"})

	journalPath := filepath.Join(dir, "journal.log")
	if err := os.WriteFile(journalPath, []byte(strings.Join(synthJournalLines(t, gormDB), "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	code := cmdAuditReplay([]string{"-config", configPath, "-file", journalPath})
	out := stdout.read()
	if code != exitOK {
		t.Fatalf("replay returned %d (expected OK), output:\n%s", code, out)
	}
	if !strings.Contains(out, "2 OK") || !strings.Contains(out, "0 missing") || !strings.Contains(out, "0 mismatch") {
		t.Fatalf("unexpected summary:\n%s", out)
	}
}

func TestAuditReplay_MissingRow(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	cfg, _ := config.Load(configPath)
	gormDB, _ := db.Open(cfg.DBPath)
	auditRepo := repo.NewAuditRepo(gormDB, nil, zap.NewNop(), repo.OutputAlways, 0)
	_, _ = auditRepo.Log(context.Background(), repo.Event{Kind: dbm.KindStartup})
	_, _ = auditRepo.Log(context.Background(), repo.Event{Kind: dbm.KindStartup})

	lines := synthJournalLines(t, gormDB)
	// Delete the second row from the DB — the journal still claims it exists.
	if err := gormDB.Where("id = 2").Delete(&dbm.AuditEvent{}).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	journalPath := filepath.Join(dir, "journal.log")
	if err := os.WriteFile(journalPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	code := cmdAuditReplay([]string{"-config", configPath, "-file", journalPath})
	out := stdout.read()
	if code != exitCheckFail {
		t.Fatalf("replay returned %d (expected exitCheckFail), output:\n%s", code, out)
	}
	if !strings.Contains(out, "MISSING_IN_DB") {
		t.Fatalf("expected a MISSING_IN_DB line:\n%s", out)
	}
}

func TestAuditReplay_HashMismatch(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	cfg, _ := config.Load(configPath)
	gormDB, _ := db.Open(cfg.DBPath)
	auditRepo := repo.NewAuditRepo(gormDB, nil, zap.NewNop(), repo.OutputAlways, 0)
	_, _ = auditRepo.Log(context.Background(), repo.Event{Kind: dbm.KindStartup})

	lines := synthJournalLines(t, gormDB)
	// Tamper the DB: change the row_hash so the stored value no longer
	// matches the journal's claim.
	if err := gormDB.Exec(`UPDATE audit_events SET row_hash = X'00' WHERE id = 1`).Error; err != nil {
		t.Fatalf("tamper: %v", err)
	}

	journalPath := filepath.Join(dir, "journal.log")
	if err := os.WriteFile(journalPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	code := cmdAuditReplay([]string{"-config", configPath, "-file", journalPath})
	out := stdout.read()
	if code != exitCheckFail {
		t.Fatalf("replay returned %d (expected exitCheckFail), output:\n%s", code, out)
	}
	if !strings.Contains(out, "HASH_MISMATCH") {
		t.Fatalf("expected a HASH_MISMATCH line:\n%s", out)
	}
}

func TestAuditReplay_MalformedLineIgnored(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	cfg, _ := config.Load(configPath)
	gormDB, _ := db.Open(cfg.DBPath)
	auditRepo := repo.NewAuditRepo(gormDB, nil, zap.NewNop(), repo.OutputAlways, 0)
	_, _ = auditRepo.Log(context.Background(), repo.Event{Kind: dbm.KindStartup})

	lines := synthJournalLines(t, gormDB)
	// Prepend junk + an unrelated JSON line. Replay must ignore both.
	mixed := []string{
		"some plaintext that is not JSON",
		`{"msg":"not-audit","note":"unrelated"}`,
	}
	mixed = append(mixed, lines...)

	journalPath := filepath.Join(dir, "journal.log")
	if err := os.WriteFile(journalPath, []byte(strings.Join(mixed, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	code := cmdAuditReplay([]string{"-config", configPath, "-file", journalPath})
	out := stdout.read()
	if code != exitOK {
		t.Fatalf("replay returned %d (junk lines should be ignored), output:\n%s", code, out)
	}
	if !strings.Contains(out, "1 journal entries") || !strings.Contains(out, "1 OK") {
		t.Fatalf("expected '1 journal entries, 1 OK':\n%s", out)
	}
}

func TestAuditExport_JSONL(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	// Seed one audit row via the repo so export has something to emit.
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	gormDB, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	auditRepo := repo.NewAuditRepo(gormDB, nil, zap.NewNop(), repo.OutputAlways, 0)
	uid := int64(42)
	if _, err := auditRepo.Log(context.Background(), repo.Event{
		UserID: &uid, Kind: dbm.KindCommandRun, Cmd: "echo hi",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	code := cmdAuditExport([]string{"-config", configPath, "--format", "json"})
	if code != exitOK {
		t.Fatalf("audit export returned %d", code)
	}
	line := strings.TrimSpace(stdout.read())
	if line == "" {
		t.Fatalf("expected at least one JSONL row")
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatalf("unmarshal: %v\nline: %q", err, line)
	}
	if row["kind"] != "command_run" {
		t.Fatalf("expected kind=command_run, got %v", row["kind"])
	}
}

func TestAuditExport_CSV(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	cfg, _ := config.Load(configPath)
	gormDB, _ := db.Open(cfg.DBPath)
	auditRepo := repo.NewAuditRepo(gormDB, nil, zap.NewNop(), repo.OutputAlways, 0)
	uid := int64(7)
	_, _ = auditRepo.Log(context.Background(), repo.Event{UserID: &uid, Kind: dbm.KindStartup})
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	if code := cmdAuditExport([]string{"-config", configPath, "--format", "csv"}); code != exitOK {
		t.Fatalf("csv export returned %d", code)
	}
	r := csv.NewReader(strings.NewReader(stdout.read()))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected header + at least 1 row, got %d", len(records))
	}
	if records[0][0] != "id" {
		t.Fatalf("expected 'id' column header, got %q", records[0][0])
	}
}

func TestDBInfo(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	stdout := redirectStdout(t)
	defer stdout.restore()

	if code := cmdDBInfo([]string{"-config", configPath}); code != exitOK {
		t.Fatalf("db info returned %d", code)
	}
	out := stdout.read()
	for _, want := range []string{"rows(users)", "rows(audit_events)", "pragma.journal_mode"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestDBBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)
	out := filepath.Join(dir, "snapshot.db")

	if code := cmdDBBackup([]string{"-config", configPath, out}); code != exitOK {
		t.Fatalf("db backup returned %d", code)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("backup file is empty")
	}
}

func TestDBVacuum(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	if code := cmdDBVacuum([]string{"-config", configPath}); code != exitOK {
		t.Fatalf("db vacuum returned %d", code)
	}
}

func TestUsersList_OnlySeededSuper(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	// Seed the super row so ListAll returns at least one entry.
	cfg, _ := config.Load(configPath)
	gormDB, _ := db.Open(cfg.DBPath)
	userRepo := repo.NewUserRepo(gormDB)
	if err := userRepo.SeedSuperadmin(42); err != nil {
		t.Fatalf("seed super: %v", err)
	}
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	if code := cmdUsersList([]string{"-config", configPath}); code != exitOK {
		t.Fatalf("users list returned %d", code)
	}
	out := stdout.read()
	if !strings.Contains(out, "superadmin") || !strings.Contains(out, "42") {
		t.Fatalf("expected super row in output:\n%s", out)
	}
}

func TestUsersTree_SingleSuper(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	cfg, _ := config.Load(configPath)
	gormDB, _ := db.Open(cfg.DBPath)
	if err := repo.NewUserRepo(gormDB).SeedSuperadmin(42); err != nil {
		t.Fatalf("seed super: %v", err)
	}
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	if code := cmdUsersTree([]string{"-config", configPath}); code != exitOK {
		t.Fatalf("users tree returned %d", code)
	}
	out := stdout.read()
	if !strings.Contains(out, "superadmin") || !strings.Contains(out, "(42)") {
		t.Fatalf("expected super root in tree output:\n%s", out)
	}
	// Single-root tree must not print any branch glyphs.
	if strings.ContainsAny(out, "├└") {
		t.Fatalf("single-root tree should have no branch chars:\n%s", out)
	}
}

func TestUsersTree_MultiLevel(t *testing.T) {
	dir := t.TempDir()
	configPath := writeValidConfig(t, dir)

	cfg, _ := config.Load(configPath)
	gormDB, _ := db.Open(cfg.DBPath)
	userRepo := repo.NewUserRepo(gormDB)
	if err := userRepo.SeedSuperadmin(42); err != nil {
		t.Fatalf("seed super: %v", err)
	}
	// Add a user under super, then promote to admin.
	if err := userRepo.Add(100, "user", "alice", 42); err != nil {
		t.Fatalf("add alice: %v", err)
	}
	if err := userRepo.Promote(100, 42); err != nil {
		t.Fatalf("promote alice: %v", err)
	}
	// Alice then adds bob as a user.
	if err := userRepo.Add(200, "user", "bob", 100); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	_ = db.Close(gormDB)

	stdout := redirectStdout(t)
	defer stdout.restore()

	if code := cmdUsersTree([]string{"-config", configPath}); code != exitOK {
		t.Fatalf("users tree returned %d", code)
	}
	out := stdout.read()
	for _, want := range []string{"superadmin", "(42)", "admin", "(100)", "alice", "user", "(200)", "bob",
		"promoted by 42", "added by 100"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in tree output:\n%s", want, out)
		}
	}
	// Tree glyphs must appear.
	if !strings.ContainsAny(out, "├└") {
		t.Fatalf("multi-level tree missing branch chars:\n%s", out)
	}
}

func TestService_UnknownVerb(t *testing.T) {
	if code := cmdService([]string{"nope"}); code != exitUsage {
		t.Fatalf("unknown verb: want exitUsage, got %d", code)
	}
}

func TestService_NoArgsShowsUsage(t *testing.T) {
	if code := cmdService(nil); code != exitUsage {
		t.Fatalf("no args: want exitUsage, got %d", code)
	}
}

func TestService_Help(t *testing.T) {
	stdout := redirectStdout(t)
	defer stdout.restore()
	if code := cmdService([]string{"-h"}); code != exitOK {
		t.Fatalf("-h: want exitOK, got %d", code)
	}
	out := stdout.read()
	for _, want := range []string{"status", "restart", "enable", "disable", "logs"} {
		if !strings.Contains(out, want) {
			t.Fatalf("service help missing %q in output:\n%s", want, out)
		}
	}
}

func TestSimulate_Match(t *testing.T) {
	// simulate doesn't need a real config.
	t.Setenv("SHELLBOTO_TOKEN", "")
	code := cmdSimulate([]string{"rm", "-rf", "/"})
	if code != exitCheckFail {
		t.Fatalf("want exitCheckFail for dangerous cmd, got %d", code)
	}
}

func TestSimulate_Clean(t *testing.T) {
	t.Setenv("SHELLBOTO_TOKEN", "")
	code := cmdSimulate([]string{"ls", "-la"})
	if code != exitOK {
		t.Fatalf("want exitOK for benign cmd, got %d", code)
	}
}

func TestSimulate_NoArgs(t *testing.T) {
	code := cmdSimulate(nil)
	if code != exitUsage {
		t.Fatalf("want exitUsage, got %d", code)
	}
}

func TestMintSeed_Format(t *testing.T) {
	stdout := redirectStdout(t)
	defer stdout.restore()

	if code := cmdMintSeed(nil); code != exitOK {
		t.Fatalf("mint-seed returned %d", code)
	}
	line := strings.TrimSpace(stdout.read())
	if len(line) != 64 { // 32 bytes hex-encoded
		t.Fatalf("expected 64 hex chars, got %d: %q", len(line), line)
	}
	if _, err := hex.DecodeString(line); err != nil {
		t.Fatalf("not valid hex: %v", err)
	}
}

func TestMintSeed_EnvStyle(t *testing.T) {
	stdout := redirectStdout(t)
	defer stdout.restore()

	if code := cmdMintSeed([]string{"-env"}); code != exitOK {
		t.Fatalf("mint-seed -env returned %d", code)
	}
	line := strings.TrimSpace(stdout.read())
	if !strings.HasPrefix(line, "SHELLBOTO_AUDIT_SEED=") {
		t.Fatalf("expected env-style prefix, got %q", line)
	}
}

func TestCompletion_Shells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			stdout := redirectStdout(t)
			defer stdout.restore()
			if code := cmdCompletion([]string{shell}); code != exitOK {
				t.Fatalf("completion %s returned %d", shell, code)
			}
			if stdout.read() == "" {
				t.Fatalf("completion %s produced empty output", shell)
			}
		})
	}
}

func TestCompletion_UnknownShell(t *testing.T) {
	if code := cmdCompletion([]string{"csh"}); code != exitUsage {
		t.Fatalf("want exitUsage for unknown shell, got %d", code)
	}
}

// Integration check: the compiled binary preserves bot-startup behavior
// for unrelated flags (-version) so the systemd unit keeps working.
func TestCompiledBinary_VersionStillWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("requires go build")
	}
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "shellboto")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/shellboto")
	build.Dir = findRepoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, err := exec.Command(binPath, "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("-version failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "shellboto") {
		t.Fatalf("-version output unexpected:\n%s", out)
	}
}

// findRepoRoot walks up from the test's working dir looking for go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	d := cwd
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		d = filepath.Dir(d)
	}
	t.Fatalf("could not find go.mod from %s", cwd)
	return ""
}

// redirectStdout captures everything written to os.Stdout for the test.
type stdoutCapture struct {
	orig *os.File
	r, w *os.File
	buf  *bytes.Buffer
	done chan struct{}
}

func redirectStdout(t *testing.T) *stdoutCapture {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	c := &stdoutCapture{
		orig: os.Stdout,
		r:    r,
		w:    w,
		buf:  &bytes.Buffer{},
		done: make(chan struct{}),
	}
	os.Stdout = w
	go func() {
		_, _ = io.Copy(c.buf, r)
		close(c.done)
	}()
	return c
}

func (c *stdoutCapture) restore() {
	_ = c.w.Close()
	<-c.done
	os.Stdout = c.orig
}

// read returns what's been written so far. Closes the write end on first
// call so the goroutine drains; subsequent calls return the same buffer.
func (c *stdoutCapture) read() string {
	_ = c.w.Close()
	<-c.done
	return c.buf.String()
}

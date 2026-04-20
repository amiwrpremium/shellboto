package repo

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amiwrpremium/shellboto/internal/db"
	"github.com/amiwrpremium/shellboto/internal/db/models"
)

func TestAuditLogAndFetchOutput(t *testing.T) {
	_, audit := newTestRepo(t)
	ctx := context.Background()

	uid := int64(7)
	exit := 0
	bytesOut := 13
	dur := int64(42)
	output := []byte("hello from bash\n")
	id, err := audit.Log(ctx, Event{
		UserID:      &uid,
		Kind:        models.KindCommandRun,
		Cmd:         "echo hi",
		ExitCode:    &exit,
		BytesOut:    &bytesOut,
		DurationMS:  &dur,
		Termination: "completed",
		OutputBody:  output,
	})
	if err != nil || id == 0 {
		t.Fatalf("Log: id=%d err=%v", id, err)
	}

	rows, err := audit.Recent(ctx, nil, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(rows) != 1 || !rows[0].HasOutput {
		t.Fatalf("rows = %+v", rows)
	}

	plain, err := audit.DecompressOutput(ctx, id)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if string(plain) != string(output) {
		t.Fatalf("roundtrip = %q, want %q", plain, output)
	}
}

func TestLatestCommandRun(t *testing.T) {
	_, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(77)

	if _, err := audit.LatestCommandRun(ctx, uid); err == nil {
		t.Fatalf("expected ErrNotFound on empty")
	}
	_, _ = audit.Log(ctx, Event{UserID: &uid, Kind: models.KindShellSpawn})
	if _, err := audit.LatestCommandRun(ctx, uid); err == nil {
		t.Fatalf("should ignore non-command events")
	}
	id1, _ := audit.Log(ctx, Event{UserID: &uid, Kind: models.KindCommandRun})
	id2, _ := audit.Log(ctx, Event{UserID: &uid, Kind: models.KindCommandRun})
	if id1 >= id2 {
		t.Fatalf("ids not monotonic: %d,%d", id1, id2)
	}
	got, err := audit.LatestCommandRun(ctx, uid)
	if err != nil {
		t.Fatalf("LatestCommandRun: %v", err)
	}
	if got != id2 {
		t.Fatalf("got %d, want %d", got, id2)
	}
}

func TestCascadeDeleteOutput(t *testing.T) {
	_, audit := newTestRepo(t)
	ctx := context.Background()

	id, _ := audit.Log(ctx, Event{
		Kind:       models.KindCommandRun,
		Cmd:        "echo x",
		OutputBody: []byte("blob"),
	})
	gz, _, err := audit.FetchOutput(ctx, id)
	if err != nil || len(gz) == 0 {
		t.Fatalf("output not present: %v", err)
	}
	// Delete the event row; FK cascade should drop audit_outputs.
	if _, err := audit.PruneNow(ctx, -time.Hour); err != nil {
		// negative retention means cutoff in the future → delete everything
		t.Fatalf("PruneNow: %v", err)
	}
	if _, _, err := audit.FetchOutput(ctx, id); err == nil {
		t.Fatalf("expected ErrNotFound after cascade")
	}
}

func TestHashChainIntegrityHappy(t *testing.T) {
	_, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(7)

	// Log three events; each should chain to the previous.
	for _, kind := range []string{"startup", "command_run", "shutdown"} {
		_, err := audit.Log(ctx, Event{UserID: &uid, Kind: kind, Cmd: "x"})
		if err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	res, err := audit.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK {
		t.Fatalf("verify failed: %+v", res)
	}
	if res.VerifiedRows != 3 {
		t.Fatalf("verified %d rows, want 3", res.VerifiedRows)
	}
}

func TestHashChainDetectsRowEdit(t *testing.T) {
	store, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(7)

	for _, kind := range []string{"a", "b", "c"} {
		_, _ = audit.Log(ctx, Event{UserID: &uid, Kind: kind})
	}

	// Tamper: flip the kind of the middle row.
	if err := store.DB.Exec(
		"UPDATE audit_events SET kind = 'TAMPERED' WHERE id = (SELECT id FROM audit_events ORDER BY id LIMIT 1 OFFSET 1)",
	).Error; err != nil {
		t.Fatalf("tamper: %v", err)
	}

	res, err := audit.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatalf("verify should have failed after tampering: %+v", res)
	}
	// The tampered row is the second one — either the row_hash (this row)
	// or the prev_hash of the next row will not match.
	if res.FirstBadID == 0 {
		t.Fatalf("no FirstBadID reported: %+v", res)
	}
}

func TestHashChainDetectsDeletedRow(t *testing.T) {
	store, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(7)

	for _, kind := range []string{"a", "b", "c"} {
		_, _ = audit.Log(ctx, Event{UserID: &uid, Kind: kind})
	}

	// Tamper: delete the middle row — this leaves row 3's prev_hash
	// pointing at row 1's row_hash which won't match row 3's recorded
	// prev_hash (it referenced row 2 at insert time).
	if err := store.DB.Exec(
		"DELETE FROM audit_events WHERE id = (SELECT id FROM audit_events ORDER BY id LIMIT 1 OFFSET 1)",
	).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	res, err := audit.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatalf("verify should have failed after deletion: %+v", res)
	}
}

func TestHashChainDifferentSeedsProduceDifferentHashes(t *testing.T) {
	// Two audits with identical events but different seeds → different
	// row_hashes. Confirms seed participates in the hash.
	a1, b1 := makeIsolatedAuditRepo(t, []byte("seed-one-xxxxxxxxxxxxxxxxxxxxx"))
	a2, b2 := makeIsolatedAuditRepo(t, []byte("seed-two-xxxxxxxxxxxxxxxxxxxxx"))
	ctx := context.Background()

	id1, _ := a1.Log(ctx, Event{Kind: "same", Cmd: "same"})
	id2, _ := a2.Log(ctx, Event{Kind: "same", Cmd: "same"})

	type hashOnly struct{ RowHash []byte }
	var r1, r2 hashOnly
	if err := b1.Table("audit_events").Select("row_hash").Where("id = ?", id1).Scan(&r1).Error; err != nil {
		t.Fatalf("scan1: %v", err)
	}
	if err := b2.Table("audit_events").Select("row_hash").Where("id = ?", id2).Scan(&r2).Error; err != nil {
		t.Fatalf("scan2: %v", err)
	}
	if len(r1.RowHash) == 0 || len(r2.RowHash) == 0 {
		t.Fatalf("row_hash not stored: r1=%d r2=%d", len(r1.RowHash), len(r2.RowHash))
	}
	if string(r1.RowHash) == string(r2.RowHash) {
		t.Fatalf("different seeds produced same hash")
	}
}

func TestAuditOutputModeNeverSkipsBlob(t *testing.T) {
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close(gormDB)
	audit := NewAuditRepo(gormDB, nil, nil, OutputNever, 0)
	ctx := context.Background()

	exit := 0
	id, err := audit.Log(ctx, Event{
		Kind:       models.KindCommandRun,
		Cmd:        "echo hi",
		ExitCode:   &exit,
		OutputBody: []byte("hi\n"),
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if _, _, err := audit.FetchOutput(ctx, id); err == nil {
		t.Fatalf("expected no output to be stored with OutputNever")
	}
}

func TestAuditOutputModeErrorsOnly(t *testing.T) {
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close(gormDB)
	audit := NewAuditRepo(gormDB, nil, nil, OutputErrorsOnly, 0)
	ctx := context.Background()

	// Exit 0 → no output stored.
	exit0 := 0
	okID, _ := audit.Log(ctx, Event{
		Kind: models.KindCommandRun, Cmd: "true",
		ExitCode: &exit0, OutputBody: []byte("ok\n"),
	})
	if _, _, err := audit.FetchOutput(ctx, okID); err == nil {
		t.Errorf("errors_only should skip output on exit=0")
	}

	// Exit != 0 → output stored (redacted).
	exit1 := 1
	badID, _ := audit.Log(ctx, Event{
		Kind: models.KindCommandRun, Cmd: "false",
		ExitCode: &exit1, OutputBody: []byte("permission denied\n"),
	})
	plain, err := audit.DecompressOutput(ctx, badID)
	if err != nil {
		t.Fatalf("errors_only should keep output on exit!=0: %v", err)
	}
	if string(plain) != "permission denied\n" {
		t.Fatalf("roundtrip mismatch: %q", plain)
	}
}

func TestAuditRedactsCmdAndOutput(t *testing.T) {
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close(gormDB)
	audit := NewAuditRepo(gormDB, nil, nil, OutputAlways, 0)
	ctx := context.Background()

	exit := 0
	id, err := audit.Log(ctx, Event{
		Kind:       models.KindCommandRun,
		Cmd:        `mysql -u root -pS3cr3tP4ssw0rd -e "SELECT 1"`,
		ExitCode:   &exit,
		OutputBody: []byte("AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n"),
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Cmd must not contain the original password literal.
	rows, _ := audit.Recent(ctx, nil, 1)
	if len(rows) == 0 {
		t.Fatalf("no row returned")
	}
	if strings.Contains(rows[0].Cmd, "S3cr3tP4ssw0rd") {
		t.Errorf("cmd stored with password in clear: %q", rows[0].Cmd)
	}
	if !strings.Contains(rows[0].Cmd, "[REDACTED]") {
		t.Errorf("cmd missing redaction marker: %q", rows[0].Cmd)
	}

	// Output must not contain the raw AWS key; must contain the placeholder.
	plain, err := audit.DecompressOutput(ctx, id)
	if err != nil {
		t.Fatalf("DecompressOutput: %v", err)
	}
	if strings.Contains(string(plain), "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY") {
		t.Errorf("output contains raw secret: %q", plain)
	}
	if !strings.Contains(string(plain), "[REDACTED]") {
		t.Errorf("output missing redaction marker: %q", plain)
	}
}

func TestAuditMaxBlobBytesDropsOversized(t *testing.T) {
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close(gormDB)
	// 1 KB cap; the 10 KB blob below should be dropped.
	audit := NewAuditRepo(gormDB, nil, nil, OutputAlways, 1024)
	ctx := context.Background()

	big := make([]byte, 10*1024)
	for i := range big {
		big[i] = 'x'
	}
	exit := 0
	id, err := audit.Log(ctx, Event{
		Kind:       models.KindCommandRun,
		Cmd:        "echo big",
		ExitCode:   &exit,
		OutputBody: big,
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if _, _, err := audit.FetchOutput(ctx, id); err == nil {
		t.Fatalf("expected blob to be dropped (oversized)")
	}

	rows, _ := audit.Recent(ctx, nil, 1)
	if len(rows) == 0 {
		t.Fatalf("no audit row")
	}
	detail := rows[0].Detail
	if !strings.Contains(detail, "output_oversized") {
		t.Errorf("detail missing output_oversized flag: %q", detail)
	}
	if !strings.Contains(detail, "output_size") {
		t.Errorf("detail missing output_size: %q", detail)
	}
}

func TestAuditMaxBlobBytesZeroDisables(t *testing.T) {
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close(gormDB)
	// maxBlobBytes=0 → no cap beyond runtime.
	audit := NewAuditRepo(gormDB, nil, nil, OutputAlways, 0)
	ctx := context.Background()

	big := make([]byte, 100*1024)
	for i := range big {
		big[i] = 'y'
	}
	exit := 0
	id, err := audit.Log(ctx, Event{
		Kind: models.KindCommandRun, Cmd: "x", ExitCode: &exit, OutputBody: big,
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if _, _, err := audit.FetchOutput(ctx, id); err != nil {
		t.Fatalf("blob should have been stored: %v", err)
	}
}

func TestAuditMaxBlobBytesPreservesCallerDetail(t *testing.T) {
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close(gormDB)
	audit := NewAuditRepo(gormDB, nil, nil, OutputAlways, 1024)
	ctx := context.Background()

	// Caller passes a map detail AND sends output large enough to trip
	// the oversized flag.
	big := make([]byte, 2*1024)
	for i := range big {
		big[i] = 'z'
	}
	exit := 0
	_, err = audit.Log(ctx, Event{
		Kind:       models.KindCommandRun,
		Cmd:        "x",
		ExitCode:   &exit,
		OutputBody: big,
		Detail:     map[string]any{"output_truncated": true, "cap_bytes": 512},
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	rows, _ := audit.Recent(ctx, nil, 1)
	if len(rows) == 0 {
		t.Fatalf("no row")
	}
	detail := rows[0].Detail
	// Both caller's flag AND our oversized flag should appear.
	if !strings.Contains(detail, "output_truncated") {
		t.Errorf("caller detail lost: %q", detail)
	}
	if !strings.Contains(detail, "output_oversized") {
		t.Errorf("oversized flag missing: %q", detail)
	}
	if !strings.Contains(detail, "cap_bytes") {
		t.Errorf("caller's cap_bytes lost: %q", detail)
	}
}

func TestAuditStripsANSIFromStoredOutput(t *testing.T) {
	_, audit := newTestRepo(t)
	ctx := context.Background()

	// Output containing a CSI clear-screen, OSC window-title, and BEL.
	raw := []byte("line1\n\x1b[2J\x1b[0;0H\x1b]0;pwned\x07line2\x07\n")
	exit := 0
	id, err := audit.Log(ctx, Event{
		Kind: models.KindCommandRun, Cmd: "echo x",
		ExitCode: &exit, OutputBody: raw,
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	plain, err := audit.DecompressOutput(ctx, id)
	if err != nil {
		t.Fatalf("DecompressOutput: %v", err)
	}
	// ESC, BEL, and the OSC body "pwned" (with its terminator) must all be gone.
	if strings.ContainsRune(string(plain), '\x1b') {
		t.Errorf("stored output still contains ESC: %q", plain)
	}
	if strings.ContainsRune(string(plain), '\x07') {
		t.Errorf("stored output still contains BEL: %q", plain)
	}
	if strings.Contains(string(plain), "[2J") || strings.Contains(string(plain), "pwned") {
		t.Errorf("ANSI sequence body leaked into stored output: %q", plain)
	}
	// Content around the escapes should survive.
	if !strings.Contains(string(plain), "line1") || !strings.Contains(string(plain), "line2") {
		t.Errorf("plain text around escapes lost: %q", plain)
	}
}

// TestCanonicalRowHashIsStable locks the hash-chain canonical form
// against accidental change.
//
// It hashes a fully-specified AuditEvent against a known seed + known
// output-sha and compares the result to a hardcoded hex golden. Any
// change that shifts the output — reordering canonicalRow's fields,
// changing a field's json tag, Go's json.Marshal semantics drift,
// altering computeRowHash — breaks this test.
//
// IF YOU INTENTIONALLY CHANGE THE CANONICAL FORM: update the `want`
// below AND acknowledge that every already-stored audit row's
// `row_hash` becomes invalid on the next /audit-verify. Coordinate
// with operators before shipping such a change.
func TestCanonicalRowHashIsStable(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339Nano, "2026-04-18T12:00:00Z")
	uid := int64(42)
	exit := 0
	bytesOut := 13
	dur := int64(250)
	row := &models.AuditEvent{
		TS:            ts,
		UserID:        &uid,
		Kind:          models.KindCommandRun,
		Cmd:           "echo test",
		ExitCode:      &exit,
		BytesOut:      &bytesOut,
		DurationMS:    &dur,
		Termination:   "completed",
		DangerPattern: "",
		Detail:        "",
	}
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	// sha256 of the empty byte string, i.e. a row with no stored output.
	outSHA := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	got := hex.EncodeToString(computeRowHash(seed, row, outSHA))
	// Computed at: 2026-04-19 with canonicalRow fields in declared
	// order (ts, user_id, kind, cmd, exit_code, bytes_out,
	// duration_ms, termination, danger_pattern, detail, output_sha256)
	// and Go 1.25's encoding/json defaults.
	want := "ee2e43a5e2aa638fd25040277195ca81cc64e42180818a729e48a825010f8e3a"

	if got != want {
		t.Fatalf("canonical row hash changed — regression lock tripped\n"+
			"  got:  %s\n  want: %s\n"+
			"If this change is intentional (field reorder, new field, etc.): "+
			"update the `want` golden AND acknowledge that EVERY existing "+
			"DB's stored row_hash becomes invalid, so /audit-verify will "+
			"report BROKEN until all pre-change rows prune out.", got, want)
	}
}

func TestVerifyOKAfterPruneFlagsPostPrune(t *testing.T) {
	store, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(7)

	// Log 4 events → chain of 4 rows, ids 1..4.
	for _, k := range []string{"a", "b", "c", "d"} {
		if _, err := audit.Log(ctx, Event{UserID: &uid, Kind: k}); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	// Simulate retention prune: delete the oldest two rows. The
	// remaining rows (ids 3, 4) still chain correctly to each other,
	// but row 3's stored prev_hash points at the now-deleted row 2.
	if err := store.DB.Exec("DELETE FROM audit_events WHERE id IN (1, 2)").Error; err != nil {
		t.Fatalf("prune: %v", err)
	}

	res, err := audit.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK {
		t.Fatalf("post-prune Verify should be OK, got %+v", res)
	}
	if !res.PostPrune {
		t.Fatalf("PostPrune flag should be set, got %+v", res)
	}
	if res.VerifiedRows != 2 {
		t.Fatalf("verified %d rows, want 2", res.VerifiedRows)
	}
}

func TestVerifyPostPruneStillDetectsInternalTampering(t *testing.T) {
	// After pruning, chain continuity between surviving rows must
	// still be enforced — otherwise an attacker could rewrite rows
	// post-prune undetected beyond row 1.
	store, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(7)
	for _, k := range []string{"a", "b", "c", "d"} {
		if _, err := audit.Log(ctx, Event{UserID: &uid, Kind: k}); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}
	// Prune rows 1 and 2, then tamper with row 3's kind (breaks row 4's
	// prev_hash → row 3's row_hash chain link).
	if err := store.DB.Exec("DELETE FROM audit_events WHERE id IN (1, 2)").Error; err != nil {
		t.Fatalf("prune: %v", err)
	}
	if err := store.DB.Exec("UPDATE audit_events SET kind = 'TAMPERED' WHERE id = 3").Error; err != nil {
		t.Fatalf("tamper: %v", err)
	}

	res, err := audit.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatalf("tampering should be detected even post-prune: %+v", res)
	}
	if res.FirstBadID == 0 {
		t.Fatalf("FirstBadID unset: %+v", res)
	}
}

// TestVerifyUsesStoredOutputSHA256 is the fast-path regression
// lock. New rows with output populate the output_sha256 column at
// insert time. Verify's chain check must read that column (not
// decompress the blob). We prove the "reads the column" path by
// nulling the audit_outputs.gz_body AFTER insert — Verify still
// passes because it uses the stored SHA, which matches the
// chain-computed value.
func TestVerifyUsesStoredOutputSHA256(t *testing.T) {
	store, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(7)

	id, err := audit.Log(ctx, Event{
		UserID: &uid, Kind: models.KindCommandRun,
		Cmd:        "echo hi",
		OutputBody: []byte("hi\n"),
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	// Confirm the column was populated.
	var got string
	if err := store.DB.Raw("SELECT output_sha256 FROM audit_events WHERE id = ?", id).Scan(&got).Error; err != nil {
		t.Fatalf("scan sha: %v", err)
	}
	if got == "" {
		t.Fatalf("output_sha256 not populated on insert")
	}

	// Corrupt the gz_body so decompress would fail. Verify must NOT
	// touch audit_outputs; it should read output_sha256 from events.
	if err := store.DB.Exec("UPDATE audit_outputs SET gz_body = ? WHERE audit_id = ?", []byte{0x00, 0x01, 0x02}, id).Error; err != nil {
		t.Fatalf("corrupt gz: %v", err)
	}

	res, err := audit.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK {
		t.Fatalf("Verify should pass using stored SHA, got %+v", res)
	}
}

// TestVerifyLegacyRowFallback is the legacy-row safety test.
// Simulates a pre-column row by nulling output_sha256 on an existing
// row. Verify must fall back to the decompress path and still validate.
func TestVerifyLegacyRowFallback(t *testing.T) {
	store, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(7)

	id, err := audit.Log(ctx, Event{
		UserID: &uid, Kind: models.KindCommandRun,
		Cmd:        "echo hi",
		OutputBody: []byte("hi\n"),
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	// Wipe the stored SHA to simulate a legacy row.
	if err := store.DB.Exec("UPDATE audit_events SET output_sha256 = '' WHERE id = ?", id).Error; err != nil {
		t.Fatalf("null sha: %v", err)
	}
	// Leave gz_body intact — the fallback path decompresses it.
	res, err := audit.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK {
		t.Fatalf("legacy fallback Verify should pass, got %+v", res)
	}
}

func TestAuditLogConcurrentPreservesChain(t *testing.T) {
	_, audit := newTestRepo(t)
	ctx := context.Background()

	// Fire N concurrent Log calls. Without the serialization mutex,
	// two (or more) txs can race on "read latest + insert new row",
	// producing sibling rows with identical prev_hash and breaking
	// Verify. The mutex ensures a linear chain even under full-speed
	// concurrency.
	const N = 64
	var wg sync.WaitGroup
	wg.Add(N)
	uid := int64(7)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			_, err := audit.Log(ctx, Event{
				UserID: &uid,
				Kind:   models.KindCommandRun,
				Cmd:    fmt.Sprintf("cmd-%d", i),
			})
			if err != nil {
				t.Errorf("concurrent Log[%d]: %v", i, err)
			}
		}()
	}
	wg.Wait()

	res, err := audit.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK {
		t.Fatalf("chain broken under concurrency: %+v", res)
	}
	if res.VerifiedRows != N {
		t.Fatalf("verified %d rows, want %d", res.VerifiedRows, N)
	}
}

// TestAuditRedactsDetailJSON locks in detail-map scrubbing: if a
// caller places secret-shaped strings in the Event.Detail map, they
// must be scrubbed before storage so a future misuse can't leak a
// token into the audit DB.
func TestAuditRedactsDetailJSON(t *testing.T) {
	_, audit := newTestRepo(t)
	ctx := context.Background()
	uid := int64(7)
	_, err := audit.Log(ctx, Event{
		UserID: &uid,
		Kind:   models.KindAuthReject,
		Detail: map[string]any{
			"source":       "text",
			"leaked_aws":   "AKIAIOSFODNN7EXAMPLE",
			"leaked_pwarg": "--password=hunter2",
			"username":     "@alice", // non-secret, must survive
		},
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	rows, err := audit.Recent(ctx, nil, 1)
	if err != nil || len(rows) == 0 {
		t.Fatalf("Recent: err=%v rows=%d", err, len(rows))
	}
	detail := rows[0].Detail
	if strings.Contains(detail, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("AWS-shaped key leaked into detail: %q", detail)
	}
	if strings.Contains(detail, "hunter2") {
		t.Errorf("password literal leaked into detail: %q", detail)
	}
	if !strings.Contains(detail, "[REDACTED") {
		t.Errorf("expected some [REDACTED…] marker in detail: %q", detail)
	}
	// Non-secret fields must still be present.
	if !strings.Contains(detail, "@alice") {
		t.Errorf("non-secret field dropped: %q", detail)
	}
	if !strings.Contains(detail, "\"source\"") {
		t.Errorf("non-secret key dropped: %q", detail)
	}
}

func TestPruneByRetention(t *testing.T) {
	_, audit := newTestRepo(t)
	ctx := context.Background()

	id, _ := audit.Log(ctx, Event{Kind: models.KindStartup})
	n, err := audit.PruneNow(ctx, -time.Hour) // cutoff = now + 1h → everything
	if err != nil {
		t.Fatalf("PruneNow: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected ≥1 deletion, got %d", n)
	}
	if _, _, err := audit.FetchOutput(ctx, id); err == nil {
		t.Fatalf("row should be gone")
	}
}

// TestComputeExpectedRowHash_MatchesInsertPath locks in that the
// exported helper used by `shellboto audit replay` produces the same
// hash the insert path does. Insert a row, read it back from the DB,
// project into JournalRow, recompute — hashes must be equal.
func TestComputeExpectedRowHash_MatchesInsertPath(t *testing.T) {
	store, audit := newTestRepo(t)
	ctx := context.Background()

	uid := int64(7)
	exit := 0
	bytesOut := 42
	dur := int64(123)
	id, err := audit.Log(ctx, Event{
		UserID:      &uid,
		Kind:        models.KindCommandRun,
		Cmd:         "echo hi",
		ExitCode:    &exit,
		BytesOut:    &bytesOut,
		DurationMS:  &dur,
		Termination: "completed",
		OutputBody:  []byte("hello from bash\n"),
	})
	if err != nil || id == 0 {
		t.Fatalf("Log: id=%d err=%v", id, err)
	}

	// Read back the stored event so we get the *exact* TS + stored
	// OutputSHA256 + PrevHash that contributed to the chain.
	var row models.AuditEvent
	if err := store.DB.First(&row, id).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}

	jr := JournalRow{
		TS:            row.TS,
		UserID:        row.UserID,
		Kind:          row.Kind,
		Cmd:           row.Cmd,
		ExitCode:      row.ExitCode,
		BytesOut:      row.BytesOut,
		DurationMS:    row.DurationMS,
		Termination:   row.Termination,
		DangerPattern: row.DangerPattern,
		Detail:        row.Detail,
		OutputSHA256:  row.OutputSHA256,
	}
	got := ComputeExpectedRowHash(row.PrevHash, jr)
	if !bytes.Equal(got, row.RowHash) {
		t.Fatalf("exported hash != stored row_hash\n got:  %x\n want: %x", got, row.RowHash)
	}
}

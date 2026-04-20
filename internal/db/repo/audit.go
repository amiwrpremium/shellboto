package repo

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/redact"
)

// AuditOutputMode controls whether command output is persisted.
type AuditOutputMode string

const (
	OutputAlways     AuditOutputMode = "always"
	OutputErrorsOnly AuditOutputMode = "errors_only"
	OutputNever      AuditOutputMode = "never"
)

const cmdDisplayCap = 4096

// Event is the DTO callers use to log an audit row.
type Event struct {
	UserID        *int64
	Kind          string
	Cmd           string
	ExitCode      *int
	BytesOut      *int
	DurationMS    *int64
	Termination   string
	DangerPattern string
	Detail        any
	OutputBody    []byte // gzipped on insert when non-empty
}

// Row is a projection returned by Recent.
type Row struct {
	ID            int64
	TS            time.Time
	UserID        *int64
	Kind          string
	Cmd           string
	ExitCode      *int
	BytesOut      *int
	DurationMS    *int64
	Termination   string
	DangerPattern string
	Detail        string
	HasOutput     bool
}

// AuditRepo holds audit-table queries and hash-chain state.
type AuditRepo struct {
	db           *gorm.DB
	seed         []byte      // genesis prev_hash for the first row ever chained
	journal      *zap.Logger // parallel sink — every Log call also writes here
	outputMode   AuditOutputMode
	maxBlobBytes int // 0 = unlimited

	// logMu serializes Log calls end-to-end. Without it, two concurrent
	// Log goroutines open BEGIN DEFERRED txs, both read the same
	// `latest` under their own snapshot, and both insert rows with the
	// same prev_hash — which breaks the chain on the next Verify.
	// The mutex matches the audit log's semantic (strictly sequential
	// by design) and has negligible cost since audit writes are fast
	// (~ms). It also orders the journal mirror after the DB insert
	// consistently across goroutines.
	logMu sync.Mutex
}

// NewAuditRepo constructs an audit repository.
//   - seed: the genesis prev_hash used when no prior chained rows exist.
//     Pass nil or an empty slice to fall back to an all-zeros seed (dev
//     mode). Production callers should pass a 32-byte secret from
//     SHELLBOTO_AUDIT_SEED so an attacker can't forge a fresh chain.
//   - journal: a zap logger (typically named "audit") that receives a
//     structured Info-level record per audit event. Pass zap.NewNop() to
//     disable. The journald sink is the second leg of tamper-evidence
//     (DB rows + journald mirror must both be corrupted to hide tracks).
//   - outputMode: controls whether the captured stdout/stderr of
//     command_run events is persisted. Regardless of mode, `cmd` and
//     stored output (when kept) are always passed through the redact
//     scrubber — mode only chooses whether output is stored at all.
//   - maxBlobBytes: post-redact size cap for the stored blob. 0 = no
//     cap beyond what runtime already enforces via shell.Job.MaxBytes.
//     Oversized redacted outputs are dropped from storage and flagged
//     as `output_oversized:true` in the audit row's `detail`.
func NewAuditRepo(db *gorm.DB, seed []byte, journal *zap.Logger, outputMode AuditOutputMode, maxBlobBytes int) *AuditRepo {
	if len(seed) == 0 {
		seed = make([]byte, 32) // all-zeros fallback
	}
	if journal == nil {
		journal = zap.NewNop()
	}
	if outputMode == "" {
		outputMode = OutputAlways
	}
	return &AuditRepo{
		db:           db,
		seed:         seed,
		journal:      journal,
		outputMode:   outputMode,
		maxBlobBytes: maxBlobBytes,
	}
}

// Log persists an event, computes + stores the hash chain link, and
// mirrors the event to the journal logger.
//
// Redaction: `cmd` and (when stored) the output blob are passed through
// the redact scrubber before hashing / storing / journaling. The hash
// chain attests to the *redacted* content — an attacker who edits the
// stored redacted row still breaks the chain.
func (a *AuditRepo) Log(ctx context.Context, e Event) (int64, error) {
	// Serialize the read-latest + insert sequence across
	// goroutines so the hash chain stays linear under concurrent writes.
	a.logMu.Lock()
	defer a.logMu.Unlock()

	redactedCmd := redact.RedactString(e.Cmd)
	redactedOutput := a.outputToStore(e)

	// Size cap: drop the blob if the redacted output exceeds the
	// configured BLOB cap; surface the fact in `detail`.
	oversized := false
	rawRedactedSize := len(redactedOutput)
	if a.maxBlobBytes > 0 && rawRedactedSize > a.maxBlobBytes {
		oversized = true
		redactedOutput = nil
	}

	row := &models.AuditEvent{
		TS:            time.Now().UTC(),
		UserID:        e.UserID,
		Kind:          e.Kind,
		Cmd:           truncate(redactedCmd, cmdDisplayCap),
		ExitCode:      e.ExitCode,
		BytesOut:      e.BytesOut,
		DurationMS:    e.DurationMS,
		Termination:   e.Termination,
		DangerPattern: e.DangerPattern,
		// OutputSHA256 is populated below after outHash is computed.
	}
	if merged := mergeDetail(e.Detail, oversized, rawRedactedSize, a.maxBlobBytes); merged != nil {
		// Redact string values inside the detail BEFORE
		// marshal. Running redact on the serialized JSON byte stream
		// is unsafe — some redact patterns end with greedy `\S+`
		// which would consume past quote/comma boundaries and corrupt
		// the JSON. Per-value scrubbing stays well-contained.
		redactStringsInPlace(merged)
		b, err := json.Marshal(merged)
		if err != nil {
			return 0, err
		}
		row.Detail = string(b)
	}

	outHash := ""
	if len(redactedOutput) > 0 {
		sum := sha256.Sum256(redactedOutput)
		outHash = hex.EncodeToString(sum[:])
	}
	// Persist the output hash on the audit_events row so
	// Verify can re-check the chain without a decompress round-trip
	// per row. Empty when there's no output.
	row.OutputSHA256 = outHash

	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Fetch the previous row's row_hash (if any) for the chain link.
		var prev []byte
		var latest models.AuditEvent
		ferr := tx.Where("row_hash IS NOT NULL").
			Order("id DESC").Limit(1).
			First(&latest).Error
		if ferr != nil && !errors.Is(ferr, gorm.ErrRecordNotFound) {
			return ferr
		}
		if ferr == nil {
			prev = latest.RowHash
		}
		if len(prev) == 0 {
			prev = a.seed
		}
		row.PrevHash = prev
		row.RowHash = computeRowHash(prev, row, outHash)

		if err := tx.Create(row).Error; err != nil {
			return err
		}
		if len(redactedOutput) > 0 {
			var gz bytes.Buffer
			// BestSpeed (level 1): ~3-4× faster than DefaultCompression
			// for ~5% larger output. Audit writes are on the hot path.
			w, err := gzip.NewWriterLevel(&gz, gzip.BestSpeed)
			if err != nil {
				return err
			}
			if _, err := w.Write(redactedOutput); err != nil {
				return err
			}
			if err := w.Close(); err != nil {
				return err
			}
			return tx.Create(&models.AuditOutput{
				AuditID:  row.ID,
				GzBody:   gz.Bytes(),
				OrigSize: len(redactedOutput),
			}).Error
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Journal duplicate: structured record on stderr (captured by
	// journald). Mirrors the chained-row content so a full wipe of the
	// DB leaves journald as a secondary source of truth.
	a.journal.Info("audit",
		zap.Int64("id", row.ID),
		zap.Time("ts", row.TS),
		zap.Int64p("user_id", e.UserID),
		zap.String("kind", row.Kind),
		zap.String("cmd", row.Cmd),
		zap.Intp("exit_code", e.ExitCode),
		zap.Intp("bytes_out", e.BytesOut),
		zap.Int64p("duration_ms", e.DurationMS),
		zap.String("termination", row.Termination),
		zap.String("danger_pattern", row.DangerPattern),
		zap.String("detail", row.Detail),
		zap.String("output_sha256", outHash),
		zap.String("prev_hash", hex.EncodeToString(row.PrevHash)),
		zap.String("row_hash", hex.EncodeToString(row.RowHash)),
	)

	return row.ID, nil
}

// canonicalRow is the stable serialization used for hash computation.
// Fields are declared in a fixed order; changing this struct's shape
// invalidates all existing rows. Use `json,omitempty` with pointer types
// so a nil field produces a deterministic absence instead of a zero.
type canonicalRow struct {
	TS            string `json:"ts"`
	UserID        *int64 `json:"user_id,omitempty"`
	Kind          string `json:"kind"`
	Cmd           string `json:"cmd,omitempty"`
	ExitCode      *int   `json:"exit_code,omitempty"`
	BytesOut      *int   `json:"bytes_out,omitempty"`
	DurationMS    *int64 `json:"duration_ms,omitempty"`
	Termination   string `json:"termination,omitempty"`
	DangerPattern string `json:"danger_pattern,omitempty"`
	Detail        string `json:"detail,omitempty"`
	OutputSHA256  string `json:"output_sha256,omitempty"`
}

// canonical returns the deterministic JSON of the chain-relevant fields.
func canonical(row *models.AuditEvent, outputSHA256 string) []byte {
	c := canonicalRow{
		TS:            row.TS.UTC().Format(time.RFC3339Nano),
		UserID:        row.UserID,
		Kind:          row.Kind,
		Cmd:           row.Cmd,
		ExitCode:      row.ExitCode,
		BytesOut:      row.BytesOut,
		DurationMS:    row.DurationMS,
		Termination:   row.Termination,
		DangerPattern: row.DangerPattern,
		Detail:        row.Detail,
		OutputSHA256:  outputSHA256,
	}
	b, _ := json.Marshal(c) // struct fields marshal in declared order
	return b
}

// computeRowHash = sha256(prev || canonical(row)).
func computeRowHash(prev []byte, row *models.AuditEvent, outputSHA256 string) []byte {
	h := sha256.New()
	h.Write(prev)
	h.Write(canonical(row, outputSHA256))
	return h.Sum(nil)
}

// JournalRow is the subset of an audit-event's fields that feed into
// the chain-hash canonical form. Exported for offline verification
// (`shellboto audit replay`), which reconstructs these from the zap
// journal JSON and needs to recompute `row_hash` without touching the
// DB. Field semantics match the internal canonical form exactly;
// changing either breaks every existing chain.
type JournalRow struct {
	TS            time.Time
	UserID        *int64
	Kind          string
	Cmd           string
	ExitCode      *int
	BytesOut      *int
	DurationMS    *int64
	Termination   string
	DangerPattern string
	Detail        string
	OutputSHA256  string
}

// ComputeExpectedRowHash returns sha256(prev || canonical(row)) for a
// JournalRow. Delegates to the private `canonical` + sha256 path so
// the on-disk hash semantics stay in one place; the exported helper
// is a thin shim around a *models.AuditEvent projection of the
// journal row.
func ComputeExpectedRowHash(prev []byte, r JournalRow) []byte {
	ev := &models.AuditEvent{
		TS:            r.TS,
		UserID:        r.UserID,
		Kind:          r.Kind,
		Cmd:           r.Cmd,
		ExitCode:      r.ExitCode,
		BytesOut:      r.BytesOut,
		DurationMS:    r.DurationMS,
		Termination:   r.Termination,
		DangerPattern: r.DangerPattern,
		Detail:        r.Detail,
	}
	return computeRowHash(prev, ev, r.OutputSHA256)
}

// outputToStore returns the scrubbed output bytes that should be
// persisted for this event, honoring AuditOutputMode.
//
// Two scrub passes apply to every stored blob:
//   - redact.Redact          — mask secrets (tokens, passwords, keys)
//   - redact.StripTerminalEscapes — remove ANSI / BEL so `zcat` on the
//     stored blob can't clear an operator's screen or retitle windows
//     when viewed via `zcat`.
//
// Non-command_run events with output pass through the same scrub.
func (a *AuditRepo) outputToStore(e Event) []byte {
	if len(e.OutputBody) == 0 {
		return nil
	}
	scrub := func(b []byte) []byte {
		return redact.StripTerminalEscapes(redact.Redact(b))
	}
	if e.Kind != models.KindCommandRun {
		return scrub(e.OutputBody)
	}
	switch a.outputMode {
	case OutputNever:
		return nil
	case OutputErrorsOnly:
		if e.ExitCode != nil && *e.ExitCode == 0 {
			return nil
		}
		return scrub(e.OutputBody)
	default: // OutputAlways
		return scrub(e.OutputBody)
	}
}

// VerifyResult summarizes a chain walk.
type VerifyResult struct {
	OK           bool
	VerifiedRows int
	FirstBadID   int64
	Reason       string
	// PostPrune is true when the oldest surviving row has id > 1,
	// indicating the retention pruner has removed older rows. In that
	// state Verify cannot enforce the genesis seed binding (the first
	// surviving row's stored prev_hash points at a now-deleted row),
	// so it skips that check and only verifies chain continuity
	// between the remaining rows. The journal mirror (the `audit` zap
	// logger) is the source of pre-prune tamper-evidence.
	PostPrune bool
}

// Verify walks the audit_events table in id order, recomputes each
// row_hash, and reports the first mismatch. Rows without a row_hash
// (pre-chain — e.g. inserted before the chained-audit work shipped)
// are skipped.
//
// After the pruner deletes old rows, the first surviving row's
// stored prev_hash no longer matches the seed. Verify detects this
// via the first row's id: id==1 is the unpruned genesis case (enforce
// seed check); id>1 is post-prune (skip seed check, start chain from
// the surviving row's own prev_hash). Chain continuity for every
// subsequent row is still verified either way.
//
// Rows are materialized up-front rather than iterated via a live
// Rows cursor. This avoids holding the single pooled DB connection
// while issuing the per-row outputHashFor sub-query, which
// would otherwise deadlock. Memory cost is bounded by retention; a
// DB with many millions of rows could still matter, in which case
// paginate this in 10k-row chunks.
func (a *AuditRepo) Verify(ctx context.Context) (VerifyResult, error) {
	var rows []models.AuditEvent
	if err := a.db.WithContext(ctx).
		Where("row_hash IS NOT NULL").
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return VerifyResult{}, err
	}

	var prevHash []byte
	var expectedPrev []byte
	var postPrune bool
	for i := range rows {
		row := rows[i]

		if i == 0 {
			if row.ID == 1 {
				// Unpruned: first row must link to the genesis seed.
				expectedPrev = a.seed
			} else {
				// Pruned: use the surviving row's stored prev_hash as
				// our starting baseline. Genesis binding is lost but
				// chain continuity from here onward is still checked.
				expectedPrev = row.PrevHash
				postPrune = true
			}
		} else {
			expectedPrev = prevHash
		}
		if !bytes.Equal(row.PrevHash, expectedPrev) {
			return VerifyResult{
				OK:           false,
				VerifiedRows: i,
				FirstBadID:   row.ID,
				Reason: fmt.Sprintf("prev_hash mismatch: row %d expected prev=%s, got %s",
					row.ID, hex.EncodeToString(expectedPrev), hex.EncodeToString(row.PrevHash)),
				PostPrune: postPrune,
			}, nil
		}

		// Recompute row_hash, including the output blob's hash if any.
		// Prefer the stored OutputSHA256 column (O(1) read);
		// fall back to decompress-and-hash only for legacy rows that
		// predate the column. New rows skip every audit_outputs
		// round-trip.
		var outHash string
		if row.OutputSHA256 != "" {
			outHash = row.OutputSHA256
		} else {
			hashed, err := a.outputHashFor(ctx, row.ID)
			if err != nil {
				return VerifyResult{}, err
			}
			outHash = hashed
		}
		want := computeRowHash(row.PrevHash, &row, outHash)
		if !bytes.Equal(row.RowHash, want) {
			return VerifyResult{
				OK:           false,
				VerifiedRows: i,
				FirstBadID:   row.ID,
				Reason: fmt.Sprintf("row_hash mismatch: row %d expected %s, got %s",
					row.ID, hex.EncodeToString(want), hex.EncodeToString(row.RowHash)),
				PostPrune: postPrune,
			}, nil
		}
		prevHash = row.RowHash
	}
	return VerifyResult{OK: true, VerifiedRows: len(rows), PostPrune: postPrune}, nil
}

// outputHashFor returns the hex SHA-256 of the uncompressed output blob
// for an audit id, or "" if there is no attached output.
func (a *AuditRepo) outputHashFor(ctx context.Context, id int64) (string, error) {
	plain, err := a.DecompressOutput(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	sum := sha256.Sum256(plain)
	return hex.EncodeToString(sum[:]), nil
}

// Recent returns the latest n rows, optionally filtered by user.
func (a *AuditRepo) Recent(ctx context.Context, userID *int64, n int) ([]*Row, error) {
	if n <= 0 {
		n = 20
	}
	q := a.db.WithContext(ctx).Table("audit_events AS ae").
		Select(`ae.id, ae.ts, ae.user_id, ae.kind, COALESCE(ae.cmd,'') AS cmd,
			ae.exit_code, ae.bytes_out, ae.duration_ms,
			COALESCE(ae.termination,'') AS termination,
			COALESCE(ae.danger_pattern,'') AS danger_pattern,
			COALESCE(ae.detail,'') AS detail,
			EXISTS(SELECT 1 FROM audit_outputs o WHERE o.audit_id = ae.id) AS has_output`).
		Order("ae.id DESC").Limit(n)
	if userID != nil {
		q = q.Where("ae.user_id = ?", *userID)
	}
	var rows []*Row
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// LatestCommandRun returns the id of the newest command_run row for a user.
func (a *AuditRepo) LatestCommandRun(ctx context.Context, userID int64) (int64, error) {
	var id int64
	err := a.db.WithContext(ctx).Model(&models.AuditEvent{}).
		Where("user_id = ? AND kind = ?", userID, models.KindCommandRun).
		Order("id DESC").Limit(1).
		Pluck("id", &id).Error
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, ErrNotFound
	}
	return id, nil
}

// LastActivity returns the ts of the most recent row for a user.
func (a *AuditRepo) LastActivity(ctx context.Context, userID int64) (time.Time, error) {
	var row models.AuditEvent
	err := a.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").Limit(1).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return time.Time{}, ErrNotFound
		}
		return time.Time{}, err
	}
	return row.TS, nil
}

// FetchOutput returns the gzipped body + original uncompressed size.
func (a *AuditRepo) FetchOutput(ctx context.Context, auditID int64) ([]byte, int, error) {
	var out models.AuditOutput
	err := a.db.WithContext(ctx).First(&out, "audit_id = ?", auditID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	return out.GzBody, out.OrigSize, nil
}

// DecompressOutput returns the plain-text output for an audit row.
func (a *AuditRepo) DecompressOutput(ctx context.Context, auditID int64) ([]byte, error) {
	gz, _, err := a.FetchOutput(ctx, auditID)
	if err != nil {
		return nil, err
	}
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}

// Pruner deletes rows older than retention on a ticker.
//
// Each pruner tick runs inside a recover closure so a panic
// inside PruneNow (e.g., a driver error surfaced as a panic, or a
// future contributor's bug) can't silently kill the pruner goroutine
// — that would stop retention enforcement until next restart, letting
// the audit DB grow unbounded. The recover logs at Error on the
// audit journal logger; operators grep journald for
// `"audit pruner panic"` to find it.
func (a *AuditRepo) Pruner(ctx context.Context, retention time.Duration) {
	if retention <= 0 {
		return
	}
	run := func() {
		defer func() {
			if r := recover(); r != nil {
				a.journal.Error("audit pruner panic",
					zap.Any("panic", r),
					zap.Stack("stack"))
			}
		}()
		_, _ = a.PruneNow(ctx, retention)
	}
	run()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// PruneNow deletes rows older than retention immediately.
//
// Pruning DELETES old rows from the tail of the chain. The chain walk
// still works for the remaining rows: Verify uses the seed as the
// expected prev_hash only for the FIRST remaining row, which means
// pruning always breaks the expected-prev relationship for the new
// first row. To accommodate this, Verify treats the seed as the
// expected prev_hash of whatever the oldest surviving row is — so a
// legitimate prune is indistinguishable from a malicious deletion of
// the oldest rows. That's an acknowledged limitation; treat prune
// events as an attested operation.
func (a *AuditRepo) PruneNow(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention).UTC()
	res := a.db.WithContext(ctx).Where("ts < ?", cutoff).Delete(&models.AuditEvent{})
	return res.RowsAffected, res.Error
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("… [+%d bytes]", len(s)-n)
}

// redactStringsInPlace walks a detail-map (potentially nested) and
// applies redact.RedactString to every string leaf value. Used
// before JSON-marshal so the per-pattern `\S+` tails can't eat past a
// value's end into JSON syntax. Handles map[string]any and
// []any; leaves non-string scalars alone.
func redactStringsInPlace(v any) {
	switch node := v.(type) {
	case map[string]any:
		for k, val := range node {
			switch inner := val.(type) {
			case string:
				node[k] = redact.RedactString(inner)
			case map[string]any, []any:
				redactStringsInPlace(inner)
			}
		}
	case []any:
		for i, val := range node {
			switch inner := val.(type) {
			case string:
				node[i] = redact.RedactString(inner)
			case map[string]any, []any:
				redactStringsInPlace(inner)
			}
		}
	}
}

// mergeDetail returns the final `detail` value for an audit row,
// combining the caller-provided Event.Detail with the oversized flag.
// Never mutates the caller's map. Returns nil when nothing needs to be
// stored (caller had no detail AND the blob wasn't oversized).
func mergeDetail(original any, oversized bool, rawSize, cap int) any {
	if original == nil && !oversized {
		return nil
	}
	m, ok := original.(map[string]any)
	if !ok {
		if original != nil {
			// Non-map caller detail: nest under a wrapper key so our
			// oversized flag sits next to it without collision.
			m = map[string]any{"_detail": original}
		} else {
			m = map[string]any{}
		}
	} else {
		// Copy to avoid mutating caller state.
		clone := make(map[string]any, len(m)+3)
		for k, v := range m {
			clone[k] = v
		}
		m = clone
	}
	if oversized {
		m["output_oversized"] = true
		m["output_size"] = rawSize
		m["output_cap"] = cap
	}
	return m
}

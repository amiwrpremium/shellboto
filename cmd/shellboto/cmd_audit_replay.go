package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"gorm.io/gorm"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
)

// cmdAuditReplay reads the zap-JSON journal mirror of the audit log
// (captured by journald: `journalctl -u shellboto -o cat`) and
// cross-references each audit entry with the DB.
//
// Three outcomes per journal entry:
//   - OK            — DB has the row and its stored row_hash matches
//     what recomputing from the journal canonical form
//     produces.
//   - MISSING_IN_DB — journal has a row with this id; DB does not.
//     (Could be pruned retention, or tampering.)
//   - HASH_MISMATCH — both have the row; hashes differ.
//
// Exit codes: 0 = all clean, 3 = at least one flagged row, 1 = parser
// error.
//
// Caveats:
//   - Journald may rotate and/or be size-capped. Replay only covers
//     the entries the caller hands us. Partial coverage is normal.
//   - The zap JSON line has two "ts" keys (zap's auto-timestamp and
//     our audit row's zap.Time("ts", ...)). standard JSON last-wins,
//     which gives us the audit row's ts — what we need.
//   - output_sha256 is verified as an opaque string; replay doesn't
//     decompress the blob (that's `audit verify`'s job).
func cmdAuditReplay(args []string) int {
	fs := flag.NewFlagSet("audit replay", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	filePath := fs.String("file", "", "read journal from FILE instead of stdin")
	verbose := fs.Bool("verbose", false, "print OK lines in addition to flagged rows")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, err := loadConfigForCLI(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	gormDB, cleanup, err := openDBForCLI(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	defer cleanup()

	var reader io.Reader = os.Stdin
	if *filePath != "" {
		f, err := os.Open(*filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open %s: %v\n", *filePath, err)
			return exitErr
		}
		defer f.Close()
		reader = f
	}

	fmt.Println("audit replay — reading journal entries")

	scanner := bufio.NewScanner(reader)
	// Generous per-line cap — audit JSON with a big `detail` can be
	// long. 1 MiB covers any realistic row.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var total, okCount, missingCount, mismatchCount int
	for scanner.Scan() {
		entry, ok := parseJournalLine(scanner.Bytes())
		if !ok {
			continue // non-audit line, or unparseable
		}
		total++
		result, err := compareEntry(gormDB, entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "id %d: lookup error: %v\n", entry.ID, err)
			continue
		}
		switch result.status {
		case replayOK:
			okCount++
			if *verbose {
				fmt.Printf("  ✓ id %d OK   (kind=%s)\n", entry.ID, entry.Kind)
			}
		case replayMissing:
			missingCount++
			fmt.Printf("  ✗ id %d MISSING_IN_DB   (kind=%s, expected row_hash=%s)\n",
				entry.ID, entry.Kind, hex.EncodeToString(result.expected))
		case replayMismatch:
			mismatchCount++
			fmt.Printf("  ✗ id %d HASH_MISMATCH   (kind=%s)\n", entry.ID, entry.Kind)
			fmt.Printf("      journal row_hash: %s\n", hex.EncodeToString(entry.RowHash))
			fmt.Printf("      db      row_hash: %s\n", hex.EncodeToString(result.dbRowHash))
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		return exitErr
	}

	fmt.Printf("\nsummary: %d journal entries, %d OK, %d missing, %d mismatch\n",
		total, okCount, missingCount, mismatchCount)
	if missingCount+mismatchCount > 0 {
		return exitCheckFail
	}
	return exitOK
}

// journalEntry is the parsed form of one zap-JSON line that represents
// an audit event. Numeric / nullable fields match the AuditEvent model.
type journalEntry struct {
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
	OutputSHA256  string
	PrevHash      []byte
	RowHash       []byte
}

// journalJSON is the on-the-wire zap JSON shape. `json.Unmarshal`
// handles duplicate "ts" keys last-wins, which correctly picks the
// audit row's ts (it's emitted after zap's auto-timestamp).
type journalJSON struct {
	Msg           string     `json:"msg"`
	ID            int64      `json:"id"`
	TS            *time.Time `json:"ts"`
	UserID        *int64     `json:"user_id"`
	Kind          string     `json:"kind"`
	Cmd           string     `json:"cmd"`
	ExitCode      *int       `json:"exit_code"`
	BytesOut      *int       `json:"bytes_out"`
	DurationMS    *int64     `json:"duration_ms"`
	Termination   string     `json:"termination"`
	DangerPattern string     `json:"danger_pattern"`
	Detail        string     `json:"detail"`
	OutputSHA256  string     `json:"output_sha256"`
	PrevHash      string     `json:"prev_hash"`
	RowHash       string     `json:"row_hash"`
}

// parseJournalLine returns (entry, true) when the line is a valid
// JSON object with `"msg":"audit"`. All other inputs (non-JSON, other
// msgs, missing fields) return (_, false) — the scanner skips them.
func parseJournalLine(line []byte) (*journalEntry, bool) {
	var j journalJSON
	if err := json.Unmarshal(line, &j); err != nil {
		return nil, false
	}
	if j.Msg != "audit" {
		return nil, false
	}
	if j.TS == nil {
		return nil, false
	}
	prev, err := hex.DecodeString(j.PrevHash)
	if err != nil {
		return nil, false
	}
	row, err := hex.DecodeString(j.RowHash)
	if err != nil {
		return nil, false
	}
	return &journalEntry{
		ID:            j.ID,
		TS:            *j.TS,
		UserID:        j.UserID,
		Kind:          j.Kind,
		Cmd:           j.Cmd,
		ExitCode:      j.ExitCode,
		BytesOut:      j.BytesOut,
		DurationMS:    j.DurationMS,
		Termination:   j.Termination,
		DangerPattern: j.DangerPattern,
		Detail:        j.Detail,
		OutputSHA256:  j.OutputSHA256,
		PrevHash:      prev,
		RowHash:       row,
	}, true
}

// replayStatus is the per-entry outcome.
type replayStatus int

const (
	replayOK replayStatus = iota
	replayMissing
	replayMismatch
)

type replayResult struct {
	status    replayStatus
	expected  []byte // expected row_hash when comparing the journal against itself
	dbRowHash []byte // stored row_hash in DB (only set on mismatch)
}

// compareEntry looks up the journal entry's id in the DB and reports
// OK / MISSING / MISMATCH. The comparison is between the journal's
// stored row_hash and the DB's stored row_hash, so any DB-side tamper
// shows up directly; the journal itself is treated as the
// known-good baseline.
func compareEntry(gormDB *gorm.DB, e *journalEntry) (*replayResult, error) {
	var row dbm.AuditEvent
	err := gormDB.First(&row, e.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Journal has it, DB doesn't — missing.
		expected := repo.ComputeExpectedRowHash(e.PrevHash, journalRowToRepo(e))
		return &replayResult{status: replayMissing, expected: expected}, nil
	}
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(row.RowHash, e.RowHash) {
		return &replayResult{status: replayMismatch, dbRowHash: row.RowHash}, nil
	}
	return &replayResult{status: replayOK}, nil
}

// journalRowToRepo projects a journalEntry into the repo.JournalRow
// shape ComputeExpectedRowHash wants.
func journalRowToRepo(e *journalEntry) repo.JournalRow {
	return repo.JournalRow{
		TS:            e.TS,
		UserID:        e.UserID,
		Kind:          e.Kind,
		Cmd:           e.Cmd,
		ExitCode:      e.ExitCode,
		BytesOut:      e.BytesOut,
		DurationMS:    e.DurationMS,
		Termination:   e.Termination,
		DangerPattern: e.DangerPattern,
		Detail:        e.Detail,
		OutputSHA256:  e.OutputSHA256,
	}
}

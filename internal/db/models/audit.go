package models

import "time"

// Audit event kinds.
const (
	KindCommandRun      = "command_run"
	KindDangerRequested = "danger_requested"
	KindDangerConfirmed = "danger_confirmed"
	KindDangerExpired   = "danger_expired"
	KindFileDownload    = "file_download"
	KindFileUpload      = "file_upload"
	KindAuthReject      = "auth_reject"
	KindShellSpawn      = "shell_spawn"
	KindShellReset      = "shell_reset"
	KindShellReaped     = "shell_reaped"
	KindUserAdded       = "user_added"
	KindUserRemoved     = "user_removed"
	KindUserBanned      = "user_banned"
	KindRoleChanged     = "role_changed"
	KindStartup         = "startup"
	KindShutdown        = "shutdown"
)

// AuditEvent is the GORM model for the `audit_events` table.
//
// Tamper-evidence: each row carries a `prev_hash` (the row_hash of the
// previous row, or the genesis seed for the first row) and a `row_hash`
// computed as `sha256(prev_hash || canonical(row))`. See AuditRepo.Log
// and AuditRepo.Verify for the canonical form and chain walk.
type AuditEvent struct {
	ID            int64     `gorm:"primaryKey;autoIncrement;column:id"`
	TS            time.Time `gorm:"not null;column:ts;index"`
	UserID        *int64    `gorm:"column:user_id;index"`
	Kind          string    `gorm:"not null;column:kind;index"`
	Cmd           string    `gorm:"column:cmd"`
	ExitCode      *int      `gorm:"column:exit_code"`
	BytesOut      *int      `gorm:"column:bytes_out"`
	DurationMS    *int64    `gorm:"column:duration_ms"`
	Termination   string    `gorm:"column:termination"`
	DangerPattern string    `gorm:"column:danger_pattern"`
	Detail        string    `gorm:"column:detail"`
	PrevHash      []byte    `gorm:"column:prev_hash"`
	RowHash       []byte    `gorm:"column:row_hash;index"`
	// OutputSHA256 is the hex sha256 of the uncompressed output blob
	// (post-redact, post-strip) captured at insert time. Stored so
	// Verify can validate the chain without decompressing
	// audit_outputs blobs, dropping it from O(N) decompress
	// round-trips to a single Find. Empty on legacy rows (pre-column)
	// and on rows with no output; Verify falls back to the legacy
	// decompress path in that case.
	OutputSHA256 string `gorm:"column:output_sha256"`

	Output *AuditOutput `gorm:"foreignKey:AuditID;constraint:OnDelete:CASCADE"`
}

func (AuditEvent) TableName() string { return "audit_events" }

// AuditOutput stores the compressed command-output blob.
type AuditOutput struct {
	AuditID  int64  `gorm:"primaryKey;column:audit_id"`
	GzBody   []byte `gorm:"not null;column:gz_body"`
	OrigSize int    `gorm:"not null;column:orig_size"`
}

func (AuditOutput) TableName() string { return "audit_outputs" }

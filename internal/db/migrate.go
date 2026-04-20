package db

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/amiwrpremium/shellboto/internal/db/models"
)

// Migrate runs GORM's AutoMigrate across every model, plus a small set
// of explicit one-off drops for columns that were renamed in earlier
// schema changes. AutoMigrate itself is additive only (never drops);
// the drops here are idempotent on fresh DBs.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.User{},
		&models.AuditEvent{},
		&models.AuditOutput{},
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}
	if err := dropLegacyColumns(db); err != nil {
		return fmt.Errorf("drop legacy columns: %w", err)
	}
	return nil
}

// dropLegacyColumns removes columns that were renamed in earlier
// migrations and are no longer referenced by any model. Each drop is
// guarded by HasColumn so fresh DBs (which never had the legacy name)
// are a no-op.
//
// NOTE: `db.Migrator().DropColumn` silently no-ops when the struct
// no longer has the corresponding field (GORM's LookUpField returns
// nil → early return). We bypass with raw SQL here because by
// definition these columns are dropped AFTER the struct removal.
// SQLite 3.35+ supports native `ALTER TABLE … DROP COLUMN`; our
// bundled driver is well past that.
func dropLegacyColumns(db *gorm.DB) error {
	// `first_name` was renamed to `name`. Dev DBs migrated
	// across the rename still carry the orphan column. Drop it — any
	// data there is stale (Touch() stopped writing to it when the
	// rename landed) and all reads now go through `name`.
	if db.Migrator().HasColumn(&models.User{}, "first_name") {
		if err := db.Exec("ALTER TABLE users DROP COLUMN first_name").Error; err != nil {
			return fmt.Errorf("users.first_name: %w", err)
		}
	}
	return nil
}

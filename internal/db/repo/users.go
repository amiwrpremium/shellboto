package repo

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/amiwrpremium/shellboto/internal/db/models"
)

var ErrNotFound = errors.New("not found")

// ErrTargetIsSuperadmin is returned by Add when the target row already
// exists with role=superadmin. Callers treat this as a hard violation:
// an admin who attempts /adduser against the super is trying
// to demote them via the ON CONFLICT upsert path. The callback layer
// turns this into a ban.
var ErrTargetIsSuperadmin = errors.New("target is superadmin")

// UserRepo holds user-table queries.
type UserRepo struct{ db *gorm.DB }

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

// Lookup returns the row for telegramID (active or disabled).
func (u *UserRepo) Lookup(telegramID int64) (*models.User, error) {
	var user models.User
	if err := u.db.First(&user, telegramID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

// LookupActive returns the row only if not disabled.
func (u *UserRepo) LookupActive(telegramID int64) (*models.User, error) {
	usr, err := u.Lookup(telegramID)
	if err != nil {
		return nil, err
	}
	if !usr.IsActive() {
		return nil, ErrNotFound
	}
	return usr, nil
}

// List returns active users ordered by role then id.
func (u *UserRepo) List() ([]*models.User, error) {
	var users []*models.User
	err := u.db.Where("disabled_at IS NULL").
		Order(`CASE role WHEN 'superadmin' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, telegram_id`).
		Find(&users).Error
	return users, err
}

// ListAll returns active + disabled users. Active rows come first.
func (u *UserRepo) ListAll() ([]*models.User, error) {
	var users []*models.User
	err := u.db.Order("disabled_at IS NULL DESC, name COLLATE NOCASE, telegram_id").
		Find(&users).Error
	return users, err
}

// SeedSuperadmin ensures exactly one superadmin matching the env-provided ID.
func (u *UserRepo) SeedSuperadmin(telegramID int64) error {
	return u.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).
			Where("role = ? AND telegram_id != ?", models.RoleSuperadmin, telegramID).
			Update("role", models.RoleAdmin).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "telegram_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"role":        models.RoleSuperadmin,
				"disabled_at": nil,
			}),
		}).Create(&models.User{
			TelegramID: telegramID,
			Role:       models.RoleSuperadmin,
			AddedAt:    now,
		}).Error
	})
}

// Add inserts a new user or re-enables an existing disabled one with
// role=user. Rejects any other role — a future caller that wanted to
// promote-at-creation must explicitly compose Add(…RoleUser…) +
// Promote(…) so the role change is a separate, reviewed step.
//
// Refuses when the target row is currently role=superadmin — without
// this guard, the ON CONFLICT UPDATE would overwrite the super's row
// with role=user. Super's role is managed exclusively by
// SeedSuperadmin via the SHELLBOTO_SUPERADMIN_ID env var.
func (u *UserRepo) Add(telegramID int64, role, name string, addedBy int64) error {
	if role != models.RoleUser {
		return fmt.Errorf("Add only creates role=user rows; got %q", role)
	}
	if existing, err := u.Lookup(telegramID); err == nil && existing.Role == models.RoleSuperadmin {
		return ErrTargetIsSuperadmin
	}
	updates := map[string]any{
		"role":        role,
		"added_by":    addedBy,
		"disabled_at": nil,
	}
	if name != "" {
		updates["name"] = name
	}
	return u.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "telegram_id"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&models.User{
		TelegramID: telegramID,
		Role:       role,
		Name:       name,
		AddedAt:    time.Now().UTC(),
		AddedBy:    &addedBy,
	}).Error
}

// SoftDelete marks a row disabled; refuses the superadmin.
func (u *UserRepo) SoftDelete(telegramID int64) error {
	now := time.Now().UTC()
	res := u.db.Model(&models.User{}).
		Where("telegram_id = ? AND role != ?", telegramID, models.RoleSuperadmin).
		Update("disabled_at", &now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Reinstate clears disabled_at; refuses the superadmin.
func (u *UserRepo) Reinstate(telegramID int64) error {
	res := u.db.Model(&models.User{}).
		Where("telegram_id = ? AND role != ?", telegramID, models.RoleSuperadmin).
		Update("disabled_at", nil)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Promote sets role=admin + promoted_by=actorID on an active user row.
func (u *UserRepo) Promote(targetID, actorID int64) error {
	res := u.db.Model(&models.User{}).
		Where("telegram_id = ? AND role = ? AND disabled_at IS NULL", targetID, models.RoleUser).
		Updates(map[string]any{"role": models.RoleAdmin, "promoted_by": actorID})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Demote sets role=user + promoted_by=NULL for every given ID. Refuses super.
func (u *UserRepo) Demote(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return u.db.Model(&models.User{}).
		Where("telegram_id IN ? AND role != ?", ids, models.RoleSuperadmin).
		Updates(map[string]any{"role": models.RoleUser, "promoted_by": nil}).Error
}

// CollectAdminSubtree returns rootID + all admin descendants via promoted_by.
func (u *UserRepo) CollectAdminSubtree(rootID int64) ([]int64, error) {
	visited := map[int64]bool{rootID: true}
	result := []int64{rootID}
	frontier := []int64{rootID}
	for len(frontier) > 0 {
		var kids []int64
		if err := u.db.Model(&models.User{}).
			Where("role = ? AND promoted_by IN ?", models.RoleAdmin, frontier).
			Pluck("telegram_id", &kids).Error; err != nil {
			return nil, err
		}
		frontier = frontier[:0]
		for _, k := range kids {
			if visited[k] {
				continue
			}
			visited[k] = true
			result = append(result, k)
			frontier = append(frontier, k)
		}
	}
	return result, nil
}

// ListByRole returns active users of the given role, optionally filtered
// by promoted_by.
func (u *UserRepo) ListByRole(role string, promotedByFilter *int64) ([]*models.User, error) {
	q := u.db.Where("role = ? AND disabled_at IS NULL", role)
	if promotedByFilter != nil {
		q = q.Where("promoted_by = ?", *promotedByFilter)
	}
	var users []*models.User
	err := q.Order("name COLLATE NOCASE, telegram_id").Find(&users).Error
	return users, err
}

// NotifyChainFor returns the IDs to DM for an event triggered by
// actorID under the "direct-promoter + super" fanout rule. Rules:
//   - actor is super → nil (top of the hierarchy, no one above)
//   - actor is admin → [promoted_by, super], deduped
//   - actor is user → [added_by, super], deduped
//
// Inactive (soft-deleted) ancestor rows are filtered out — we never
// DM banned admins. The super row is always included (as long as
// it exists and is active). Order: direct parent first, super last.
func (u *UserRepo) NotifyChainFor(actorID int64) ([]int64, error) {
	actor, err := u.Lookup(actorID)
	if err != nil {
		return nil, err
	}
	// Super's row: exactly one at any time. Look it up to get its ID +
	// active status. An inactive super is a corrupt-DB edge case; if it
	// happens we'll treat super as absent and DM no one.
	var super models.User
	ferr := u.db.Where("role = ?", models.RoleSuperadmin).First(&super).Error
	haveSuper := ferr == nil && super.IsActive()

	// Super triggers events themselves → no ancestors.
	if actor.IsSuperadmin() {
		return nil, nil
	}

	var directID *int64
	switch actor.Role {
	case models.RoleAdmin:
		directID = actor.PromotedBy
	default: // RoleUser
		directID = actor.AddedBy
	}

	out := make([]int64, 0, 2)
	seen := map[int64]bool{}

	if directID != nil && *directID != 0 {
		if d, err := u.Lookup(*directID); err == nil && d.IsActive() {
			out = append(out, *directID)
			seen[*directID] = true
		}
	}
	if haveSuper && !seen[super.TelegramID] {
		out = append(out, super.TelegramID)
	}
	return out, nil
}

// Touch refreshes the caller's username every message. Fills `name` only
// when currently empty (admin-entered names never get overwritten).
func (u *UserRepo) Touch(telegramID int64, username, nameFallback string) {
	_ = u.db.Model(&models.User{}).
		Where("telegram_id = ?", telegramID).
		Update("username", username).Error
	if nameFallback != "" {
		_ = u.db.Model(&models.User{}).
			Where("telegram_id = ? AND (name IS NULL OR name = '')", telegramID).
			Update("name", nameFallback).Error
	}
}

// Package rbac holds permission predicates used by Telegram handlers to
// decide which buttons to show and to re-verify at click time. Kept
// separate from presentation so the rules are testable in isolation.
package rbac

import "github.com/amiwrpremium/shellboto/internal/db/models"

// CanActOnLifecycle implements the /deluser-style rule used by Remove and
// Reinstate in the /users browser:
//   - superadmin may act on any non-superadmin
//   - admin may act only on role=user
//   - otherwise: no
func CanActOnLifecycle(caller, target *models.User) bool {
	if target.IsSuperadmin() {
		return false
	}
	if caller.IsSuperadmin() {
		return true
	}
	// Caller is admin.
	return target.Role == models.RoleUser
}

// CanPromote: admin+ may promote any active user (role=user, not banned).
func CanPromote(caller, target *models.User) bool {
	if !caller.IsAdminOrAbove() {
		return false
	}
	return target.Role == models.RoleUser && target.IsActive()
}

// CanDemote: superadmin may demote any active admin; an admin may demote
// only admins they themselves promoted.
func CanDemote(caller, target *models.User) bool {
	if target.Role != models.RoleAdmin || !target.IsActive() {
		return false
	}
	if caller.IsSuperadmin() {
		return true
	}
	if !caller.IsAdminOrAbove() {
		return false
	}
	return target.PromotedBy != nil && *target.PromotedBy == caller.TelegramID
}

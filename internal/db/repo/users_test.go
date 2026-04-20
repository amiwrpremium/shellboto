package repo

import (
	"errors"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/amiwrpremium/shellboto/internal/db"
	"github.com/amiwrpremium/shellboto/internal/db/models"
)

// testStore is a lightweight holder that lets tests peek at the raw
// *gorm.DB handle (needed for the hash-chain "tamper" tests and the
// different-seeds test).
type testStore struct {
	DB *gorm.DB
}

func newTestRepo(t *testing.T) (*testStore, *AuditRepo) {
	t.Helper()
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(gormDB) })
	return &testStore{DB: gormDB}, NewAuditRepo(gormDB, nil, nil, OutputAlways, 0)
}

// newTestUsers returns a UserRepo + store for the user-focused tests.
func newTestUsers(t *testing.T) (*UserRepo, *AuditRepo) {
	t.Helper()
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(gormDB) })
	return NewUserRepo(gormDB), NewAuditRepo(gormDB, nil, nil, OutputAlways, 0)
}

// makeIsolatedAuditRepo is used by the seed-difference test.
func makeIsolatedAuditRepo(t *testing.T, seed []byte) (*AuditRepo, *gorm.DB) {
	t.Helper()
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(gormDB) })
	return NewAuditRepo(gormDB, seed, nil, OutputAlways, 0), gormDB
}

func TestSeedSuperadminDemotesOld(t *testing.T) {
	users, _ := newTestUsers(t)

	if err := users.SeedSuperadmin(111); err != nil {
		t.Fatalf("seed: %v", err)
	}
	u, err := users.Lookup(111)
	if err != nil || u.Role != models.RoleSuperadmin {
		t.Fatalf("lookup: %+v %v", u, err)
	}

	if err := users.SeedSuperadmin(222); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	old, _ := users.Lookup(111)
	if old.Role != models.RoleAdmin {
		t.Fatalf("old super role = %q, want admin", old.Role)
	}
	now, _ := users.Lookup(222)
	if now.Role != models.RoleSuperadmin {
		t.Fatalf("new super role = %q", now.Role)
	}
}

func TestAddSoftDeleteReinstate(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)

	if err := users.Add(42, models.RoleUser, "Alice", 1); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := users.SoftDelete(42); err != nil {
		t.Fatalf("softdelete: %v", err)
	}
	if _, err := users.LookupActive(42); err == nil {
		t.Fatalf("LookupActive on banned should return ErrNotFound")
	}
	if err := users.Reinstate(42); err != nil {
		t.Fatalf("reinstate: %v", err)
	}
	u, _ := users.Lookup(42)
	if u.DisabledAt != nil {
		t.Fatalf("disabled_at not cleared: %v", u.DisabledAt)
	}
}

func TestAddRejectsRoleAdmin(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)

	// Attempting to Add directly at RoleAdmin must fail — promote-at-
	// creation is not supported. Callers must compose
	// Add(RoleUser) + Promote() so the role change is a separate step.
	err := users.Add(50, models.RoleAdmin, "ShouldNotWork", 1)
	if err == nil {
		t.Fatalf("Add(RoleAdmin) should have failed")
	}
	if _, err := users.Lookup(50); err == nil {
		t.Fatalf("Add(RoleAdmin) leaked a row into users")
	}
}

func TestAddRejectsUnknownRole(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)
	if err := users.Add(60, "nonsense", "X", 1); err == nil {
		t.Fatalf("Add(invalid role) should have failed")
	}
}

func TestAddRefusesSuper(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)

	// Admin (id 2) attempts to re-add the super → must be rejected with
	// ErrTargetIsSuperadmin and leave the super's row untouched.
	err := users.Add(1, models.RoleUser, "Attempt", 2)
	if !errors.Is(err, ErrTargetIsSuperadmin) {
		t.Fatalf("Add(super) err = %v, want ErrTargetIsSuperadmin", err)
	}
	u, lookupErr := users.Lookup(1)
	if lookupErr != nil {
		t.Fatalf("Lookup super: %v", lookupErr)
	}
	if u.Role != models.RoleSuperadmin {
		t.Fatalf("super demoted: role=%q", u.Role)
	}
	if u.Name == "Attempt" {
		t.Fatalf("super name overwritten via Add: %q", u.Name)
	}
}

func TestReinstateRefusesSuper(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)
	if err := users.Reinstate(1); err == nil {
		t.Fatalf("expected ErrNotFound on super reinstate")
	}
}

func TestPromoteDemoteCascade(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)
	_ = users.Add(10, models.RoleUser, "A", 1)
	_ = users.Add(20, models.RoleUser, "B", 1)
	_ = users.Add(30, models.RoleUser, "C", 1)
	_ = users.Add(40, models.RoleUser, "D", 1)

	// super → 10 (admin). 10 promotes 20. 20 promotes 30. Super promotes 40.
	_ = users.Promote(10, 1)
	_ = users.Promote(20, 10)
	_ = users.Promote(30, 20)
	_ = users.Promote(40, 1)

	subtree, err := users.CollectAdminSubtree(10)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	got := map[int64]bool{}
	for _, id := range subtree {
		got[id] = true
	}
	for _, want := range []int64{10, 20, 30} {
		if !got[want] {
			t.Fatalf("subtree missing %d: %v", want, subtree)
		}
	}
	if got[40] {
		t.Fatalf("unrelated admin 40 in subtree")
	}

	if err := users.Demote(subtree); err != nil {
		t.Fatalf("demote: %v", err)
	}
	for _, id := range []int64{10, 20, 30} {
		x, _ := users.Lookup(id)
		if x.Role != models.RoleUser || x.PromotedBy != nil {
			t.Fatalf("id %d after cascade: role=%q pb=%v", id, x.Role, x.PromotedBy)
		}
	}
	d, _ := users.Lookup(40)
	if d.Role != models.RoleAdmin {
		t.Fatalf("unrelated admin demoted: %q", d.Role)
	}
}

func TestListByRoleFilter(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)
	_ = users.Add(10, models.RoleUser, "A", 1)
	_ = users.Add(20, models.RoleUser, "B", 1)
	_ = users.Promote(10, 1)
	_ = users.Promote(20, 10)

	id10 := int64(10)
	byAdmin, _ := users.ListByRole(models.RoleAdmin, &id10)
	seen := map[int64]bool{}
	for _, a := range byAdmin {
		seen[a.TelegramID] = true
	}
	if !seen[20] || seen[10] {
		t.Fatalf("admin 10's view wrong: %v", byAdmin)
	}

	bySuper, _ := users.ListByRole(models.RoleAdmin, nil)
	seen = map[int64]bool{}
	for _, a := range bySuper {
		seen[a.TelegramID] = true
	}
	if !seen[10] || !seen[20] {
		t.Fatalf("super's view should include 10, 20: %v", bySuper)
	}
}

func TestListAllShowsBoth(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)
	_ = users.Add(10, models.RoleUser, "A", 1)
	_ = users.Add(20, models.RoleUser, "B", 1)
	_ = users.SoftDelete(20)

	all, err := users.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	activeCount, disabledCount := 0, 0
	for _, x := range all {
		if x.IsActive() {
			activeCount++
		} else {
			disabledCount++
		}
	}
	if activeCount != 2 || disabledCount != 1 {
		t.Fatalf("expected 2/1, got %d/%d", activeCount, disabledCount)
	}
}

func TestNotifyChainFor_AdminChain(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)

	// Super promotes A, A promotes B (both admins).
	_ = users.Add(10, models.RoleUser, "A", 1)
	_ = users.Promote(10, 1) // A.promoted_by = 1 (super)
	_ = users.Add(20, models.RoleUser, "B", 10)
	_ = users.Promote(20, 10) // B.promoted_by = 10 (A)

	// A triggers an event → chain is [super] (A's direct promoter is super).
	chain, err := users.NotifyChainFor(10)
	if err != nil {
		t.Fatalf("NotifyChainFor(A): %v", err)
	}
	if len(chain) != 1 || chain[0] != 1 {
		t.Fatalf("A chain = %v, want [1]", chain)
	}

	// B triggers an event → chain is [A, super].
	chain, err = users.NotifyChainFor(20)
	if err != nil {
		t.Fatalf("NotifyChainFor(B): %v", err)
	}
	if len(chain) != 2 || chain[0] != 10 || chain[1] != 1 {
		t.Fatalf("B chain = %v, want [10, 1]", chain)
	}
}

func TestNotifyChainFor_UserUsesAddedBy(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)
	_ = users.Add(10, models.RoleUser, "A", 1)
	_ = users.Promote(10, 1)
	_ = users.Add(99, models.RoleUser, "Victim", 10) // A added user 99

	chain, err := users.NotifyChainFor(99)
	if err != nil {
		t.Fatalf("NotifyChainFor(user): %v", err)
	}
	if len(chain) != 2 || chain[0] != 10 || chain[1] != 1 {
		t.Fatalf("user chain = %v, want [10, 1]", chain)
	}
}

func TestNotifyChainFor_SuperReturnsEmpty(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)

	chain, err := users.NotifyChainFor(1)
	if err != nil {
		t.Fatalf("NotifyChainFor(super): %v", err)
	}
	if len(chain) != 0 {
		t.Fatalf("super chain = %v, want empty", chain)
	}
}

func TestNotifyChainFor_SkipsBannedAncestor(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)
	_ = users.Add(10, models.RoleUser, "A", 1)
	_ = users.Promote(10, 1)
	_ = users.Add(20, models.RoleUser, "B", 10)
	_ = users.Promote(20, 10) // B.promoted_by = A
	_ = users.SoftDelete(10)  // A is banned

	chain, err := users.NotifyChainFor(20)
	if err != nil {
		t.Fatalf("NotifyChainFor(B): %v", err)
	}
	if len(chain) != 1 || chain[0] != 1 {
		t.Fatalf("banned-ancestor-filtered chain = %v, want [1]", chain)
	}
}

func TestTouchNameOnlyIfEmpty(t *testing.T) {
	users, _ := newTestUsers(t)
	_ = users.SeedSuperadmin(1)
	_ = users.Add(42, models.RoleUser, "Alice", 1)

	users.Touch(42, "@alice", "Different")
	u, _ := users.Lookup(42)
	if u.Name != "Alice" {
		t.Fatalf("admin-entered name overwritten: %q", u.Name)
	}
	if u.Username != "@alice" {
		t.Fatalf("username not updated: %q", u.Username)
	}

	// Super has empty name; Touch should fill it once, then never overwrite.
	users.Touch(1, "@super", "Super User")
	sp, _ := users.Lookup(1)
	if sp.Name != "Super User" {
		t.Fatalf("super name fallback failed: %q", sp.Name)
	}
	users.Touch(1, "@super2", "Should Not Replace")
	sp, _ = users.Lookup(1)
	if sp.Name != "Super User" {
		t.Fatalf("super name overwritten on second touch: %q", sp.Name)
	}
}

package rbac

import (
	"testing"
	"time"

	"github.com/amiwrpremium/shellboto/internal/db/models"
)

func TestCanActOnLifecycle(t *testing.T) {
	super := &models.User{TelegramID: 1, Role: models.RoleSuperadmin}
	admin := &models.User{TelegramID: 2, Role: models.RoleAdmin}
	user := &models.User{TelegramID: 3, Role: models.RoleUser}
	otherAdmin := &models.User{TelegramID: 4, Role: models.RoleAdmin}

	cases := []struct {
		caller, target *models.User
		want           bool
		name           string
	}{
		{super, user, true, "super→user"},
		{super, admin, true, "super→admin"},
		{super, super, false, "super→super"},
		{admin, user, true, "admin→user"},
		{admin, otherAdmin, false, "admin→admin"},
		{admin, super, false, "admin→super"},
	}
	for _, c := range cases {
		if got := CanActOnLifecycle(c.caller, c.target); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCanPromote(t *testing.T) {
	admin := &models.User{TelegramID: 2, Role: models.RoleAdmin}
	user := &models.User{TelegramID: 3, Role: models.RoleUser}
	plainUser := &models.User{TelegramID: 4, Role: models.RoleUser}
	now := time.Now()
	bannedUser := &models.User{TelegramID: 5, Role: models.RoleUser, DisabledAt: &now}
	otherAdmin := &models.User{TelegramID: 6, Role: models.RoleAdmin}

	if !CanPromote(admin, plainUser) {
		t.Errorf("admin should promote active user")
	}
	if CanPromote(admin, bannedUser) {
		t.Errorf("should NOT promote banned user")
	}
	if CanPromote(admin, otherAdmin) {
		t.Errorf("should NOT promote an admin")
	}
	if CanPromote(user, plainUser) {
		t.Errorf("plain user cannot promote")
	}
}

func TestCanDemote(t *testing.T) {
	super := &models.User{TelegramID: 1, Role: models.RoleSuperadmin}
	admin := &models.User{TelegramID: 2, Role: models.RoleAdmin}
	user := &models.User{TelegramID: 3, Role: models.RoleUser}

	promoter := int64(2)
	adminByAdmin := &models.User{TelegramID: 10, Role: models.RoleAdmin, PromotedBy: &promoter}
	otherPromoter := int64(99)
	adminByOther := &models.User{TelegramID: 11, Role: models.RoleAdmin, PromotedBy: &otherPromoter}

	if !CanDemote(super, adminByAdmin) {
		t.Errorf("super should demote any admin")
	}
	if !CanDemote(admin, adminByAdmin) {
		t.Errorf("admin should demote their own promotee")
	}
	if CanDemote(admin, adminByOther) {
		t.Errorf("admin must NOT demote another admin's promotee")
	}
	if CanDemote(user, adminByAdmin) {
		t.Errorf("plain user cannot demote")
	}
}

package models

import "time"

// Role constants.
const (
	RoleSuperadmin = "superadmin"
	RoleAdmin      = "admin"
	RoleUser       = "user"
)

// User is the GORM model for the `users` table.
type User struct {
	TelegramID int64      `gorm:"primaryKey;column:telegram_id"`
	Username   string     `gorm:"column:username"`
	Name       string     `gorm:"column:name"`
	Role       string     `gorm:"not null;column:role;index:ix_users_role"`
	AddedAt    time.Time  `gorm:"not null;column:added_at"`
	AddedBy    *int64     `gorm:"column:added_by"`
	DisabledAt *time.Time `gorm:"column:disabled_at"`
	PromotedBy *int64     `gorm:"column:promoted_by"`
}

func (User) TableName() string { return "users" }

func (u *User) IsActive() bool       { return u.DisabledAt == nil }
func (u *User) IsSuperadmin() bool   { return u.Role == RoleSuperadmin }
func (u *User) IsAdminOrAbove() bool { return u.Role == RoleAdmin || u.Role == RoleSuperadmin }

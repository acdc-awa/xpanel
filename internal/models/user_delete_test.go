package models

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUserSoftDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	u := User{Username: "soft@t.com", Email: "soft@t.com", UUID: "11111111-1111-1111-1111-111111111111", SubscribeToken: "tok-soft", Role: RoleUser, Status: StatusActive}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Delete(&u).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}
	var got User
	if err := db.First(&got, u.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("soft-deleted user should not be found, err=%v", err)
	}
	var raw User
	if err := db.Unscoped().First(&raw, u.ID).Error; err != nil {
		t.Fatalf("unscoped find: %v", err)
	}
	if !raw.DeletedAt.Valid {
		t.Fatal("DeletedAt should be set after soft delete")
	}
}
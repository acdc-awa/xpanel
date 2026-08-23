package db

import (
	"errors"
	"testing"
)

func TestIsUniqueViolation(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		column string
		want   bool
	}{
		{"nil", nil, "", false},
		{"sqlite 命中列", errors.New("UNIQUE constraint failed: users.username"), "users.username", true},
		{"sqlite 不限列", errors.New("UNIQUE constraint failed: users.username"), "", true},
		{"sqlite 非该列", errors.New("UNIQUE constraint failed: users.uuid"), "users.username", false},
		{"mysql 命中", errors.New("Error 1062 (23000): Duplicate entry 'a@b.c' for key 'users.username'"), "users.username", true},
		{"普通错误", errors.New("connection refused"), "", false},
		{"约束但列不匹配", errors.New("Duplicate entry 'x' for key 'plans.name'"), "users.username", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUniqueViolation(tc.err, tc.column); got != tc.want {
				t.Errorf("IsUniqueViolation(%v, %q) = %v, want %v", tc.err, tc.column, got, tc.want)
			}
		})
	}
}

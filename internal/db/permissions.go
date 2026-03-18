package db

import (
	"fmt"
	"strings"
)

// Permission is a bitmask of granted permissions for an author.
type Permission int64

const (
	PermRead      Permission = 1 << 0
	PermComment   Permission = 1 << 1
	PermVote      Permission = 1 << 2
	PermCLCreate  Permission = 1 << 3
	PermCLUpdate  Permission = 1 << 4
	PermCLSubmit  Permission = 1 << 5
	PermCITrigger Permission = 1 << 6
	PermAdmin     Permission = -1 << 63 // 1<<63 as signed int64 (the sign bit)

	// PermAll is a convenience mask for root authors (all non-admin bits).
	PermAll = PermRead | PermComment | PermVote | PermCLCreate | PermCLUpdate | PermCLSubmit | PermCITrigger
)

// permNames maps each permission bit to its human-readable name.
var permNames = []struct {
	bit  Permission
	name string
}{
	{PermRead, "read"},
	{PermComment, "comment"},
	{PermVote, "vote"},
	{PermCLCreate, "cl_create"},
	{PermCLUpdate, "cl_update"},
	{PermCLSubmit, "cl_submit"},
	{PermCITrigger, "ci_trigger"},
	{PermAdmin, "admin"},
}

// Has returns true if p contains all bits in flag.
func (p Permission) Has(flag Permission) bool { return p&flag == flag }

// String returns a human-readable comma-separated list of permission names.
func (p Permission) String() string {
	if p == 0 {
		return "none"
	}
	var parts []string
	for _, pn := range permNames {
		if p&pn.bit != 0 {
			parts = append(parts, pn.name)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

// ParsePermissions parses a comma-separated string of permission names
// into a Permission bitmask. Example: "read,comment" -> PermRead|PermComment.
func ParsePermissions(s string) (Permission, error) {
	if s == "" || s == "none" {
		return 0, nil
	}

	// Build reverse lookup map.
	nameTobit := make(map[string]Permission, len(permNames))
	for _, pn := range permNames {
		nameTobit[pn.name] = pn.bit
	}

	var p Permission
	for _, part := range strings.Split(s, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		bit, ok := nameTobit[name]
		if !ok {
			return 0, fmt.Errorf("unknown permission: %q", name)
		}
		p |= bit
	}
	return p, nil
}

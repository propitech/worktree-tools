// Package env reads a worktree's gitignored .env file — the per-worktree
// contract bin/worktree writes (slot, database suffix, Redis index, ports) —
// using github.com/joho/godotenv for the parsing, with thin helpers for the
// keys the tool cares about.
package env

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Load parses the .env file at path into a key→value map. A missing file yields
// an empty map and no error; only a genuine read/parse error is returned.
func Load(path string) (map[string]string, error) {
	m, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	return m, nil
}

// Value returns the value of key in the .env at path, or "" if the file or key
// is absent (or the file is unparseable).
func Value(path, key string) string {
	m, err := Load(path)
	if err != nil {
		return ""
	}
	return m[key]
}

// Slot returns the WORKTREE_SLOT recorded in the .env at path. ok is false when
// the key is absent or non-numeric.
func Slot(path string) (slot int, ok bool) {
	v := Value(path, "WORKTREE_SLOT")
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

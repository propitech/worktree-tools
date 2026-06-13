// Package env reads a worktree's gitignored .env file — the per-worktree
// contract bin/worktree writes (slot, database suffix, Redis index, ports).
// It mirrors the shell env_val / slot_of_env helpers: blank lines and #
// comments are skipped, the last assignment of a key wins, and surrounding
// whitespace and quotes are stripped from the value.
package env

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// trimCut is the set of leading/trailing characters stripped from a value,
// matching the shell sed `s/^[[:space:]"']*//; s/[[:space:]"']*$//`.
const trimCut = " \t\"'"

// Load parses the .env file at path into a key→value map (last assignment
// wins). A missing file yields an empty map and no error; only a genuine read
// error is returned.
func Load(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if k = strings.TrimSpace(k); k == "" {
			continue
		}
		out[k] = strings.Trim(strings.TrimSpace(v), trimCut)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Value returns the value of key in the .env at path, or "" if the file or key
// is absent (or unreadable).
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

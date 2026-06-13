// Package slot allocates per-worktree slot numbers. The primary checkout is
// always slot 0; worktrees created by `worktree add` take the lowest free
// positive slot (1, 2, 3, …). The slot is the single source of truth that
// drives a worktree's namespace contract (database suffix, Redis DB index,
// web port).
package slot

import "sort"

// NextFree returns the lowest positive slot not present in used. Slot 0 — the
// primary checkout — is reserved and never returned, even if absent from used.
func NextFree(used []int) int {
	seen := make(map[int]bool, len(used))
	for _, n := range used {
		seen[n] = true
	}
	for n := 1; ; n++ {
		if !seen[n] {
			return n
		}
	}
}

// Used returns the sorted, de-duplicated set of slots in use, always including
// the primary (0) and ignoring negatives.
func Used(slots []int) []int {
	seen := map[int]bool{0: true}
	for _, n := range slots {
		if n >= 0 {
			seen[n] = true
		}
	}
	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

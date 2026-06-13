package slot

import (
	"reflect"
	"testing"
)

func TestNextFree(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		used []int
		want int
	}{
		{"empty", nil, 1},
		{"primary only", []int{0}, 1},
		{"gap is reused", []int{0, 1, 3}, 2},
		{"contiguous appends", []int{0, 1, 2}, 3},
		{"unsorted with dupes", []int{2, 0, 2, 1}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NextFree(tc.used); got != tc.want {
				t.Errorf("NextFree(%v) = %d, want %d", tc.used, got, tc.want)
			}
		})
	}
}

func TestUsed(t *testing.T) {
	t.Parallel()
	got := Used([]int{3, 1, 1, -2})
	want := []int{0, 1, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Used = %v, want %v", got, want)
	}
}

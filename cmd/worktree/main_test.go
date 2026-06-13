package main

import (
	"io"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no args", nil, 2},
		{"version", []string{"--version"}, 0},
		{"help", []string{"--help"}, 0},
		{"recognised but unimplemented", []string{"add"}, 70},
		{"unknown subcommand", []string{"bogus"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := run(tc.args, io.Discard, io.Discard); got != tc.want {
				t.Errorf("run(%v) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

func TestListReturnsZero(t *testing.T) {
	t.Parallel()
	// list is implemented; running it from inside the repo should succeed (exit 0).
	// Status probes (pg_isready, psql) will fail gracefully and show legacy:down.
	if got := run([]string{"list"}, io.Discard, io.Discard); got != 0 {
		t.Errorf("run([list]) = %d, want 0", got)
	}
}

func TestVersionOutput(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	if code := run([]string{"--version"}, &out, io.Discard); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Errorf("version output = %q, want %q", strings.TrimSpace(out.String()), version)
	}
}

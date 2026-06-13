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
		{"recognised but unimplemented", []string{"list"}, 70},
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

package main

import (
	"io"
	"strings"
	"testing"
)

func TestServicesNoAction(t *testing.T) {
	t.Parallel()
	if got := run([]string{"services"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([services]) with no action = %d, want 2", got)
	}
}

func TestServicesUnknownAction(t *testing.T) {
	t.Parallel()
	if got := run([]string{"services", "bogus"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([services bogus]) = %d, want 2", got)
	}
}

func TestServicesStartNotYetPorted(t *testing.T) {
	t.Parallel()
	// start/stop land in a later PR; until then they report unimplemented.
	for _, action := range []string{"start", "stop"} {
		if got := run([]string{"services", action}, io.Discard, io.Discard); got != 70 {
			t.Errorf("run([services %s]) = %d, want 70", action, got)
		}
	}
}

func TestServicesStatusReportsHealth(t *testing.T) {
	t.Parallel()
	// status is read-only and always exits 0; probes fail gracefully to "down"
	// when no shared daemons are running (e.g. in CI).
	var out strings.Builder
	if got := run([]string{"services", "status"}, &out, io.Discard); got != 0 {
		t.Fatalf("run([services status]) = %d, want 0", got)
	}
	for _, want := range []string{"Shared dev services", "postgres", "redis", "mailpit"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status output missing %q; got:\n%s", want, out.String())
		}
	}
}

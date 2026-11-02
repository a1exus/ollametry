package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()
	module := func(v string) *debug.BuildInfo { return &debug.BuildInfo{Main: debug.Module{Version: v}} }
	tests := []struct {
		name, stamped string
		info          *debug.BuildInfo
		ok            bool
		want          string
	}{
		{"stamped wins", "v1.2.3", module("v9.9.9"), true, "v1.2.3"},
		{"go install module version", "dev", module("v1.2.3"), true, "v1.2.3"},
		{"local build stays dev", "dev", module("(devel)"), true, "dev"},
		{"empty module version", "dev", module(""), true, "dev"},
		{"no build info", "dev", nil, false, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveVersion(tt.stamped, tt.info, tt.ok); got != tt.want {
				t.Errorf("resolveVersion(%q, %v) = %q, want %q", tt.stamped, tt.info, got, tt.want)
			}
		})
	}
}

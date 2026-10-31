// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"runtime/debug"
	"testing"
)

func TestVersionFrom(t *testing.T) {
	mod := func(v string) *debug.BuildInfo { return &debug.BuildInfo{Main: debug.Module{Version: v}} }
	tests := []struct {
		injected string
		info     *debug.BuildInfo
		ok       bool
		want     string
	}{
		{"v1.6.1", mod("v1.6.0"), true, "v1.6.1"},
		{"", mod("v1.6.1"), true, "v1.6.1"},
		{"", mod("(devel)"), true, "dev"},
		{"", mod(""), true, "dev"},
		{"", nil, false, "dev"},
	}
	for _, tt := range tests {
		if got := versionFrom(tt.injected, tt.info, tt.ok); got != tt.want {
			t.Errorf("versionFrom(%q, %v, %v) = %q, want %q", tt.injected, tt.info, tt.ok, got, tt.want)
		}
	}
}

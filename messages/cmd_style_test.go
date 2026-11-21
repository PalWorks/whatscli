// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"slices"
	"strings"
	"testing"
)

func TestCmdStyle(t *testing.T) {
	sm, ui := newTestSessionManager(t)
	if _, ok := commandRegistry["style"]; !ok {
		t.Fatal("style is not registered")
	}
	cmdStyle(sm, nil, "style", nil)
	cmdStyle(sm, nil, "style", []string{"Classic"})
	cmdStyle(sm, nil, "style", []string{"bubbles"})
	if want := []string{"", "classic", "bubbles"}; !slices.Equal(ui.Styles, want) {
		t.Fatalf("styles = %q, want %q", ui.Styles, want)
	}
	for _, bad := range [][]string{{"fancy"}, {"classic", "x"}} {
		cmdStyle(sm, nil, "style", bad)
		if len(ui.Styles) != 3 || !strings.Contains(ui.Texts[len(ui.Texts)-1], "Usage:") {
			t.Errorf("/style %q: want a usage message and no change, got %q", bad, ui.Texts)
		}
	}
}

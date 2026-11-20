// SPDX-License-Identifier: AGPL-3.0-only

package messages

import "testing"

// /exit is an alias of /quit: both are registered and both quit the app.
func TestQuitAndExitQuit(t *testing.T) {
	for _, name := range []string{"quit", "exit"} {
		handler, ok := commandRegistry[name]
		if !ok {
			t.Fatalf("command %q is not registered", name)
		}
		sm, ui := newTestSessionManager(t)
		handler(sm, nil, name, nil)
		if ui.QuitCalled != 1 {
			t.Errorf("/%s: Quit called %d times, want 1", name, ui.QuitCalled)
		}
	}
}

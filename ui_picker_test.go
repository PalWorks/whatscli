// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"slices"
	"testing"

	"github.com/PalWorks/whatscli/messages"
	"github.com/PalWorks/whatscli/picker"
)

func testExtensions() picker.Extensions {
	return picker.Extensions{
		Image: picker.ParseExtensions("jpg,jpeg,png,webp"),
		Video: picker.ParseExtensions("mp4,3gp,mov,mkv,webm"),
		Audio: picker.ParseExtensions("mp3,ogg,opus,m4a,aac,amr,wav"),
	}
}

func TestOpensPicker(t *testing.T) {
	tests := []struct {
		cmd    string
		params []string
		want   bool
	}{
		{"upload", nil, true},
		{"sendimage", nil, true},
		{"sendvideo", []string{}, true},
		{"sendaudio", nil, true},
		{"upload", []string{"/a/x.pdf"}, false},
		{"send", nil, false},
		{"sendfile", nil, false},
	}
	for _, tt := range tests {
		if got := opensPicker(tt.cmd, tt.params); got != tt.want {
			t.Errorf("opensPicker(%q, %q) = %v, want %v", tt.cmd, tt.params, got, tt.want)
		}
	}
}

func TestPickedCommandsFixedCommand(t *testing.T) {
	got := pickedCommands([]string{"/a/x.png", "/a/y.txt"}, "chatA", "sendimage", false, testExtensions())
	want := []messages.Command{
		{Name: "sendfile", Params: []string{"chatA", "sendimage", "/a/x.png"}},
		{Name: "sendfile", Params: []string{"chatA", "sendimage", "/a/y.txt"}},
	}
	if !slices.EqualFunc(got, want, commandEqual) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPickedCommandsAuto(t *testing.T) {
	got := pickedCommands([]string{"/a/x.JPG", "/a/v.mp4", "/a/s.mp3", "/a/d.pdf"}, "chatA", "upload", true, testExtensions())
	cmds := make([]string, len(got))
	for i, c := range got {
		cmds[i] = c.Params[1]
	}
	if want := []string{"sendimage", "sendvideo", "sendaudio", "upload"}; !slices.Equal(cmds, want) {
		t.Fatalf("got %v, want %v", cmds, want)
	}
}

func TestPickedCommandsKeepsSpaces(t *testing.T) {
	got := pickedCommands([]string{"/a/my report.pdf"}, "chatA", "upload", false, testExtensions())
	if len(got) != 1 || !slices.Equal(got[0].Params, []string{"chatA", "upload", "/a/my report.pdf"}) {
		t.Fatalf("got %v", got)
	}
}

func commandEqual(a, b messages.Command) bool {
	return a.Name == b.Name && slices.Equal(a.Params, b.Params)
}

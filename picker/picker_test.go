// SPDX-License-Identifier: AGPL-3.0-only

package picker

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// makeTree builds a sample folder: a few real files, one hidden and one
// gitignored, to check what the walk feeds to fzf.
func makeTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := []string{
		"docs/report.pdf",
		"docs/my report.pdf",
		"pics/IMG_001.JPG",
		"pics/cat.png",
		"music/song.mp3",
		".hidden/secret.png",
		"skip/ignored.png",
	}
	for _, f := range files {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("skip/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

func TestPickFiltersByExtension(t *testing.T) {
	dir := makeTree(t)
	got, err := run(Options{Root: dir, Allow: []string{"jpg", "png"}}, []string{"--filter=."})
	if err != nil {
		t.Fatal(err)
	}
	want := sorted([]string{filepath.Join(dir, "pics/IMG_001.JPG"), filepath.Join(dir, "pics/cat.png")})
	if !slices.Equal(sorted(got), want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPickUnfilteredReturnsAbsolutePaths(t *testing.T) {
	dir := makeTree(t)
	got, err := run(Options{Root: dir}, []string{"--filter=song"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(dir, "music/song.mp3")}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPickPathWithSpaces(t *testing.T) {
	dir := makeTree(t)
	got, err := run(Options{Root: dir}, []string{"--filter=my report"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, filepath.Join(dir, "docs/my report.pdf")) {
		t.Fatalf("got %v, want it to contain the path with a space", got)
	}
}

func TestPickNoMatchReturnsNil(t *testing.T) {
	dir := makeTree(t)
	got, err := run(Options{Root: dir}, []string{"--filter=zzzzqqq"})
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}

func TestPickTwice(t *testing.T) {
	dir := makeTree(t)
	for i := 0; i < 2; i++ {
		got, err := run(Options{Root: dir}, []string{"--filter=pdf"})
		if err != nil || len(got) == 0 {
			t.Fatalf("run %d: got %v, %v", i, got, err)
		}
	}
}

func TestPickMaxFiles(t *testing.T) {
	dir := makeTree(t)
	got, err := run(Options{Root: dir, MaxFiles: 2}, []string{"--filter=."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 2 {
		t.Fatalf("got %d results, want at most 2: %v", len(got), got)
	}
}

func TestPickMissingRoot(t *testing.T) {
	dir := makeTree(t)
	if _, err := run(Options{Root: filepath.Join(dir, "nope")}, []string{"--filter=."}); err == nil {
		t.Fatal("expected an error for a missing root")
	}
}

func TestExpandRoot(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ExpandRoot("~/Pictures"), filepath.Join(home, "Pictures"); got != want {
		t.Errorf("ExpandRoot(~/Pictures) = %q, want %q", got, want)
	}
	if got := ExpandRoot("~"); got != home {
		t.Errorf("ExpandRoot(~) = %q, want %q", got, home)
	}
	if got := ExpandRoot("/abs"); got != "/abs" {
		t.Errorf("ExpandRoot(/abs) = %q", got)
	}
}

// A user's FZF_DEFAULT_OPTS must not add extra output lines (query, expect
// key) that would be sent as files.
func TestSafetyArgsOverrideUserOptions(t *testing.T) {
	t.Setenv("FZF_DEFAULT_OPTS", "--print-query --expect=ctrl-x --select-1")
	dir := makeTree(t)
	got, err := run(Options{Root: dir}, append([]string{"--filter=song"}, safetyArgs...))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(dir, "music/song.mp3")}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPickArgsEndWithSafetyArgs(t *testing.T) {
	args := pickArgs("upload> ")
	if len(args) < len(safetyArgs) || !slices.Equal(args[len(args)-len(safetyArgs):], safetyArgs) {
		t.Fatalf("pickArgs = %v, want it to end with %v", args, safetyArgs)
	}
	for _, want := range []string{"--no-select-1", "--no-print-query", "--no-expect", "--no-winpty"} {
		if !slices.Contains(safetyArgs, want) {
			t.Errorf("safetyArgs missing %s", want)
		}
	}
}

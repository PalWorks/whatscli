// SPDX-License-Identifier: AGPL-3.0-only

package picker

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/boyter/gocodewalker"
	fzf "github.com/junegunn/fzf/src"
)

// DefaultMaxFiles bounds how many files one walk feeds to fzf.
const DefaultMaxFiles = 200000

// Options configures one picker run.
type Options struct {
	Root     string   // folder to search; "~" is expanded
	Allow    []string // lowercase extensions without dot; empty = all files
	Prompt   string   // fzf prompt
	MaxFiles int      // 0 = DefaultMaxFiles
}

// ExpandRoot replaces a leading "~" with the user's home folder.
func ExpandRoot(root string) string {
	if root != "~" && !strings.HasPrefix(root, "~/") && !strings.HasPrefix(root, `~\`) {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return root
	}
	return filepath.Join(home, root[1:])
}

// Pick opens fzf over the files under opts.Root and returns the chosen
// files as absolute paths. It returns nil, nil when the user cancels or
// nothing matched. The caller must hand fzf the terminal (tview's Suspend).
func Pick(opts Options) ([]string, error) {
	return run(opts, pickArgs(opts.Prompt))
}

// safetyArgs are appended last so they override the user's FZF_DEFAULT_OPTS:
// nothing may be sent without the user pressing Enter (--select-1), no extra
// output lines may be mistaken for files (--print-query, --expect), and fzf
// must not re-launch whatscli in a tmux/zellij popup or under winpty.
// Known gap: on Windows mintty >= 3.4.5 without MSYS=enable_pcon, fzf still
// re-launches through winpty; run whatscli from Windows Terminal there.
var safetyArgs = []string{"--no-select-1", "--no-exit-0", "--no-print-query", "--no-expect", "--no-tmux", "--no-winpty"}

func pickArgs(prompt string) []string {
	return append([]string{"--multi", "--scheme=path", "--prompt=" + prompt}, safetyArgs...)
}

// run is Pick with explicit fzf arguments, so tests can use --filter.
func run(opts Options, fzfArgs []string) ([]string, error) {
	root := filepath.Clean(ExpandRoot(opts.Root))
	if info, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("file picker: %w", err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("file picker: %s is not a folder", root)
	}
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFiles
	}

	fzfOpts, err := fzf.ParseOptions(true, fzfArgs)
	if err != nil {
		return nil, fmt.Errorf("file picker: %w", err)
	}
	input := make(chan string, 1024)
	output := make(chan string, 64)
	fzfOpts.Input = input
	fzfOpts.Output = output

	queue := make(chan *gocodewalker.File, 1024)
	walker := gocodewalker.NewFileWalker(root, queue)
	walker.SetErrorHandler(func(error) bool { return true }) // skip unreadable dirs
	go walker.Start()

	done := make(chan struct{})
	go feed(walker, queue, input, done, root, opts.Allow, maxFiles)

	var picked []string
	var collected sync.WaitGroup
	collected.Add(1)
	go func() {
		defer collected.Done()
		for rel := range output {
			picked = append(picked, filepath.Join(root, rel))
		}
	}()

	code, err := fzf.Run(fzfOpts)
	close(done)
	walker.Terminate()
	close(output)
	collected.Wait()

	switch code {
	case fzf.ExitOk:
		return picked, nil
	case fzf.ExitNoMatch, fzf.ExitInterrupt:
		return nil, nil
	}
	if err == nil {
		err = fmt.Errorf("exit code %d", code)
	}
	return nil, fmt.Errorf("file picker: %w", err)
}

// feed passes allowed files from the walker to fzf as root-relative paths.
// It keeps draining the walker queue after fzf is done so the walker can
// finish, and closes input once the walk ends.
func feed(walker *gocodewalker.FileWalker, queue <-chan *gocodewalker.File, input chan<- string, done <-chan struct{}, root string, allow []string, maxFiles int) {
	defer close(input)
	sent := 0
	for f := range queue {
		if sent >= maxFiles {
			continue
		}
		if len(allow) > 0 {
			ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Location), "."))
			if !slices.Contains(allow, ext) {
				continue
			}
		}
		rel, err := filepath.Rel(root, f.Location)
		if err != nil {
			continue
		}
		select {
		case input <- rel:
			sent++
			if sent >= maxFiles {
				walker.Terminate()
			}
		case <-done:
			walker.Terminate()
			drain(queue)
			return
		}
	}
}

func drain(queue <-chan *gocodewalker.File) {
	for range queue {
	}
}

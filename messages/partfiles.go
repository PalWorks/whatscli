// SPDX-License-Identifier: AGPL-3.0-only

package messages

// partfiles.go: temporary download files. downloadMedia writes to a
// ".whatscli-<digits>.part" file next to the target and renames it into place.
// A download cut short by quitting leaves that file behind, so startup removes
// leftovers (ROADMAP row 11).

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// partFilePattern is the os.CreateTemp pattern for download temp files. The
// '*' becomes a decimal number; isPartFileName must keep matching it.
const partFilePattern = ".whatscli-*.part"

const (
	partFilePrefix = ".whatscli-"
	partFileSuffix = ".part"
)

// isPartFileName reports whether name is exactly a download temp file name:
// the prefix, one or more ASCII digits, the suffix. Nothing else matches.
func isPartFileName(name string) bool {
	if len(name) <= len(partFilePrefix)+len(partFileSuffix) ||
		!strings.HasPrefix(name, partFilePrefix) || !strings.HasSuffix(name, partFileSuffix) {
		return false
	}
	mid := name[len(partFilePrefix) : len(name)-len(partFileSuffix)]
	for i := 0; i < len(mid); i++ {
		if mid[i] < '0' || mid[i] > '9' {
			return false
		}
	}
	return true
}

// sweepPartFiles removes leftover download temp files directly inside dir and
// returns how many were removed. Only regular files whose name matches
// isPartFileName are removed; symlinks, directories and other entries are
// left alone and subdirectories are not entered. A blank or missing dir is
// not an error.
func sweepPartFiles(dir string) (int, error) {
	if strings.TrimSpace(dir) == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if !isPartFileName(e.Name()) || !e.Type().IsRegular() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		// Check again without following links: the entry may have changed.
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(p); err == nil {
			removed++
		}
	}
	return removed, nil
}

// sweepDownloadFolders removes leftover temp files from the configured
// download and preview folders (each once, when both are the same folder).
func sweepDownloadFolders(dirs ...string) int {
	seen := make(map[string]bool)
	total := 0
	for _, d := range dirs {
		if strings.TrimSpace(d) == "" {
			continue
		}
		key := filepath.Clean(d)
		if seen[key] {
			continue
		}
		seen[key] = true
		n, _ := sweepPartFiles(d)
		total += n
	}
	return total
}

package messages

// pathcheck.go: directory containment checks for files handed to external
// viewers (REVIEW-3 finding 4). Both the file and the allowed directories are
// symlink-resolved, so symlinked homes (/home -> /var/home) work, and
// containment is decided with filepath.Rel, so sibling folders such as
// Downloads2 do not pass for Downloads.

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// ErrOutsideAllowedDirs is returned when a file is not inside any allowed dir.
var ErrOutsideAllowedDirs = errors.New("file path outside allowed directories")

// ResolveFileWithin resolves file (absolute, symlinks evaluated) and checks it
// lies strictly inside one of dirs, each also resolved. Empty dirs are
// skipped (an unset config value must not mean "the current directory").
// Directories that do not exist yet are resolved through their nearest
// existing ancestor. Returns the resolved file path.
func ResolveFileWithin(file string, dirs ...string) (string, error) {
	absFile, err := filepath.Abs(filepath.Clean(file))
	if err != nil {
		return "", fmt.Errorf("invalid file path: %w", err)
	}
	resolvedFile, err := filepath.EvalSymlinks(absFile)
	if err != nil {
		return "", fmt.Errorf("could not resolve path: %w", err)
	}
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		resolvedDir, err := resolveDir(dir)
		if err != nil {
			continue
		}
		if isWithin(resolvedDir, resolvedFile) {
			return resolvedFile, nil
		}
	}
	return "", ErrOutsideAllowedDirs
}

// isWithin reports whether target is strictly below dir (both already clean
// absolute paths). dir itself does not count.
func isWithin(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil || rel == "." || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveDir returns the absolute, symlink-resolved form of dir. If dir (or
// part of it) does not exist, the nearest existing ancestor is resolved and
// the missing components are appended unchanged.
func resolveDir(dir string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return "", err
	}
	var missing []string
	cur := abs
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			parts := append([]string{resolved}, missing...)
			return filepath.Join(parts...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		missing = append([]string{filepath.Base(cur)}, missing...)
		cur = parent
	}
}

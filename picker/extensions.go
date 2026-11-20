// SPDX-License-Identifier: AGPL-3.0-only

// Package picker lets the user choose files to send with an embedded fzf.
package picker

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/PalWorks/whatscli/config"
)

// Extensions holds the lowercase file extensions (without dot) that count
// as images, videos and audio.
type Extensions struct {
	Image, Video, Audio []string
}

// ParseExtensions turns a comma-separated list like "JPG, .png" into
// normalized extensions: lowercase, trimmed, no leading dot, no empties.
func ParseExtensions(csv string) []string {
	var out []string
	for _, part := range strings.Split(csv, ",") {
		ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(part), "."))
		if ext != "" {
			out = append(out, ext)
		}
	}
	return out
}

// ExtensionsFromConfig reads the extension lists from the [general] config.
func ExtensionsFromConfig() Extensions {
	g := config.Config.General
	return Extensions{
		Image: ParseExtensions(g.ImageExtensions),
		Video: ParseExtensions(g.VideoExtensions),
		Audio: ParseExtensions(g.AudioExtensions),
	}
}

// AllowFor returns the extensions the picker for a send command shows
// (sendimage, sendvideo, sendaudio); nil means every file (upload).
func (e Extensions) AllowFor(cmd string) []string {
	switch cmd {
	case "sendimage":
		return e.Image
	case "sendvideo":
		return e.Video
	case "sendaudio":
		return e.Audio
	}
	return nil
}

// CommandFor decides how a file should be sent: by extension first, then by
// sniffing its content, falling back to a document ("upload").
func (e Extensions) CommandFor(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if ext != "" {
		for _, cmd := range []string{"sendimage", "sendvideo", "sendaudio"} {
			if slices.Contains(e.AllowFor(cmd), ext) {
				return cmd
			}
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return "upload"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	switch mime := http.DetectContentType(buf[:n]); {
	case strings.HasPrefix(mime, "image/"):
		return "sendimage"
	case strings.HasPrefix(mime, "video/"):
		return "sendvideo"
	case strings.HasPrefix(mime, "audio/"):
		return "sendaudio"
	}
	return "upload"
}

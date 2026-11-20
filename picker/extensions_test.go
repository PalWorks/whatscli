// SPDX-License-Identifier: AGPL-3.0-only

package picker

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func defaultExtensions() Extensions {
	return Extensions{
		Image: ParseExtensions("jpg,jpeg,png,webp"),
		Video: ParseExtensions("mp4,3gp,mov,mkv,webm"),
		Audio: ParseExtensions("mp3,ogg,opus,m4a,aac,amr,wav"),
	}
}

func TestParseExtensions(t *testing.T) {
	got := ParseExtensions(" JPG, .png,,webp ")
	want := []string{"jpg", "png", "webp"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseExtensions = %#v, want %#v", got, want)
	}
	if got := ParseExtensions(""); len(got) != 0 {
		t.Fatalf("ParseExtensions(\"\") = %#v, want empty", got)
	}
}

func TestAllowFor(t *testing.T) {
	ext := Extensions{Image: []string{"png"}, Video: []string{"mp4"}, Audio: []string{"mp3"}}
	cases := map[string][]string{
		"sendimage": {"png"},
		"sendvideo": {"mp4"},
		"sendaudio": {"mp3"},
	}
	for cmd, want := range cases {
		if got := ext.AllowFor(cmd); !reflect.DeepEqual(got, want) {
			t.Errorf("AllowFor(%s) = %#v, want %#v", cmd, got, want)
		}
	}
	if got := ext.AllowFor("upload"); got != nil {
		t.Errorf("AllowFor(upload) = %#v, want nil", got)
	}
}

func TestCommandForUppercase(t *testing.T) {
	ext := defaultExtensions()
	cases := map[string]string{
		"/x/IMG_001.JPG": "sendimage",
		"/x/a.MP4":       "sendvideo",
		"/x/a.Opus":      "sendaudio",
		"/x/a.pdf":       "upload",
	}
	for path, want := range cases {
		if got := ext.CommandFor(path); got != want {
			t.Errorf("CommandFor(%q) = %s, want %s", path, got, want)
		}
	}
}

func TestCommandForSniffsUnknownExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "noext")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got := defaultExtensions().CommandFor(path); got != "sendimage" {
		t.Fatalf("CommandFor(png without extension) = %s, want sendimage", got)
	}
}

func TestExtensionsFromConfigDefaults(t *testing.T) {
	got := ExtensionsFromConfig()
	if want := []string{"jpg", "jpeg", "png", "webp"}; !reflect.DeepEqual(got.Image, want) {
		t.Fatalf("Image = %#v, want %#v", got.Image, want)
	}
	if want := []string{"mp3", "ogg", "opus", "m4a", "aac", "amr", "wav"}; !reflect.DeepEqual(got.Audio, want) {
		t.Fatalf("Audio = %#v, want %#v", got.Audio, want)
	}
}

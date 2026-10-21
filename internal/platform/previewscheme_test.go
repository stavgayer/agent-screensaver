package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// makePreviewRoot builds a data folder with the three served folders, one
// folder that must never be served, and symlinks in and out.
func makePreviewRoot(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root, outside = filepath.Join(base, "data"), filepath.Join(base, "secret")
	files := map[string]string{
		filepath.Join(root, "runtime", "runtime.js"):               "js",
		filepath.Join(root, "extensions", "acme.rain", "index.js"): "rain",
		filepath.Join(root, "media", "Application Support.txt"):    "spaced",
		filepath.Join(root, "payloads", "prompt.json"):             "raw prompt",
		filepath.Join(root, "config.json"):                         "{}",
		filepath.Join(outside, "id_rsa"):                           "key",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		filepath.Join(root, "media", "escape"):     filepath.Join(outside, "id_rsa"),
		filepath.Join(root, "media", "inside"):     filepath.Join(root, "runtime", "runtime.js"),
		filepath.Join(root, "media", "secret-dir"): outside,
		filepath.Join(base, "linked-data"):         root,
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	return root, outside
}

func TestResolvePreviewPath(t *testing.T) {
	root, _ := makePreviewRoot(t)
	linkedRoot := filepath.Join(filepath.Dir(root), "linked-data")
	tests := []struct {
		name string
		root string // "" means root
		url  string
		want string // relative to root; "" means not found
	}{
		{"runtime file", "", "ags-preview://data/runtime/runtime.js", "runtime/runtime.js"},
		{"extension file", "", "ags-preview://data/extensions/acme.rain/index.js", "extensions/acme.rain/index.js"},
		{"escaped characters are decoded", "", "ags-preview://data/media/Application%20Support.txt", "media/Application Support.txt"},
		{"symlink that stays inside", "", "ags-preview://data/media/inside", "runtime/runtime.js"},
		{"the root may itself be a symlink", linkedRoot, "ags-preview://data/runtime/runtime.js", "runtime/runtime.js"},
		{"payloads are never served", "", "ags-preview://data/payloads/prompt.json", ""},
		{"config is never served", "", "ags-preview://data/config.json", ""},
		{"dot dot is refused", "", "ags-preview://data/runtime/../../secret/id_rsa", ""},
		{"encoded dot dot is refused", "", "ags-preview://data/runtime/%2e%2e/%2e%2e/secret/id_rsa", ""},
		{"encoded slash and dot dot is refused", "", "ags-preview://data/runtime%2F..%2F..%2Fsecret%2Fid_rsa", ""},
		{"absolute path is refused", "", "ags-preview://data//etc/passwd", ""},
		{"NUL is refused", "", "ags-preview://data/runtime/runtime.js%00.png", ""},
		{"symlink that escapes is refused", "", "ags-preview://data/media/escape", ""},
		{"symlinked folder that escapes is refused", "", "ags-preview://data/media/secret-dir/id_rsa", ""},
		{"unknown host", "", "ags-preview://other/runtime/runtime.js", ""},
		{"other scheme", "", "file:///etc/passwd", ""},
		{"a folder is not a file", "", "ags-preview://data/runtime", ""},
		{"missing file", "", "ags-preview://data/runtime/missing.js", ""},
		{"the root itself", "", "ags-preview://data/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRoot := root
			if tt.root != "" {
				useRoot = tt.root
			}
			got, err := resolvePreviewPath(tt.url, useRoot)
			if tt.want == "" {
				if !errors.Is(err, errPreviewNotFound) {
					t.Fatalf("resolve %q = %q, %v; want not found", tt.url, got, err)
				}
				return
			}
			want, _ := filepath.EvalSymlinks(filepath.Join(root, tt.want))
			if err != nil || got != want {
				t.Fatalf("resolve %q = %q, %v; want %q", tt.url, got, err, want)
			}
		})
	}
}

func TestPreviewMIME(t *testing.T) {
	tests := map[string]string{
		"a.html": "text/html; charset=utf-8", "a.js": "text/javascript; charset=utf-8",
		"a.css": "text/css; charset=utf-8", "a.json": "application/json", "a.png": "image/png",
		"a.JPG": "image/jpeg", "a.jpeg": "image/jpeg", "a.svg": "image/svg+xml", "a.mp4": "video/mp4",
		"a.mov": "video/quicktime", "a.woff2": "font/woff2", "a.bin": "application/octet-stream",
	}
	for file, want := range tests {
		t.Run(file, func(t *testing.T) {
			if got := previewMIME(file); got != want {
				t.Fatalf("previewMIME(%q) = %q, want %q", file, got, want)
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		name          string
		header        string
		size          int64
		start, length int64
		partial       bool
		wantErr       bool
	}{
		{"no header is the whole file", "", 1000, 0, 1000, false, false},
		{"closed range", "bytes=0-1", 1000, 0, 2, true, false},
		{"open range", "bytes=900-", 1000, 900, 100, true, false},
		{"suffix range", "bytes=-100", 1000, 900, 100, true, false},
		{"end past the size is cut", "bytes=990-5000", 1000, 990, 10, true, false},
		{"start past the size", "bytes=1000-", 1000, 0, 0, false, true},
		{"end before start", "bytes=10-5", 1000, 0, 0, false, true},
		{"several ranges are not supported", "bytes=0-1,5-6", 1000, 0, 0, false, true},
		{"other unit", "items=0-1", 1000, 0, 0, false, true},
		{"not a number", "bytes=a-b", 1000, 0, 0, false, true},
		{"empty suffix", "bytes=-0", 1000, 0, 0, false, true},
		{"suffix of an empty file", "bytes=-5", 0, 0, 0, false, true},
		{"no header on an empty file", "", 0, 0, 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, length, partial, err := parseRange(tt.header, tt.size)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseRange(%q) error = %v, want error %v", tt.header, err, tt.wantErr)
			}
			if !tt.wantErr && (start != tt.start || length != tt.length || partial != tt.partial) {
				t.Fatalf("parseRange(%q) = %d, %d, %v; want %d, %d, %v", tt.header, start, length, partial, tt.start, tt.length, tt.partial)
			}
		})
	}
}

func TestServePreview(t *testing.T) {
	root, _ := makePreviewRoot(t)
	large := filepath.Join(root, "media", "clip.mov")
	if err := os.WriteFile(large, make([]byte, maxPreviewChunk+1), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, url, rangeHeader string
		status                 int
		body                   string
		length                 int64
	}{
		{"whole small file", "ags-preview://data/runtime/runtime.js", "", 200, "js", 2},
		{"a range", "ags-preview://data/runtime/runtime.js", "bytes=1-", 206, "s", 1},
		{"a bad range", "ags-preview://data/runtime/runtime.js", "bytes=9-", 416, "", 0},
		{"a large file needs a range", "ags-preview://data/media/clip.mov", "", 416, "", 0},
		{"a range of a large file is cut to one chunk", "ags-preview://data/media/clip.mov", "bytes=0-", 206, "", maxPreviewChunk},
		{"outside the served folders", "ags-preview://data/payloads/prompt.json", "", 404, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := planPreview(tt.url, tt.rangeHeader, root)
			if reply.Status != tt.status || reply.Length != tt.length {
				t.Fatalf("planPreview = %d, length %d; want %d, length %d", reply.Status, reply.Length, tt.status, tt.length)
			}
			if tt.body == "" {
				return
			}
			buffer := make([]byte, reply.Length)
			n, err := readPreview(reply, buffer)
			if err != nil || string(buffer[:n]) != tt.body {
				t.Fatalf("readPreview = %q, %v; want %q", buffer[:n], err, tt.body)
			}
		})
	}
}

// A file that shrinks between the plan and the read gives the bytes that are
// left, never padding.
func TestReadPreviewShrinkingFile(t *testing.T) {
	root, _ := makePreviewRoot(t)
	reply := planPreview("ags-preview://data/extensions/acme.rain/index.js", "", root)
	if err := os.WriteFile(filepath.Join(root, "extensions", "acme.rain", "index.js"), []byte("ra"), 0o644); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, reply.Length)
	n, err := readPreview(reply, buffer)
	if err != nil || string(buffer[:n]) != "ra" {
		t.Fatalf("readPreview = %q, %v; want %q", buffer[:n], err, "ra")
	}
}

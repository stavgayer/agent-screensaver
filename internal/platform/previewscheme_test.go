package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func makePreviewRoot(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root, outside = filepath.Join(base, "data"), filepath.Join(base, "secret")
	for _, dir := range []string{filepath.Join(root, "runtime"), outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(root, "runtime", "runtime.js"):   "js",
		filepath.Join(root, "Application Support.txt"): "spaced",
		filepath.Join(outside, "id_rsa"):               "key",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(outside, "id_rsa"), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "runtime", "runtime.js"), filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

func TestResolvePreviewPath(t *testing.T) {
	root, _ := makePreviewRoot(t)
	tests := []struct {
		name string
		url  string
		want string // relative to root; "" means not found
	}{
		{"file inside the root", "ags-preview://data/runtime/runtime.js", "runtime/runtime.js"},
		{"escaped characters are decoded", "ags-preview://data/Application%20Support.txt", "Application Support.txt"},
		{"symlink that stays inside", "ags-preview://data/inside", "runtime/runtime.js"},
		{"dot dot is refused", "ags-preview://data/runtime/../../secret/id_rsa", ""},
		{"encoded dot dot is refused", "ags-preview://data/runtime/%2e%2e/%2e%2e/secret/id_rsa", ""},
		{"symlink that escapes is refused", "ags-preview://data/escape", ""},
		{"unknown host", "ags-preview://other/runtime/runtime.js", ""},
		{"other scheme", "file:///etc/passwd", ""},
		{"a folder is not a file", "ags-preview://data/runtime", ""},
		{"missing file", "ags-preview://data/runtime/missing.js", ""},
		{"the root itself", "ags-preview://data/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePreviewPath(tt.url, root)
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
	const size = 1000
	tests := []struct {
		name          string
		header        string
		start, length int64
		partial       bool
		wantErr       bool
	}{
		{"no header is the whole file", "", 0, size, false, false},
		{"closed range", "bytes=0-1", 0, 2, true, false},
		{"open range", "bytes=900-", 900, 100, true, false},
		{"suffix range", "bytes=-100", 900, 100, true, false},
		{"end past the size is cut", "bytes=990-5000", 990, 10, true, false},
		{"start past the size", "bytes=1000-", 0, 0, false, true},
		{"end before start", "bytes=10-5", 0, 0, false, true},
		{"several ranges are not supported", "bytes=0-1,5-6", 0, 0, false, true},
		{"other unit", "items=0-1", 0, 0, false, true},
		{"not a number", "bytes=a-b", 0, 0, false, true},
		{"empty suffix", "bytes=-0", 0, 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, length, partial, err := parseRange(tt.header, size)
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
	tests := []struct {
		name, url, rangeHeader string
		status                 int
		body                   string
	}{
		{"whole file", "ags-preview://data/runtime/runtime.js", "", 200, "js"},
		{"a range", "ags-preview://data/runtime/runtime.js", "bytes=1-", 206, "s"},
		{"a bad range", "ags-preview://data/runtime/runtime.js", "bytes=9-", 416, ""},
		{"outside the root", "ags-preview://data/escape", "", 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := servePreview(tt.url, tt.rangeHeader, root)
			if reply.Status != tt.status || string(reply.Body) != tt.body {
				t.Fatalf("servePreview = %d %q, want %d %q", reply.Status, reply.Body, tt.status, tt.body)
			}
		})
	}
}

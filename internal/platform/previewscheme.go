package platform

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The settings window shows its live preview from PreviewScheme, not from
// file://, so the preview frame can be sandboxed: a sandboxed file:// frame
// cannot load file:// scripts in WebKit. ags-preview://data/<path> is <path>
// inside the data directory, and nothing else.
const (
	PreviewScheme = "ags-preview"
	previewHost   = "data"
	// maxPreviewChunk caps one ranged answer, so seeking a long video never
	// reads it whole. Media players ask again for the rest.
	maxPreviewChunk = 4 << 20
)

var errPreviewNotFound = errors.New("not found")

var previewMIMEs = map[string]string{
	".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8",
	".css": "text/css; charset=utf-8", ".json": "application/json",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".svg": "image/svg+xml",
	".mp4": "video/mp4", ".mov": "video/quicktime", ".woff2": "font/woff2",
}

// previewReply is one answer to the preview frame.
type previewReply struct {
	Status      int
	MIME        string
	Body        []byte
	Start, Size int64 // for Content-Range on a 206
}

// servePreview answers one request for rawURL with an optional Range header.
func servePreview(rawURL, rangeHeader, root string) previewReply {
	path, err := resolvePreviewPath(rawURL, root)
	if err != nil {
		return previewReply{Status: 404}
	}
	file, err := os.Open(path)
	if err != nil {
		return previewReply{Status: 404}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return previewReply{Status: 404}
	}
	start, length, partial, err := parseRange(rangeHeader, info.Size())
	if err != nil {
		return previewReply{Status: 416, Size: info.Size()}
	}
	status := 200
	if partial {
		status, length = 206, min(length, maxPreviewChunk)
	}
	body := make([]byte, length)
	if _, err := file.ReadAt(body, start); err != nil && !errors.Is(err, io.EOF) {
		return previewReply{Status: 404}
	}
	return previewReply{Status: status, MIME: previewMIME(path), Body: body, Start: start, Size: info.Size()}
}

// resolvePreviewPath maps ags-preview://data/<path> to a regular file inside
// root. Anything else is errPreviewNotFound: another scheme or host, a ".."
// segment, or a symlink that leads out of root.
func resolvePreviewPath(rawURL, root string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != PreviewScheme || parsed.Host != previewHost {
		return "", errPreviewNotFound
	}
	relative := strings.TrimPrefix(parsed.Path, "/") // Path is already decoded
	if relative == "" || slices.Contains(strings.Split(relative, "/"), "..") || !filepath.IsLocal(relative) {
		return "", errPreviewNotFound
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errPreviewNotFound
	}
	realPath, err := filepath.EvalSymlinks(filepath.Join(realRoot, relative))
	if err != nil {
		return "", errPreviewNotFound
	}
	if inside, err := filepath.Rel(realRoot, realPath); err != nil || !filepath.IsLocal(inside) {
		return "", errPreviewNotFound
	}
	if info, err := os.Stat(realPath); err != nil || !info.Mode().IsRegular() {
		return "", errPreviewNotFound
	}
	return realPath, nil
}

func previewMIME(path string) string {
	if mime, ok := previewMIMEs[strings.ToLower(filepath.Ext(path))]; ok {
		return mime
	}
	return "application/octet-stream"
}

// parseRange reads one "bytes=" range of a file of size bytes. With no
// header it is the whole file. Several ranges are not supported: WebKit's
// media player asks for one at a time.
func parseRange(header string, size int64) (start, length int64, partial bool, err error) {
	if header == "" {
		return 0, size, false, nil
	}
	spec, ok := strings.CutPrefix(header, "bytes=")
	first, last, hasDash := strings.Cut(spec, "-")
	if !ok || !hasDash || strings.Contains(spec, ",") {
		return 0, 0, false, fmt.Errorf("range %q is not supported", header)
	}
	if first == "" { // the last N bytes
		suffix, err := strconv.ParseInt(last, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, fmt.Errorf("range %q is not valid", header)
		}
		start = max(size-suffix, 0)
		return start, size - start, true, nil
	}
	start, err = strconv.ParseInt(first, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, fmt.Errorf("range %q is not satisfiable", header)
	}
	end := size - 1
	if last != "" {
		if end, err = strconv.ParseInt(last, 10, 64); err != nil || end < start {
			return 0, 0, false, fmt.Errorf("range %q is not valid", header)
		}
		end = min(end, size-1)
	}
	return start, end - start + 1, true, nil
}

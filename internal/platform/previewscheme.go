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
	// maxPreviewImage is how large an image may be when sent whole: <img>
	// never asks for ranges, and wallpaper photos are often over 4 MiB.
	maxPreviewImage = 32 << 20
)

var errPreviewNotFound = errors.New("not found")

var previewMIMEs = map[string]string{
	".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8",
	".css": "text/css; charset=utf-8", ".json": "application/json",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".svg": "image/svg+xml",
	".mp4": "video/mp4", ".mov": "video/quicktime", ".woff2": "font/woff2",
}

// previewReply says how to answer one request: the status, and for a 200
// or 206 which bytes of which file.
type previewReply struct {
	Status              int
	MIME, Path          string
	Start, Length, Size int64 // Size is the whole file, for Content-Range
}

// planPreview decides the answer to one request for rawURL with an optional
// Range header. It reads no file content: the caller reads Length bytes into
// its own buffer with readPreview. A file larger than one chunk is only sent
// in ranges, so a request can never make the window read a whole video.
func planPreview(rawURL, rangeHeader, root string) previewReply {
	path, err := resolvePreviewPath(rawURL, root)
	if err != nil {
		return previewReply{Status: 404}
	}
	info, err := os.Stat(path)
	if err != nil {
		return previewReply{Status: 404}
	}
	size := info.Size()
	start, length, partial, err := parseRange(rangeHeader, size)
	wholeLimit := int64(maxPreviewChunk)
	if strings.HasPrefix(previewMIME(path), "image/") {
		wholeLimit = maxPreviewImage
	}
	if err != nil || (!partial && size > wholeLimit) {
		return previewReply{Status: 416, Size: size}
	}
	status := 200
	if partial {
		status, length = 206, min(length, maxPreviewChunk)
	}
	return previewReply{Status: status, MIME: previewMIME(path), Path: path, Start: start, Length: length, Size: size}
}

// readPreview reads the planned bytes into buffer and returns how many it
// read. A file that shrank since the plan gives fewer bytes, never padding.
func readPreview(reply previewReply, buffer []byte) (int, error) {
	file, err := os.Open(reply.Path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	n, err := file.ReadAt(buffer, reply.Start)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return n, err
}

// previewFolders are the only folders of the data directory the preview
// needs: the runtime, the extensions and the media. The rest (payloads with
// raw prompts, the spool, config.json, logs) is never served.
var previewFolders = []string{"runtime", "extensions", "media"}

// resolvePreviewPath maps ags-preview://data/<path> to a regular file inside
// one of previewFolders under root. Anything else is errPreviewNotFound:
// another scheme or host, another folder, a ".." segment, a NUL, or a
// symlink that leads out of the served folders.
func resolvePreviewPath(rawURL, root string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != PreviewScheme || parsed.Host != previewHost {
		return "", errPreviewNotFound
	}
	relative := strings.TrimPrefix(parsed.Path, "/") // Path is already decoded
	segments := strings.Split(relative, "/")
	if slices.Contains(segments, "..") || strings.ContainsRune(relative, 0) || !filepath.IsLocal(relative) ||
		!slices.Contains(previewFolders, segments[0]) {
		return "", errPreviewNotFound
	}
	realPath, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil || !insideServedFolder(realPath, root) {
		return "", errPreviewNotFound
	}
	if info, err := os.Stat(realPath); err != nil || !info.Mode().IsRegular() {
		return "", errPreviewNotFound
	}
	return realPath, nil
}

// insideServedFolder reports whether realPath, with every symlink resolved,
// lies in one of previewFolders, also with its symlinks resolved.
func insideServedFolder(realPath, root string) bool {
	for _, folder := range previewFolders {
		realFolder, err := filepath.EvalSymlinks(filepath.Join(root, folder))
		if err != nil {
			continue
		}
		if inside, err := filepath.Rel(realFolder, realPath); err == nil && filepath.IsLocal(inside) {
			return true
		}
	}
	return false
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
	if size == 0 {
		return 0, 0, false, fmt.Errorf("range %q of an empty file", header)
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

package platform

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework UniformTypeIdentifiers
#include <stdlib.h>
// One answer for the preview scheme. body and mime are malloc'd; the
// Objective-C side frees them.
typedef struct {
	int status;
	char *mime;
	void *body;
	long long length, start, size;
} PreviewReply;
void RunSettingsWindow(const char *title, const char *page, const char *readAccess);
void EvaluateInSettingsWindow(const char *script);
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"unsafe"
)

// Cocoa must run on the main thread: the init in saverwatch_darwin.go locks
// the main goroutine there, which Run relies on.

const settingsWindowTitle = "Agent Screen Saver"

// pageHandler and previewRoot serve the one settings window a process can
// have. They are only touched on the main thread.
var (
	pageHandler PageHandler
	previewRoot string
)

type webKitWindow struct{}

// NewSettingsWindow returns the macOS window: an NSWindow with a WKWebView
// and a script-message bridge named "ags".
func NewSettingsWindow() SettingsWindow { return webKitWindow{} }

// Run shows the page and serves the app loop until the window closes.
func (webKitWindow) Run(page, readAccess string, handle PageHandler) error {
	if _, err := os.Stat(page); err != nil {
		return fmt.Errorf("settings page: %w", err)
	}
	pageHandler, previewRoot = handle, readAccess
	title, cPage, cAccess := C.CString(settingsWindowTitle), C.CString(page), C.CString(readAccess)
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(cPage))
	defer C.free(unsafe.Pointer(cAccess))
	C.RunSettingsWindow(title, cPage, cAccess)
	return nil
}

// settingsPageMessage is called by the script-message handler on the main
// thread with one request from the page, encoded as JSON.
//
//export settingsPageMessage
func settingsPageMessage(message *C.char) {
	script, err := answerPage([]byte(C.GoString(message)), pageHandler)
	if err != nil {
		log.Printf("settings window: %v", err)
		return
	}
	cScript := C.CString(script)
	defer C.free(unsafe.Pointer(cScript))
	C.EvaluateInSettingsWindow(cScript)
}

// previewRequest answers one ags-preview:// request from the preview frame,
// on the main thread. The file is read once, straight into a C buffer that
// the Objective-C side hands to WebKit and frees.
//
//export previewRequest
func previewRequest(rawURL, rangeHeader *C.char, reply *C.PreviewReply) {
	plan := planPreview(C.GoString(rawURL), C.GoString(rangeHeader), previewRoot)
	reply.status = C.int(plan.Status)
	reply.start, reply.size = C.longlong(plan.Start), C.longlong(plan.Size)
	if plan.MIME != "" {
		reply.mime = C.CString(plan.MIME)
	}
	if plan.Length == 0 {
		return
	}
	buffer := C.malloc(C.size_t(plan.Length))
	n, err := readPreview(plan, unsafe.Slice((*byte)(buffer), plan.Length))
	if err != nil || n == 0 {
		C.free(buffer)
		reply.status = 404
		return
	}
	reply.body, reply.length = buffer, C.longlong(n)
}

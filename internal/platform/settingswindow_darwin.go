package platform

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework UniformTypeIdentifiers
#include <stdlib.h>
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

// pageHandler serves the one settings window a process can have. It is only
// touched on the main thread.
var pageHandler PageHandler

type webKitWindow struct{}

// NewSettingsWindow returns the macOS window: an NSWindow with a WKWebView
// and a script-message bridge named "ags".
func NewSettingsWindow() SettingsWindow { return webKitWindow{} }

// Run shows the page and serves the app loop until the window closes.
func (webKitWindow) Run(page, readAccess string, handle PageHandler) error {
	if _, err := os.Stat(page); err != nil {
		return fmt.Errorf("settings page: %w", err)
	}
	pageHandler = handle
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

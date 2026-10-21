package platform

/*
#include <stdlib.h>
char *PickFile(const char *prompt, const char *types, int folders);
*/
import "C"

import (
	"strings"
	"unsafe"
)

type openPanelPicker struct{}

// NewFilePicker returns a picker that shows an NSOpenPanel. It must be used
// on the main thread, from a process that runs the app loop (the settings
// window's handler calls it there).
func NewFilePicker() FilePicker { return openPanelPicker{} }

func (openPanelPicker) PickFile(request FileRequest) (string, bool, error) {
	prompt, types := C.CString(request.Prompt), C.CString(strings.Join(request.Types, ","))
	defer C.free(unsafe.Pointer(prompt))
	defer C.free(unsafe.Pointer(types))
	folders := C.int(0)
	if request.Folders {
		folders = 1
	}
	chosen := C.PickFile(prompt, types, folders)
	if chosen == nil {
		return "", false, nil
	}
	defer C.free(unsafe.Pointer(chosen))
	return C.GoString(chosen), true, nil
}

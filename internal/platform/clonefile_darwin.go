package platform

/*
#include <stdlib.h>
#include <sys/clonefile.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type apfsCloner struct{}

// NewFileCloner returns a cloner that uses clonefile(2). It works only on
// APFS, and only when src and dst are on the same volume.
func NewFileCloner() FileCloner { return apfsCloner{} }

// Clone follows a symbolic link at src, so the clone is always a real file.
// clonefile refuses an existing dst, so nothing is ever overwritten.
func (apfsCloner) Clone(src, dst string) error {
	cSource, cTarget := C.CString(src), C.CString(dst)
	defer C.free(unsafe.Pointer(cSource))
	defer C.free(unsafe.Pointer(cTarget))
	if result, err := C.clonefile(cSource, cTarget, 0); result != 0 {
		return fmt.Errorf("clone %s to %s: %w", src, dst, err)
	}
	return nil
}

package platform

/*
#cgo LDFLAGS: -framework Foundation
void RegisterSaverObservers(void);
int PumpMainLoop(double seconds);
*/
import "C"

import (
	"runtime"
	"sync/atomic"
	"time"
)

// A package init runs on the main thread; locking it here keeps the main
// goroutine there, which RunMainLoop needs.
func init() { runtime.LockOSThread() }

// saverOn is written by the Objective-C callback and read by Active.
var saverOn atomic.Bool

// saverStateChanged is called from the Objective-C observer.
//
//export saverStateChanged
func saverStateChanged(active C.int) {
	saverOn.Store(active != 0)
}

type distributedWatcher struct{}

// NewSaverWatcher returns the watcher. It hears nothing until the main
// goroutine calls RunMainLoop.
func NewSaverWatcher() SaverWatcher { return distributedWatcher{} }

func (distributedWatcher) Active() bool { return saverOn.Load() }

// RunMainLoop listens for the screen saver notifications until done is
// closed. It must be called from the main goroutine, because macOS delivers
// distributed notifications on the main thread.
func RunMainLoop(done <-chan struct{}) {
	C.RegisterSaverObservers()
	for {
		select {
		case <-done:
			return
		default:
		}
		if C.PumpMainLoop(0.2) != 0 {
			time.Sleep(200 * time.Millisecond) // nothing to wait for: do not spin
		}
	}
}

package platform

/*
#cgo LDFLAGS: -framework Foundation
void RunSaverObserver(void);
*/
import "C"

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// saverOn is written by the Objective-C callback and read by Active.
var saverOn atomic.Bool

var startObserver sync.Once

// saverStateChanged is called from the Objective-C observer.
//
//export saverStateChanged
func saverStateChanged(active C.int) {
	saverOn.Store(active != 0)
}

type distributedWatcher struct{}

// NewSaverWatcher starts listening for the screen saver notifications. The
// run loop must stay on one OS thread, so the goroutine locks it and never
// gives it back.
func NewSaverWatcher() SaverWatcher {
	startObserver.Do(func() {
		go func() {
			runtime.LockOSThread()
			C.RunSaverObserver()
		}()
	})
	return distributedWatcher{}
}

func (distributedWatcher) Active() bool { return saverOn.Load() }

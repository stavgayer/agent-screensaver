#import <Foundation/Foundation.h>
#include "_cgo_export.h"

// RegisterSaverObservers must run on the main thread: macOS delivers
// distributed notifications there, so an observer on another thread never
// hears them.
void RegisterSaverObservers(void) {
	NSDistributedNotificationCenter *center = [NSDistributedNotificationCenter defaultCenter];
	[center addObserverForName:@"com.apple.screensaver.didstart" object:nil queue:nil
		usingBlock:^(NSNotification *note) { @autoreleasepool { saverStateChanged(1); } }];
	[center addObserverForName:@"com.apple.screensaver.didstop" object:nil queue:nil
		usingBlock:^(NSNotification *note) { @autoreleasepool { saverStateChanged(0); } }];
}

// PumpMainLoop serves the current run loop for a short while, so the caller
// can check whether to stop. It returns 1 when the run loop has nothing to
// wait for.
int PumpMainLoop(double seconds) {
	@autoreleasepool {
		return CFRunLoopRunInMode(kCFRunLoopDefaultMode, seconds, false) == kCFRunLoopRunFinished;
	}
}

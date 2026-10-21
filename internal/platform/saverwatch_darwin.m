#import <Foundation/Foundation.h>
#include "_cgo_export.h"

// RunSaverObserver subscribes to the screen saver notifications and then
// serves the run loop for ever, so it never returns.
void RunSaverObserver(void) {
	@autoreleasepool {
		NSDistributedNotificationCenter *center = [NSDistributedNotificationCenter defaultCenter];
		[center addObserverForName:@"com.apple.screensaver.didstart" object:nil queue:nil
			usingBlock:^(NSNotification *note) { saverStateChanged(1); }];
		[center addObserverForName:@"com.apple.screensaver.didstop" object:nil queue:nil
			usingBlock:^(NSNotification *note) { saverStateChanged(0); }];
		CFRunLoopRun();
	}
}

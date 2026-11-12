package platform

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/ps/IOPowerSources.h>
#include <IOKit/ps/IOPSKeys.h>

// Fills the out values from the internal battery. Returns 0 when there is
// no battery, 1 when out is filled, -1 when IOKit gives nothing.
static int readBattery(int *onBattery, int *charging, int *percent, int *minutesLeft) {
    CFTypeRef info = IOPSCopyPowerSourcesInfo();
    if (info == NULL) return -1;
    CFArrayRef list = IOPSCopyPowerSourcesList(info);
    if (list == NULL) { CFRelease(info); return -1; }
    int found = 0;
    for (CFIndex i = 0; i < CFArrayGetCount(list) && !found; i++) {
        CFDictionaryRef ps = IOPSGetPowerSourceDescription(info, CFArrayGetValueAtIndex(list, i));
        if (ps == NULL) continue;
        CFStringRef type = CFDictionaryGetValue(ps, CFSTR(kIOPSTypeKey));
        if (type == NULL || !CFEqual(type, CFSTR(kIOPSInternalBatteryType))) continue;
        int current = 0, max = 0, minutes = -1;
        CFNumberRef n;
        if ((n = CFDictionaryGetValue(ps, CFSTR(kIOPSCurrentCapacityKey)))) CFNumberGetValue(n, kCFNumberIntType, &current);
        if ((n = CFDictionaryGetValue(ps, CFSTR(kIOPSMaxCapacityKey)))) CFNumberGetValue(n, kCFNumberIntType, &max);
        if ((n = CFDictionaryGetValue(ps, CFSTR(kIOPSTimeToEmptyKey)))) CFNumberGetValue(n, kCFNumberIntType, &minutes);
        CFStringRef state = CFDictionaryGetValue(ps, CFSTR(kIOPSPowerSourceStateKey));
        CFBooleanRef isCharging = CFDictionaryGetValue(ps, CFSTR(kIOPSIsChargingKey));
        *onBattery = state != NULL && CFEqual(state, CFSTR(kIOPSBatteryPowerValue));
        *charging = isCharging != NULL && CFBooleanGetValue(isCharging);
        *percent = max > 0 ? (current * 100 + max / 2) / max : 0;
        *minutesLeft = minutes;
        found = 1;
    }
    CFRelease(list);
    CFRelease(info);
    return found;
}
*/
import "C"

import (
	"errors"

	"agent-screensaver/internal/power"
)

// BatteryReader reads the internal battery of this Mac through IOKit.
type BatteryReader struct{}

// Read returns one battery sample. A Mac with no internal battery, such as a
// desktop, gives the zero Reading and no error.
func (BatteryReader) Read() (power.Reading, error) {
	var onBattery, charging, percent, minutesLeft C.int
	switch C.readBattery(&onBattery, &charging, &percent, &minutesLeft) {
	case -1:
		return power.Reading{}, errors.New("read battery: IOKit gave no power sources")
	case 0:
		return power.Reading{}, nil
	}
	return power.Reading{
		HasBattery:  true,
		OnBattery:   onBattery != 0,
		Charging:    charging != 0,
		Percent:     min(max(int(percent), 0), 100), // keep a bad IOKit value from leaving 0..100
		MinutesLeft: int(minutesLeft),
	}, nil
}

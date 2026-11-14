package daemon

import "agent-screensaver/internal/power"

// batteryEveryMs is the time between two reads of the battery. A read is a
// few IOKit calls, so Step does it inline.
const batteryEveryMs = 10_000

// BatteryReader reads the battery of this Mac.
type BatteryReader interface {
	Read() (power.Reading, error)
}

// readBattery reads the battery on the first step and then every
// batteryEveryMs, and keeps the last good reading. A failed read clears it,
// so the file shows no battery, and its error is logged only when it
// changes.
func (d *Daemon) readBattery(nowMs int64) {
	if d.Battery == nil {
		d.batteryReading = power.Reading{}
		return
	}
	if d.batteryReadOnce && nowMs-d.batteryReadMs < batteryEveryMs {
		return
	}
	d.batteryReadOnce, d.batteryReadMs = true, nowMs
	reading, err := d.Battery.Read()
	if message := errText(err); message != d.batteryErr {
		d.batteryErr = message
		if message != "" {
			d.Logf("battery: %s", message)
		}
	}
	if err != nil {
		reading = power.Reading{}
	}
	d.batteryReading = reading
}

// batteryWarns reports whether the battery asks the owner to save or stop
// the agents now.
func (d *Daemon) batteryWarns() bool {
	return d.power != nil && d.power.Warn
}

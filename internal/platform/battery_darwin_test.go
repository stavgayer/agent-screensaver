package platform

import (
	"testing"

	"agent-screensaver/internal/power"
)

// Any Mac can run this test. A desktop Mac or a CI runner has no internal
// battery, so the reading must be the zero value; a MacBook must report a
// percent between 0 and 100.
func TestBatteryReaderReads(t *testing.T) {
	reading, err := BatteryReader{}.Read()
	if err != nil {
		t.Skipf("no readable power source on this machine: %v", err)
	}
	if reading.HasBattery {
		if reading.Percent < 0 || reading.Percent > 100 {
			t.Errorf("Percent = %d, want 0..100", reading.Percent)
		}
		return
	}
	if reading != (power.Reading{}) {
		t.Errorf("no battery but reading = %+v, want zero Reading", reading)
	}
}

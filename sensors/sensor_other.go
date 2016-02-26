// +build !linux

package sensors

import (
	// Internal
	"github.com/FlushCapacitor/flush-capacitor/sensors/spec"
)

func FromSpec(ds *spec.DeviceSpec) ([]Sensor, error) {
	panic("Not available on this architecture")
}

func FromSpecFile(filename string) ([]Sensor, error) {
	panic("Not available on this architecture")
}

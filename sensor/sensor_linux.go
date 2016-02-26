package sensors

import (
	// Stdlib
	"encoding/json"
	"os"

	// Internal
	"github.com/FlushCapacitor/flush-capacitor/sensors/rpi"
	"github.com/FlushCapacitor/flush-capacitor/sensors/spec"
)

func FromSpec(ds *spec.DeviceSpec) (Sensor, error) {
	// Check the spec.
	if err := ds.Validate(); err != nil {
		return nil, err
	}

	// Create a sensor based on the spec.
	return rpi.SensorFromSpec(ds)
}

func FromSpecFile(filename string) (Sensor, error) {
	// Open the spec file.
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read and parse the spec file.
	var ds spec.DeviceSpec
	if err := json.NewDecoder(file).Decode(&ds); err != nil {
		return nil, err
	}

	// Pass the spec to FromSpec.
	return FromSpec(&ds)
}
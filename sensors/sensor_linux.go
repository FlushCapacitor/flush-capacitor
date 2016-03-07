package sensors

import (
	// Stdlib
	"encoding/json"
	"os"

	// Internal
	"github.com/FlushCapacitor/flush-capacitor/sensors/rpi"
	"github.com/FlushCapacitor/flush-capacitor/sensors/spec"
)

func FromSpec(ds *spec.DeviceSpec) ([]Sensor, error) {
	// Check the spec.
	if err := ds.Validate(); err != nil {
		return nil, err
	}

	// Instantiate sensors according to the spec.
	sensors := make([]Sensor, 0, len(ds.Circuits))
	for _, circuit := range ds.Circuits {
		sensor, err := rpi.SensorFromCircuitSpec(circuit)
		if err != nil {
			// In case there is an error, close the sensors already created.
			for _, sensor := range sensors {
				sensor.Close()
			}
			// Return the error.
			return err
		}
		sensors = append(sensors, sensor)
	}

	// Return the sensors.
	return sensors, nil
}

func FromSpecFile(filename string) ([]Sensor, error) {
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

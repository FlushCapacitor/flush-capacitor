package sensors

import (
	// Stdlib
	"encoding/json"
	"os"

	// Internal
	"github.com/FlushCapacitor/flush-capacitor/sensors/gpio"
	"github.com/FlushCapacitor/flush-capacitor/sensors/spec"
)

type Sensor interface {
	Name() string
	State() string
	Watch(func()) error
	Close() error
}

func FromSpec(config *spec.Spec) ([]Sensor, error) {
	// Validate the spec.
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Instantiate the sensors according to the spec.
	sensors := make([]Sensor, 0, len(config.Sensors))
	for _, sensorSpec := range config.Sensors {
		sensor, err := gpio.SensorFromSpec(sensorSpec)
		if err != nil {
			return nil, err
		}
		sensors = append(sensors, sensor)
	}
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
	var config spec.Spec
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}

	// Pass the spec to FromSpec.
	return FromSpec(&config)
}

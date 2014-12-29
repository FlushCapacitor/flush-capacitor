package main

import (
	"github.com/FlushCapacitor/flush-capacitor/sensors"
	"github.com/FlushCapacitor/flush-capacitor/sensors/random"
)

func getRandomSensors() []sensors.Sensor {
	return []sensors.Sensor{
		random.NewSensor("L", "unlocked", "locked"),
		random.NewSensor("R", "unlocked", "locked"),
	}
}

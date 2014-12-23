package main

import (
	"github.com/tchap/go-pi-indicator/sensors"
	"github.com/tchap/go-pi-indicator/sensors/random"
)

func getRandomSensors() []sensors.Sensor {
	return []sensors.Sensor{
		random.NewSensor("L", "unlocked", "locked"),
		random.NewSensor("R", "unlocked", "locked"),
	}
}

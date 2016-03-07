package main

import (
	"github.com/FlushCapacitor/flush-capacitor/sensors"
	io "github.com/FlushCapacitor/flush-capacitor/sensors/gpio"
	"github.com/davecheney/gpio"
	"github.com/davecheney/gpio/rpi"
)

func getSensors() ([]sensors.Sensor, error) {
	// Get the sensor for the first toilet.
	pinL, err := rpi.OpenPin(gpio.GPIO24, gpio.ModeInput)
	if err != nil {
		return nil, err
	}
	sensorL := io.NewSensor(pinL, toiletNameLeft, toiletStateUnlocked, toiletStateLocked)

	// Get the sensor for the second toilet.
	pinR, err := rpi.OpenPin(gpio.GPIO25, gpio.ModeInput)
	if err != nil {
		pinL.Close()
		return nil, err
	}
	sensorR := io.NewSensor(pinR, toiletNameRight, toiletStateUnlocked, toiletStateLocked)

	// Return the toilet sensors.
	return []sensors.Sensor{sensorL, sensorR}, nil
}

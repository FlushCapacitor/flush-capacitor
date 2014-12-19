package main

import (
	"github.com/davecheney/gpio"
	"github.com/davecheney/gpio/rpi"
	"github.com/tchap/go-pi-indicator/sensors"
	io "github.com/tchap/go-pi-indicator/sensors/gpio"
)

func getSensors() ([]sensors.Sensor, error) {
	// Get the sensor for the first toilet.
	pinL, err := rpi.OpenPin(gpio.GPIO8, gpio.ModeInput)
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

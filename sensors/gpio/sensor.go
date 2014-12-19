package gpio

import (
	"sync"

	"github.com/davecheney/gpio"
)

type Sensor struct {
	pin gpio.Pin

	name      string
	stateLow  string
	stateHigh string

	mu           *sync.Mutex
	currentValue bool
}

func NewSensor(pin gpio.Pin, name, stateLow, stateHigh string) *Sensor {
	return &Sensor{
		pin:          pin,
		name:         name,
		stateLow:     stateLow,
		stateHigh:    stateHigh,
		mu:           new(sync.Mutex),
		currentValue: pin.Get(),
	}
}

func (sensor *Sensor) Name() string {
	return sensor.name
}

func (sensor *Sensor) State() string {
	if sensor.pin.Get() {
		return sensor.stateHigh
	}
	return sensor.stateLow
}

func (sensor *Sensor) Watch(watcher func()) error {
	return sensor.pin.BeginWatch(gpio.EdgeBoth, func() {
		// Make sure the state actually changed.
		sensor.mu.Lock()
		value := sensor.pin.Get()
		if value == sensor.currentValue {
			sensor.mu.Unlock()
			return
		}
		sensor.currentValue = value
		sensor.mu.Unlock()

		// In case the state changed, run the watcher.
		watcher()
	})
}

func (sensor *Sensor) Close() error {
	return sensor.pin.Close()
}

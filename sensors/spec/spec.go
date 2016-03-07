package spec

import (
	// Stdlib
	"errors"
	"fmt"

	// Vendor
	log "gopkg.in/inconshreveable/log15.v2"
)

type DeviceSpec struct {
	Circuits []*Circuit `json:"circuits"`
}

func (spec *DeviceSpec) Validate() error {
	var (
		logger  = log.New(log.Ctx{"module": "spec"})
		invalid bool
	)

	for i, circuit := range spec.Circuits {
		fieldNotSet := func(path string) {
			logger.Error("required field not set", log.Ctx{
				"path": fmt.Sprintf("circuits[%v].%v", i, path),
			})
			invalid = true
		}

		if circuit.Name == "" {
			fieldNotSet("name")
		}

		if circuit.Sensor == nil || circuit.Sensor.Pin == nil {
			fieldNotSet("sensor.pin")
		}

		if led := circuit.Led; led != nil {
			if led.PinGreen == nil {
				fieldNotSet("led.pin_green")
			}
			if led.PinRed == nil {
				fieldNotSet("led.pin_red")
			}
		}
	}

	if invalid {
		return errors.New("invalid")
	}
	return nil
}

type Circuit struct {
	Name   string      `json:"name"`
	Sensor *SensorSpec `json:"sensor"`
	Led    *LedSpec    `json:"led"`
}

func (circuit *Circuit) SensorPin() int {
	return *circuit.Sensor.Pin
}

func (circuit *Circuit) LedPresent() bool {
	return circuit.Led != nil
}

func (circuit *Circuit) LedPinGreen() int {
	return *circuit.Led.PinGreen
}

func (circuit *Circuit) LedPinRed() int {
	return *circuit.Led.PinRed
}

type SensorSpec struct {
	Pin *int `json:"pin"`
}

type LedSpec struct {
	PinGreen *int `json:"pin_green"`
	PinRed   *int `json:"pin_red"`
}

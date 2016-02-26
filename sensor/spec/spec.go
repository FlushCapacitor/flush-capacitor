package spec

import (
	// Stdlib
	"errors"

	// Vendor
	log "gopkg.in/inconshreveable/log15.v2"
)

type DeviceSpec struct {
	Name   string      `json:"name"`
	Sensor *SensorSpec `json:"sensor"`
	Led    *LedSpec    `json:"led"`
}

type SensorSpec struct {
	Pin *int `json:"pin"`
}

type LedSpec struct {
	PinGreen *int `json:"pin_green"`
	PinRed   *int `json:"pin_red"`
}

func (spec *DeviceSpec) Validate() error {
	var (
		logger  = log.New(log.Ctx{"module": "spec"})
		invalid bool
	)

	fieldNotSet := func(path string) {
		logger.Error("required field not set", log.Ctx{
			"path": path,
		})
		invalid = true
	}

	if spec.Name == "" {
		fieldNotSet("name")
	}

	if spec.Sensor == nil || spec.Sensor.Pin == nil {
		fieldNotSet("sensor.pin")
	}

	if led := spec.Led; led != nil {
		if led.PinGreen == nil {
			fieldNotSet("led.pin_green")
		}
		if led.PinRed == nil {
			fieldNotSet("led.pin_red")
		}
	}

	if invalid {
		return errors.New("invalid")
	}
	return nil
}

func (spec *DeviceSpec) SensorPin() int {
	return *spec.Sensor.Pin
}

func (spec *DeviceSpec) LedPresent() bool {
	return spec.Led != nil
}

func (spec *DeviceSpec) LedPinGreen() int {
	return *spec.Led.PinGreen
}

func (spec *DeviceSpec) LedPinRed() int {
	return *spec.Led.PinRed
}

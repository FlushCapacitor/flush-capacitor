package spec

import (
	// Stdlib
	"errors"

	// Vendor
	log "gopkg.in/inconshreveable/log15.v2"
)

type DeviceSpec struct {
	Name   string `json:"name"`
	Sensor struct {
		Pin int `json:"pin"`
	} `json:"sensor"`
	Led struct {
		PinGreen int `json:"pin_green"`
		PinRed   int `json:"pin_red"`
	} `json:"led"`
}

func (spec *Spec) Validate() error {
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
	if spec.Sensor.Pin == 0 {
		fieldNotSet("sensor.pin")
	}
	if spec.Led.PinGreen != 0 || spec.Led.PinRed != 0 {
		if spec.Led.PinGreen == 0 {
			fieldNotSet("led.pin_green")
		}
		if spec.Led.PinRed == 0 {
			fieldNotSet("led.pin_red")
		}
	}
	if invalid {
		return errors.New("invalid")
	}
	return nil
}

package spec

import (
	// Stdlib
	"errors"
	"fmt"

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
	for i, sensor := range spec.Sensors {
		fieldNotSet := func(path string) {
			logger.Error("required field not set", log.Ctx{
				"path": fmt.Sprintf("sensors[%v].%v", i, path),
			})
			invalid = true
		}

		if sensor.Name == "" {
			fieldNotSet("name")
		}
		if sensor.Switch == nil || sensor.Switch.Pin == 0 {
			fieldNotSet("switch.pin")
		}
		if led := sensor.Led; led != nil {
			if led.PinGreen == 0 {
				fieldNotSet("led.pin_green")
			}
			if led.PinRed == 0 {
				fieldNotSet("led.pin_red")
			}
		}
	}
	if invalid {
		return errors.New("invalid")
	}
	return nil
}

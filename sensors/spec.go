package sensors

import (
	// Stdlib
	"encoding/json"
	"errors"
	"fmt"
	"os"

	// Vendor
	log "gopkg.in/inconshreveable/log15.v2"
)

type Spec struct {
	Sensors []SensorSpec `json:"sensors"`
}

func (spec *Spec) Validate() error {
	var (
		logger = log.New(log.Ctx{
			"module": "sensors",
			"area":   "spec",
		})
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

type SensorSpec struct {
	Name   string      `json:"name"`
	Switch *SwitchSpec `json:"switch"`
	Led    *LedSpec    `json:"led"`
}

type SwitchSpec struct {
	Pin int `json:"pin"`
}

type LedSpec struct {
	PinGreen int `json:"pin_green"`
	PinRed   int `json:"pin_red"`
}

func ReadSpec(filename string) (*Spec, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var spec Spec
	if err := json.NewDecoder(file).Decode(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func SensorsFromSpec(spec *Spec) ([]Sensor, error) {

}

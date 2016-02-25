package config

import (
	// Stdlib
	"encoding/json"
	"errors"
	"fmt"
	"os"

	// Vendor
	log "gopkg.in/inconshreveable/log15.v2"
)

type Config struct {
	Sensors []SensorConfig `json:"sensors"`
}

func (config *Config) Validate() error {
	var (
		logger  = log.New(log.Ctx{"component": "config"})
		invalid bool
	)
	for i, sensor := range config.Sensors {
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

type SensorConfig struct {
	Name   string        `json:"name"`
	Switch *SwitchConfig `json:"switch"`
	Led    *LedConfig    `json:"led"`
}

type SwitchConfig struct {
	Pin int `json:"pin"`
}

type LedConfig struct {
	PinGreen int `json:"pin_green"`
	PinRed   int `json:"pin_red"`
}

func ReadSensorConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

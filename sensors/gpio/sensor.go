package gpio

import (
	// Stdlib
	"sync"

	// Internal
	"github.com/FlushCapacitor/flush-capacitor/sensors/spec"

	// Vendor
	"github.com/davecheney/gpio"
	log "gopkg.in/inconshreveable/log15.v2"
)

const (
	StateLow  = "unlocked"
	StateHigh = "locked"
)

type Sensor struct {
	name string

	pinSwitch   gpio.Pin
	pinLedGreen gpio.Pin
	pinLedRed   gpio.Pin

	valueSwitch bool

	logger log.Logger
	mu     *sync.Mutex
}

func SensorFromSpec(config *spec.Spec) (sensor *Sensor, err error) {
	// Prepare a logger.
	logger := log.New(log.Ctx{"component": "GPIO sensor"})

	// A cleanup helper in case something goes wrong.
	closeOnError := func(pin gpio.Pin, pinNum int) {
		if err != nil {
			if err := pin.Close(); err != nil {
				logger.Error("failed to close a pin", log.Ctx{
					"error": err,
					"pin":   pinNum,
				})
			}
		}
	}

	// Open the switch pin.
	pinSwitch, err := gpio.OpenPin(config.Switch.Pin, gpio.ModeInput)
	if err != nil {
		logger.Error("failed to open the switch pin", log.Ctx{
			"error": err,
			"pin":   config.Switch.Pin,
		})
		return nil, err
	}
	defer closeOnError(pinSwitch, config.Switch.Pin)

	// Open the led pins if necessary.
	var (
		pinLedGreen gpio.Pin
		pinLedRed   gpio.Pin
	)
	if led := config.Led; led != nil {
		// Open the green light pin.
		pinLedGreen, err = gpio.OpenPin(led.PinGreen, gpio.ModeOutput)
		if err != nil {
			logger.Error("failed to open the green light pin", log.Ctx{
				"error": err,
				"pin":   led.PinGreen,
			})
			return nil, err
		}
		defer closeOnError(pinLedGreen, led.PinGreen)

		// Set the green light pin so that is shines green by default.
		pinLedGreen.Set()
		if err := pinLedGreen.Err(); err != nil {
			logger.Error("failed to set the green light pin", log.Ctx{
				"error": err,
				"pin":   led.PinGreen,
			})
			return nil, err
		}

		// Open the red light pin.
		pinLedRed, err = gpio.OpenPin(led.PinGreen, gpio.ModeOutput)
		if err != nil {
			logger.Error("failed to open the red light pin", log.Ctx{
				"error": err,
				"pin":   led.PinRed,
			})
			return nil, err
		}
		defer closeOnError(pinLedRed, led.PinRed)

		// In case the switch is set, set the red light pin as well.
		if pinSwitch.Get() {
			pinLedRed.Set()
			if err := pinLedRed.Err(); err != nil {
				logger.Error("failed to set the red light pin", log.Ctx{
					"error": err,
					"pin":   led.PinRed,
				})
			}
		}
	}

	// Done.
	return &Sensor{
		name:        config.Name,
		pinSwitch:   pinSwitch,
		pinLedGreen: pinLedGreen,
		pinLedRed:   pinLedRed,
		valueSwitch: pinSwitch.Get(),
		logger:      logger,
		mu:          new(sync.Mutex),
	}, nil
}

func (sensor *Sensor) Name() string {
	return sensor.name
}

func (sensor *Sensor) State() string {
	if sensor.pinSwitch.Get() {
		return StateHigh
	}
	return StateLow
}

func (sensor *Sensor) Watch(watcher func()) error {
	return sensor.pinSwitch.BeginWatch(gpio.EdgeBoth, func() {
		// Make sure the state actually changed.
		sensor.mu.Lock()
		value := sensor.pinSwitch.Get()
		if value == sensor.valueSwitch {
			sensor.mu.Unlock()
			return
		}
		sensor.valueSwitch = value
		sensor.mu.Unlock()

		// In case the state changed, run the watcher.
		watcher()
	})
}

func (sensor *Sensor) Close() error {
	// Close the switch.
	if err := sensor.pinSwitch.Close(); err != nil {
		return err
	}

	// Close the led if present.
	if pin := sensor.pinLedGreen; pin != nil {
		if err := pin.Close(); err != nil {
			return err
		}
	}
	if pin := sensor.pinLedRed; pin != nil {
		if err := pin.Close(); err != nil {
			return err
		}
	}

	// Done.
	return nil
}

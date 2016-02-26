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
	StateLow   = "unlocked"
	StateHigh  = "locked"
	StateError = "error"
)

type Sensor struct {
	name  string
	state string

	pinSwitch   gpio.Pin
	pinLedGreen gpio.Pin
	pinLedRed   gpio.Pin

	logger log.Logger
	mu     *sync.RWMutex
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
	}

	// Instantiate a Sensor so that we can start using its methods.
	sensor := &Sensor{
		name:        config.Name,
		pinSwitch:   pinSwitch,
		pinLedGreen: pinLedGreen,
		pinLedRed:   pinLedRed,
		ledPresent:  pinLedGreen != nil && pinLedRed != nil,
		logger:      logger,
		mu:          new(sync.RWMutex),
	}

	// Configure the sensor so that the led is in sync.
	if sensor.ledPresent {
		// Turn the green light on.
		if err := sensor.setLedGreen(); err != nil {
			return nil, err
		}

		// Trun the red light on in case the switch is set.
		switchSet, err := sensor.getSwitch()
		if err != nil {
			return nil, err
		}
		if switchSet {
			if err := sensor.setLedRed(); err != nil {
				return nil, err
			}
		}
	}

	// Done.
	return sensor, nil
}

func (sensor *Sensor) init() error {
	// Read the switch pin so that the internal state is in sync.
	if err := sensor.getSwitch(); err != nil {
		return nil, err
	}

	// Register a watcher.
	if err := pinSwitch.BeginWatch(gpio.EdgeBoth, sensor.onIRQEvent); err != nil {
		logger.Error("failed to begin watching the switch", log.Ctx{
			"error": err,
			"pin":   config.Switch.Pin,
		})
		return nil, err
	}
}

func (sensor *Sensor) onIRQEvent() {
	// Make sure the state actually changed.
	sensor.mu.Lock()
	defer sensor.mu.Unlock()

	// Get the current value.
	value, err := sensor.getSwitch()
	if err != nil {
		watcher()
		return
	}

	// In case there is no change, we are done.
	if value == sensor.valueSwitch {
		return
	}
	sensor.valueSwitch = value

	// Run the watcher.
	watcher()
}

func (sensor *Sensor) getSwitch() (bool, error) {
	sensor.mu.Lock()
	defer sensor.mu.Unlock()

	var (
		value = sensor.pinSwitch.Get()
		err   = sensor.pinSwitch.Err()
	)
	if err != nil {
		sensor.logger.Error("failed to read the switch pin", log.Ctx{
			"error": err,
			"pin":   sensor.pinNumSwitch,
		})
	}
	sensor.state = stateString(value, err)
	return value, err
}

func (sensor *Sensor) Name() string {
	sensor.mu.RLock()
	defer sensor.mu.RUnlock()
	return sensor.name
}

func (sensor *Sensor) State() string {
	sensor.mu.RLock()
	defer sensor.mu.RUnlock()
	return sensor.state
}

func (sensor *Sensor) Watch(watcher func()) error {
	sensor.mu.Lock()
	defer sensor.mu.Unlock()
	sensor.watcher = watcher
	return nil
}

func (sensor *Sensor) Close() error {
	// Close the switch.
	if err := sensor.pinSwitch.Close(); err != nil {
		return err
	}

	// Close the led if present.
	if sensor.ledPresent {
		if err := sensor.pinLedGreen.Close(); err != nil {
			return err
		}
		if err := sensor.pinLedRed.Close(); err != nil {
			return err
		}
	}

	// Done.
	return nil
}

func stateString(value bool, err error) string {
	switch {
	case err != nil:
		return StateError
	case value:
		return StateLocked
	default:
		return StateUnlocked
	}
}

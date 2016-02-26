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

	sensorPin gpio.Pin

	ledPresent  bool
	ledGreenPin gpio.Pin
	ledRedPin   gpio.Pin

	logger log.Logger
	mu     *sync.RWMutex
}

func SensorFromSpec(ds *spec.DeviceSpec) (sensor *Sensor, err error) {
	// Prepare a logger.
	logger := log.New(log.Ctx{"component": "sensor"})

	// A helper to be used to log OpenPin errors.
	failedToOpen := func(err error, kind string, pinNum int) error {
		logger.Error("failed to open a pin", log.Ctx{
			"error": err,
			"kind":  kind,
			"pin":   pinNum,
		})
		return err
	}

	// A cleanup helper in case something breaks. To be used with defer.
	closeOnError := func(pin gpio.Pin, kind string, pinNum int) {
		if err == nil {
			return
		}
		if err := pin.Close(); err != nil {
			logger.Error("failed to close a pin", log.Ctx{
				"error": err,
				"kind":  kind,
				"pin":   pinNum,
			})
		}
	}

	// Open the sensor pin.
	sensorPinNum := ds.SensorPin()
	sensorPin, err := gpio.OpenPin(sensorPinNum, gpio.ModeInput)
	if err != nil {
		return nil, failedToOpen(err, "sensor", sensorPinNum)
	}
	defer closeOnError(sensorPin, "sensor", sensorPinNum)

	// Open the led pins (optional).
	var (
		ledGreenPin gpio.Pin
		ledRedPin   gpio.Pin
	)
	if ds.LedPresent() {
		// Open the green light pin.
		ledGreenPinNum := ds.LedPinGreen()
		ledGreenPin, err = gpio.OpenPin(ledGreenPinNum, gpio.ModeOutput)
		if err != nil {
			return nil, failedToOpen(err, "green light", ledGreenPinNum)
		}
		defer closeOnError(ledGreenPin, "green light", ledGreenPinNum)

		// Open the red light pin.
		ledRedPinNum := ds.LedPinRed()
		ledRedPin, err = gpio.OpenPin(ledRedPinNum, gpio.ModeOutput)
		if err != nil {
			return nil, failedToOpen(err, "red light", ledRedPinNum)
		}
		defer closeOnError(ledRedPin, "red light", ledRedPinNum)
	}

	// Instantiate a Sensor so that we can start using its methods.
	sensor := &Sensor{
		name:        config.Name,
		sensorPin:   pinSwitch,
		ledPresent:  ds.LedPresent(),
		ledGreenPin: ledGreenPin,
		ledRedPin:   ledRedPin,
		logger:      logger,
		mu:          new(sync.RWMutex),
	}

	// Init all the pins.
	if err := sensor.initPins(); err != nil {
		return nil, err
	}

	// Done.
	return sensor, nil
}

func (sensor *Sensor) initPins() error {
	// Read the sensor pin so that the internal state is in sync.
	if err := sensor.getSwitch(); err != nil {
		return nil, err
	}

	// Register a watcher.
	if err := pinSwitch.BeginWatch(gpio.EdgeBoth, sensor.onIRQEvent); err != nil {
		logger.Error("failed to begin watching the sensor", log.Ctx{
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
		sensor.logger.Error("failed to read the sensor pin", log.Ctx{
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
	// Close the sensor.
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

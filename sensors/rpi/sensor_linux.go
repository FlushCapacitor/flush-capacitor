package rpi

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

// Sensor represents a RPi device.
// It implements sensors.Sensor interface.
type Sensor struct {
	name  string
	state string

	ds *spec.DeviceSpec

	sensorPin      gpio.Pin
	sensorPinValue bool

	ledPresent  bool
	ledGreenPin gpio.Pin
	ledRedPin   gpio.Pin

	watcher func()

	logger log.Logger

	mu *sync.RWMutex
}

// SensorFromSpec creates a new sensor according to the device spec.
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
	s := &Sensor{
		name:        ds.Name,
		ds:          ds,
		sensorPin:   sensorPin,
		ledPresent:  ds.LedPresent(),
		ledGreenPin: ledGreenPin,
		ledRedPin:   ledRedPin,
		logger:      logger,
		mu:          new(sync.RWMutex),
	}

	// Init all the pins.
	if err := s.initPins(); err != nil {
		return nil, err
	}

	// Done.
	return s, nil
}

func (sensor *Sensor) initPins() error {
	// Register a watcher.
	if err := sensor.sensorPin.BeginWatch(gpio.EdgeBoth, sensor.onIRQEvent); err != nil {
		sensor.logger.Error("failed to begin watching the sensor", log.Ctx{
			"error": err,
			"pin":   sensor.ds.SensorPin(),
		})
		return err
	}

	// Read the sensor pin so that the internal state is in sync.
	_, _, err := sensor.getUnsafe()
	return err
}

func (sensor *Sensor) onIRQEvent() {
	// Lock.
	sensor.mu.Lock()
	defer sensor.mu.Unlock()

	// Handle panics coming from foreign code.
	defer func() {
		if r := recover(); r != nil {
			sensor.logger.Warn("panic recovered in onIRQEvent", log.Ctx{
				"panic": r,
			})
		}
	}()

	// Run the watcher when appropriate, i.e. when there is an error or the value has changed.
	_, changed, err := sensor.getUnsafe()
	if (err != nil || changed) && sensor.watcher != nil {
		sensor.watcher()
	}
}

func (sensor *Sensor) getUnsafe() (value, changed bool, err error) {
	var (
		v  = sensor.sensorPin.Get()
		ex = sensor.sensorPin.Err()
	)

	if ex != nil {
		sensor.logger.Error("failed to read the sensor pin", log.Ctx{
			"error": err,
			"pin":   sensor.ds.SensorPin(),
		})
		sensor.state = StateError
		return false, false, ex
	}

	value = v
	changed = v != sensor.sensorPinValue

	sensor.state = stateString(v)
	sensor.sensorPinValue = v

	return
}

// Name returns the name assigned to this sensor.
func (sensor *Sensor) Name() string {
	sensor.mu.RLock()
	defer sensor.mu.RUnlock()
	return sensor.name
}

// State returns the state of this sensor.
// Can be "locked", "unlocked" or "error".
func (sensor *Sensor) State() string {
	sensor.mu.RLock()
	defer sensor.mu.RUnlock()
	return sensor.state
}

// Watch sets sensor to run the given watcher function when there is a change.
// The watcher is run for any change, i.e also when an error occurs.
func (sensor *Sensor) Watch(watcher func()) error {
	sensor.mu.Lock()
	defer sensor.mu.Unlock()
	sensor.watcher = watcher
	return nil
}

// Close closes all pins being used internally.
func (sensor *Sensor) Close() error {
	// Close the sensor.
	if err := sensor.sensorPin.Close(); err != nil {
		return err
	}

	// Close the led if necessary.
	if sensor.ledPresent {
		if err := sensor.ledGreenPin.Close(); err != nil {
			return err
		}
		if err := sensor.ledRedPin.Close(); err != nil {
			return err
		}
	}

	// Done.
	return nil
}

func stateString(value bool) string {
	switch {
	case value:
		return StateLow
	default:
		return StateHigh
	}
}
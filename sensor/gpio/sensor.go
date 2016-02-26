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

	ds *spec.DeviceSpec

	sensorPin      gpio.Pin
	sensorPinValue bool

	ledPresent  bool
	ledGreenPin gpio.Pin
	ledRedPin   gpio.Pin

	logger log.Logger

	mu *sync.RWMutex
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
	if err := sensor.initPins(); err != nil {
		return nil, err
	}

	// Done.
	return sensor, nil
}

func (sensor *Sensor) initPins() error {
	// Register a watcher.
	if err := sensor.sensorPin.BeginWatch(gpio.EdgeBoth, sensor.onIRQEvent); err != nil {
		logger.Error("failed to begin watching the sensor", log.Ctx{
			"error": err,
			"pin":   sensor.ds.SensorPin(),
		})
		return err
	}

	// Read the sensor pin so that the internal state is in sync.
	if _, _, err := sensor.getUnsafe(); err != nil {
		return err
	}
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
		v   = sensor.sensorPin.Get()
		err = sensor.sensorPin.Err()
	)

	if err != nil {
		sensor.logger.Error("failed to read the sensor pin", log.Ctx{
			"error": err,
			"pin":   sensor.ds.SensorPin(),
		})
		sensor.state = StateError
		return false, false, err
	}

	value = v
	changed = v != sensor.sensorPinValue
	err = nil

	sensor.state = stateString(v)
	sensor.sensorPinValue = v

	return
}

func (sensor *Sensor) runWatcher() {
	if sensor.watcher != nil {
		sensor.watcher()
	}
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

func stateString(value bool) string {
	switch {
	case value:
		return StateLow
	default:
		return StateHigh
	}
}

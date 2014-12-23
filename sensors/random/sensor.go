package random

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

const MaxWaitingTimeSeconds = 20

var (
	ErrWatcherRegistered = errors.New("watcher already registered")
	ErrClosed            = errors.New("sensor closed")
)

type Sensor struct {
	name         string
	currentState string
	nextState    string
	watcher      func()
	changeCh     chan struct{}
	termCh       chan struct{}
	mu           *sync.Mutex
}

func NewSensor(name, stateLow, stateHigh string) *Sensor {
	sensor := &Sensor{
		name:         name,
		currentState: stateLow,
		nextState:    stateHigh,
		changeCh:     make(chan struct{}),
		termCh:       make(chan struct{}),
		mu:           new(sync.Mutex),
	}

	go sensor.loop()
	return sensor
}

func (sensor *Sensor) Name() string {
	return sensor.name
}

func (sensor *Sensor) State() string {
	sensor.mu.Lock()
	defer sensor.mu.Unlock()
	return sensor.currentState
}

func (sensor *Sensor) toggle() {
	sensor.mu.Lock()
	defer sensor.mu.Unlock()
	sensor.currentState, sensor.nextState = sensor.nextState, sensor.currentState
}

func (sensor *Sensor) Watch(watcher func()) error {
	sensor.mu.Lock()
	defer sensor.mu.Unlock()

	if sensor.watcher != nil {
		return ErrWatcherRegistered
	}
	sensor.watcher = watcher

	go func() {
		for {
			select {
			case <-sensor.changeCh:
				sensor.watcher()
			case <-sensor.termCh:
				return
			}
		}
	}()

	return nil
}

func (sensor *Sensor) Close() error {
	select {
	case <-sensor.termCh:
		return ErrClosed
	default:
		close(sensor.termCh)
		return nil
	}
}

func (sensor *Sensor) loop() {
	var (
		random = rand.New(rand.NewSource(rand.Int63n(42)))
	)
	for {
		select {
		case <-time.After(time.Duration(random.Intn(MaxWaitingTimeSeconds)) * time.Second):
			sensor.toggle()
			sensor.changeCh <- struct{}{}
		case <-sensor.termCh:
			return
		}
	}
}

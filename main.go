package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/FlushCapacitor/flush-capacitor/config"
	"github.com/FlushCapacitor/flush-capacitor/sensors"
)

const (
	toiletNameLeft  = "L"
	toiletNameRight = "R"

	toiletStateUnlocked = "unlocked"
	toiletStateLocked   = "locked"
)

func main() {
	if err := run(); err != nil {
		log.Fatalln("Error:", err)
	}
}

func run() error {
	// Parse the flags.
	addr := flag.String("listen", "localhost:8080", "network address to listen on")
	canonicalUrl := flag.String("canonical_url", "localhost:8080",
		"URL to be used to access the server")
	spec := flag.String("sensor_spec", "", "sensor specification file")

	var forward StringSliceFlag
	flag.Var(&forward, "forward", "forward events from another device")

	flag.Parse()

	// Load the config file when desired.
	var (
		sensorConfig *config.Config
		err          error
	)
	if *spec != "" {
		sensorConfig, err = config.ReadSensorConfig(*spec)
		if err != nil {
			return err
		}
		if err := sensorConfig.Validate(); err != nil {
			return err
		}
	}

	// Instantiate the server.
	srv, err := NewServer(
		SetAddr(*addr),
		SetCanonicalUrl(*canonicalUrl),
		ForwardDevices(forward.Values),
	)
	if err != nil {
		return err
	}

	// Start handling signals to be able to exit cleanly.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-ch
		terminate(srv)
	}()

	// Get the sensors.
	var ss []sensors.Sensor
	switch {
	case len(forward.Values) == 0:
		var err error
		ss, err = getSensors()
		if err != nil {
			return err
		}
	}

	// Close the sensors on exit.
	for _, sensor := range ss {
		defer func(sensor sensors.Sensor) {
			if err := sensor.Close(); err != nil {
				log.Println("Warning:", err)
			}
		}(sensor)
	}

	// Register the sensors with the server.
	for _, sensor := range ss {
		if err := srv.RegisterSensor(sensor); err != nil {
			terminate(srv)
			return err
		}
	}

	// Start processing requests and block until the server is terminated.
	return srv.ListenAndServe()
}

func terminate(srv *Server) {
	if err := srv.Terminate(); err != nil {
		log.Println("Warning:", err)
	}
}

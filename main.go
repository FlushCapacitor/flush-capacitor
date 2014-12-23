package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/tchap/go-pi-indicator/sensors"
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
	rand := flag.Bool("random", false, "trigger sensor changes randomly")
	watch := flag.Bool("watch", false, "watch the toilet status in the command line")

	flag.Parse()

	if *watch {
		return monitor()
	}

	// Instantiate the server.
	srv, err := NewServer(setAddr(*addr), setCanonicalUrl(*canonicalUrl))
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
	if *rand {
		ss = getRandomSensors()
	} else {
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

func setAddr(addr string) func(*Server) {
	return func(srv *Server) {
		srv.SetAddr(addr)
	}
}

func setCanonicalUrl(canonicalUrl string) func(*Server) {
	return func(srv *Server) {
		srv.SetCanonicalUrl(canonicalUrl)
	}
}

func terminate(srv *Server) {
	if err := srv.Terminate(); err != nil {
		log.Println("Warning:", err)
	}
}

func monitor() error {
	// Start catching signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Get the sensors.
	ss, err := getSensors()
	if err != nil {
		return err
	}

	// Close the sensors on exit.
	for _, s := range ss {
		defer func(s sensors.Sensor) {
			if err := s.Close(); err != nil {
				log.Println("Warning:", err)
			}
		}(s)
	}

	// Get the left and the right toilet sensor.
	var (
		left  sensors.Sensor
		right sensors.Sensor
	)
	if ss[0].Name() == toiletNameLeft {
		left, right = ss[0], ss[1]
	} else {
		left, right = ss[1], ss[0]
	}

	// Print the initial status.
	fmt.Println(" L | R")
	fmt.Println("---|---")
	fmt.Printf(" %v | %v\n", toFlag(left.State()), toFlag(right.State()))

	// Start watching the sensors.
	for _, s := range ss {
		err := func(s sensors.Sensor) error {
			return s.Watch(func() {
				fmt.Printf(
					" %v | %v\n", toFlag(left.State()), toFlag(right.State()))
			})
		}(s)
		if err != nil {
			return err
		}
	}

	// Wait for a signal to arrive.
	<-sigCh

	return nil
}

func toFlag(state string) string {
	if state == "locked" {
		return "X"
	}
	return "O"
}

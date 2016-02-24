package forwarder

import (
	// Stdlib
	"time"

	// Internal
	"github.com/FlushCapacitor/flush-capacitor/common"

	// Vendor
	"github.com/gorilla/websocket"
	"gopkg.in/tomb.v2"
)

type Forwarder struct {
	t tomb.Tomb
}

func Forward(changesAddr string, forwardCh chan<- *common.Sensor) (*Forwarder, error) {
	// Connect to the remote address.
	dialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(changesAddr, nil)
	if err != nil {
		return nil, err
	}

	// Start the forwarder.
	var forwarder Forwarder
	forwarder.t.Go(forwarder.loop)
	return &forwarder, nil
}

func (forwarder *Forwarder) loop() error {
	for {
	}
}

func (forwarder *Forwarder) Stop() error {
	forwarder.t.Kill(nil)
	return forwarder.t.Wait()
}

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

const (
	// Every message must be written to the peer in less than 10 seconds.
	writeTimeout = 10 * time.Second

	// Send PING every minute. Compute the read timeout based on that.
	pingPeriod  = 1 * time.Minute
	pongTimeout = pingPeriod / 10 * 11
)

type Forwarder struct {
	ws *websocket.Conn
	t  tomb.Tomb
}

func Forward(changesAddr string, forwardCh chan<- *common.SensorState) (*Forwarder, error) {
	// Connect to the remote address.
	dialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(changesAddr, nil)
	if err != nil {
		return nil, err
	}

	// Start the forwarder.
	forwarder := &Forwarder{ws: conn}
	forwarder.t.Go(forwarder.loop)
	return forwarder, nil
}

func (forwarder *Forwarder) loop() error {
	for {
		messageType, messagePayload, err := forwarder.ws.ReadMessage()
		if err != nil {

		}
	}
}

func (forwarder *Forwarder) Stop() {
	forwarder.t.Kill(nil)
}

func (forwarder *Forwarder) Dead() <-chan struct{} {
	return forwarder.t.Dead()
}

func (forwarder *Forwarder) Wait() error {
	return forwarder.t.Wait()
}

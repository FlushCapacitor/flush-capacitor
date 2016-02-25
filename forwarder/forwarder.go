package forwarder

import (
	// Stdlib
	"encoding/json"
	"time"

	// Internal
	"github.com/FlushCapacitor/flush-capacitor/common"

	// Vendor
	"github.com/gorilla/websocket"
	log "gopkg.in/inconshreveable/log15.v2"
	"gopkg.in/tomb.v2"
)

const (
	// The initial handshake timeout before we start exponential backoff.
	initialHandshakeTimeout = 2 * time.Second

	// The backoff should be limited to certain time.
	maxHandshakeTimeout = 1 * time.Minute

	// Every message must be written to the peer in less than 10 seconds.
	writeTimeout = 10 * time.Second

	// Send PING every minute. Compute the read timeout based on that.
	pingPeriod  = 1 * time.Minute
	pongTimeout = pingPeriod / 10 * 11
)

type Forwarder struct {
	log log.Logger
	t   tomb.Tomb
}

func Forward(changesAddr string, forwardCh chan<- *common.SensorState) *Forwarder {
	// Store the arguments in the forwarder.
	forwarder := &Forwarder{
		log: log.New(log.Ctx{
			"component":      "Forwarder",
			"source address": changedAddr,
		}),
		changesAddr: changesAddr,
		forwardCh:   forwardCh,
	}

	// Start the loop.
	forwarder.t.Go(forwarder.loop)

	// Return the new forwarder.
	return forwarder
}

func (forwarder *Forwarder) loop() error {
	// Set up logging for this goroutine.
	logger := forwarder.log.New(log.Ctx{"thread": "main"})

	// Handshake timeout handling.
	var handshakeTimeout time.Duration

	resetHandshakeTimeout := func() {
		handshakeTimeout = initialHandshakeTimeout
	}

	incrementHandshakeTimeout := func() {
		handshakeTimeout = minInt(2*handshakeTimeout, maxHandshakeTimeout)
	}

	resetHandshakeTimeout()

	for {
		// Try to connect to the source device using the current handshake timeout.
		dialer := &websocket.Dialer{
			HandshakeTimeout: handshakeTimeout,
		}

		logger.Info("connecting to the source device", log.Ctx{
			"timeout": handshakeTimeout,
		})
		conn, resp, err := dialer.Dial(changesAddr, nil)
		if err != nil {
			// In case there is an error, we log it and increment the handshake timeout.
			// Then we try to connect again, doing this infinitely.
			logger.Error("failed to connect to the source device", log.Ctx{
				"error":   err,
				"timeout": handshakeTimeout,
			})
			incrementHandshakeTimeout()
			continue
		}

		// In case we succeeded, we reset the handshake timeout to the initial value.
		resetHandshakeTimeout()

		// Set up read deadlines.
		setReadDeadline := func() {
			conn.SetReadDeadline(time.Now().Add(pongTimeout))
		}

		setReadDeadline()
		conn.SetPongHandler(func(string) error {
			setReadDeadline()
			return nil
		})

		// Start the PING loop.
		go forwarder.loopPing(conn)

		// Enter the receiving loop.
		for {
			// Read a message.
			messageType, messagePayload, err := conn.ReadMessage()
			if err != nil {
				logger.Error("failed to read a message", log.Ctx{"error": err})
				break
			}

			// Drop the message unless it is a text message.
			if messageType != websocket.TextMessage {
				logger.Warn("text message expected, message dropped")
				continue
			}

			// Decode the message payload.
			var event common.SensorStateChangedEvent
			if err := json.NewDecoder(bytes.NewReader(messagePayload)).Decode(&event); err != nil {
				forwarder.log.Warn("failed to decode a message", log.Ctx{
					"error":   err,
					"message": messagePayload,
				})
				continue
			}

			// Forward the event.
			forwarder.forwardCh <- &event
		}
	}
}

func (forwarder *Forwarder) loopPing(conn *websocket.Conn) {
	// Set up logging for this goroutine.
	logger := forwarder.log.New(log.Ctx{"thread": "ping"})

	// Set up a ticker to tick every pingPeriod.
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
	}()

	// Send a PING message every time the ticker ticks.
	for {
		<-ticker.C
		conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			// In case this is a CloseError, return.
			if _, ok := err.(*websocket.CloseError); ok {
				logger.Info("connection closed, exiting...")
				return
			}

			// Otherwise log the error and continue.
			logger.Error("failed to send a PING message", log.Ctx{"error": err})
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

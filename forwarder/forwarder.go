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
	ws       *websocket.Conn
	connTomb tomb.Tomb

	log log.Logger
	t   tomb.Tomb
}

func Forward(changeFeedURL string, forwardCh chan<- *common.SensorStateChangedEvent) *Forwarder {
	// Store the arguments in the forwarder.
	forwarder := &Forwarder{
		log: log.New(log.Ctx{
			"component":      "Forwarder",
			"source address": changedAddr,
		}),
		sourceAddr: changeFeedURL,
		forwardCh:  forwardCh,
	}

	// Start the connection manager.
	forwarder.t.Go(forwarder.connectionManager)

	// Return the new forwarder.
	return forwarder
}

func (forwarder *Forwarder) connectionManager() error {
	// Set up logging for this goroutine.
	logger := forwarder.log.New(log.Ctx{"thread": "connection manager"})

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
		// Try to connect to the source feed using the current handshake timeout.
		dialer := &websocket.Dialer{
			HandshakeTimeout: handshakeTimeout,
		}

		logger.Info("connecting to the source device", log.Ctx{"timeout": handshakeTimeout})

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
		forwarder.conn = conn

		// In case we succeeded, we reset the handshake timeout to the initial value.
		resetHandshakeTimeout()

		// Star the loops.
		var connTomb tomb.Tomb
		connTomb.Go(forwarder.readLoop)
		connTomb.Go(forwarder.writeLoop)

		select {
		// In case the loops die, we continue = reconnect.
		case <-connTomb.Dead():
			continue

		// In case Stop() is called, we close the connection cleanly.
		case <-forwarder.t.Dying():
			// Try to send a CLOSE message to shut down cleanly.
			if err := forwarder.writeMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "KTHXBYE"),
			); err != nil {
				logger.Warn("failed to close the connection cleanly", log.Ctx{"error": err})
			}

			// Then call Close() in any case.
			if err := conn.Close(); err != nil {
				logger.Warn("failed to close the connection", log.Ctx{"error": err})
			}

			// And finally, wait for the loops to die.
			connTomb.Wait()
			return nil
		}
	}
}

func (forwarder *Forwarder) writeLoop() error {
	// Set up logging for this goroutine.
	logger := forwarder.log.New(log.Ctx{"thread": "ping"})

	// Set up a ticker to tick every pingPeriod.
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		// Wait for the next tick.
		case <-ticker.C:
			// Send a PING message.
			if err := forwarder.writeMessage(websocket.PingMessage, []byte{}); err != nil {
				// In case this is a CloseError, log and return.
				if _, ok := err.(*websocket.CloseError); ok {
					logger.Debug("connection closed, exiting...", log.Ctx{"error": err})
					return nil
				}

				// Otherwise log the error and return it.
				// Returning a non-nil error will cause the connection to be closed.
				logger.Error("failed to send PING", log.Ctx{"error": err})
				return err
			}

		// Return immediately in case Stop() is called.
		case <-forwarder.t.Dying():
			logger.Debug("forwarder being stopped, exiting...")
			return nil
		}
	}
}

func (forwarder *Forwarder) readLoop() error {
	// Some initial stuff.
	var (
		conn   = forwarder.conn
		logger = forwarder.log.New(log.Ctx{"thread": "reader"})
	)

	// Read deadlines handling.
	setReadDeadline := func() {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
	}

	setReadDeadline()
	conn.SetPongHandler(func(string) error {
		setReadDeadline()
		return nil
	})

	// Read until the world comes to an end.
	for {
		// Read another message.
		messageType, messagePayload, err := conn.ReadMessage()
		if err != nil {
			// In case this is a CloseError, log and return.
			if _, ok := err.(*websocket.CloseError); ok {
				logger.Debug("connection closed, exiting...", log.Ctx{"error": err})
				return nil
			}

			// Otherwise log the error and return it.
			// Returning a non-nil error will cause the connection to be closed.
			logger.Error("failed to read another message", log.Ctx{"error": err})
			return err
		}

		// Drop the message unless it is a text message.
		if messageType != websocket.TextMessage {
			logger.Warn("text message expected")
			continue
		}

		// Decode the message payload.
		var event common.SensorStateChangedEvent
		if err := json.NewDecoder(bytes.NewReader(messagePayload)).Decode(&event); err != nil {
			logger.Warn("failed to decode a message", log.Ctx{
				"error":   err,
				"message": messagePayload,
			})
			continue
		}

		// Forward the event.
		// Make sure we unblock in case Stop() is called.
		select {
		case forwarder.forwardCh <- &event:
		case forwarder.t.Dying():
			return nil
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

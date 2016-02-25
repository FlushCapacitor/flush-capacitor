package forwarder

import (
	// Stdlib
	"bytes"
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
	// Connection handshake timeout.
	handshakeTimeout = 5 * time.Second

	// The initial handshake timeout before we start exponential backoff.
	initialReconnectBackoff = 2 * time.Second

	// The backoff should be limited to certain time.
	maxReconnectBackoff = 1 * time.Minute

	// Every message must be written to the peer in less than 10 seconds.
	writeTimeout = 10 * time.Second

	// Send PING every minute. Compute the read timeout based on that.
	pingPeriod  = 1 * time.Minute
	pongTimeout = pingPeriod / 10 * 11
)

type Forwarder struct {
	sourceAddr string
	ws         *websocket.Conn

	forwardCh chan<- *common.SensorStateChangedEvent

	log log.Logger
	t   tomb.Tomb
}

func Forward(changesFeedURL string, forwardCh chan<- *common.SensorStateChangedEvent) *Forwarder {
	// Store the arguments in the forwarder.
	forwarder := &Forwarder{
		log: log.New(log.Ctx{
			"component":      "Forwarder",
			"source address": changesFeedURL,
		}),
		sourceAddr: changesFeedURL,
		forwardCh:  forwardCh,
	}

	// Start the connection manager.
	forwarder.t.Go(forwarder.connectionManager)

	// Return the new forwarder.
	return forwarder
}

func (forwarder *Forwarder) connectionManager() error {
	// Set up logging.
	logger := forwarder.log.New(log.Ctx{"thread": "connection manager"})

	// Set up exponential backoff for reconnection.
	var (
		reconnectCh      <-chan time.Time
		reconnectBackoff time.Duration
	)

	reconnectNow := func() {
		logger.Debug("will try to reconnect now")
		reconnectCh = time.After(0)
		reconnectBackoff = initialReconnectBackoff
	}

	reconnectWithBackoff := func() {
		logger.Debug("will try to reconnect", log.Ctx{
			"backoff (seconds)": reconnectBackoff / time.Second,
		})
		reconnectCh = time.After(reconnectBackoff)
		reconnectBackoff = minDuration(2*reconnectBackoff, maxReconnectBackoff)
	}

	reconnectNow()

	// A tomb for read/write goroutines.
	var workerTomb tomb.Tomb

	// Enter the main loop.
	for {
		select {
		// Here we obviously try to reconnect.
		case <-reconnectCh:
			// Try to connect to the source feed.
			dialer := &websocket.Dialer{
				HandshakeTimeout: handshakeTimeout,
			}

			logger.Info("connecting to the source changes feed", log.Ctx{"timeout": handshakeTimeout})

			ws, _, err := dialer.Dial(forwarder.sourceAddr, nil)
			if err != nil {
				// In case there is an error, we log it and increase the backoff.
				// Then we try to connect again, doing this infinitely.
				logger.Error("failed to connect to the source changes feed", log.Ctx{
					"error":   err,
					"timeout": handshakeTimeout,
				})
				reconnectWithBackoff()
				continue
			}
			forwarder.ws = ws

			// In case we succeed, we set reconnectCh to nil not to reconnect again immediately.
			reconnectCh = nil

			// Star the loops.
			workerTomb = tomb.Tomb{}
			workerTomb.Go(forwarder.readLoop)
			workerTomb.Go(forwarder.writeLoop)

		// In case the loops die, we reconnect immediately.
		case <-workerTomb.Dead():
			reconnectNow()

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
			if err := forwarder.ws.Close(); err != nil {
				logger.Warn("failed to close the connection", log.Ctx{"error": err})
			}

			// And finally, wait for the loops to die.
			logger.Debug("waiting for the worker threads to exit")
			workerTomb.Wait()
			return nil
		}
	}
}

func (forwarder *Forwarder) readLoop() error {
	// Some initial stuff.
	var (
		conn   = forwarder.ws
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
		case <-forwarder.t.Dying():
			return nil
		}
	}
}

func (forwarder *Forwarder) writeLoop() error {
	// Set up logging for this goroutine.
	logger := forwarder.log.New(log.Ctx{"thread": "writer"})

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

func (forwarder *Forwarder) writeMessage(messageType int, messagePayload []byte) error {
	forwarder.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
	return forwarder.ws.WriteMessage(messageType, messagePayload)
}

// Stop causes the forwarder to stop forwarding.
func (forwarder *Forwarder) Stop() {
	forwarder.t.Kill(nil)
}

// Dead returns a channel that is closed once the forwarder has stopped.
func (forwarder *Forwarder) Dead() <-chan struct{} {
	return forwarder.t.Dead()
}

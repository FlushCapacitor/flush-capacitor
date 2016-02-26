package server

import (
	// Stdlib
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"

	// Internal
	"github.com/FlushCapacitor/flush-capacitor/common"
	"github.com/FlushCapacitor/flush-capacitor/forwarder"
	"github.com/FlushCapacitor/flush-capacitor/sensors"

	// Vendor
	"github.com/codegangsta/negroni"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/unrolled/render"
)

var (
	ErrRunning     = errors.New("the server is already running")
	ErrTerminating = errors.New("the server is terminating")
	ErrTerminated  = errors.New("the server has been terminated")
)

type ErrRegistered struct {
	sensorName string
}

func (err *ErrRegistered) Error() string {
	return fmt.Sprintf("sensor already registered: %v", err.sensorName)
}

type Server struct {
	addr         string
	canonicalUrl string
	forwardAddrs []string

	handler  http.Handler
	homePage []byte

	listener net.Listener

	sensors []*common.Sensor
	conns   []*websocket.Conn

	forwarders []*forwarder.Forwarder
	forwardCh  chan *common.Sensor

	registerSensorCh chan *registerCmd
	sensorChangedCh  chan *common.Sensor
	registerConnCh   chan *websocket.Conn
	unregisterConnCh chan *websocket.Conn
	serveSensorsCh   chan *sensorsCmd
	runningCh        chan struct{}
	termCh           chan struct{}
	termAckCh        chan struct{}
}

func New(options ...func(*Server)) (*Server, error) {
	// Instantiate Server.
	srv := &Server{
		addr:             "localhost:8080",
		canonicalUrl:     "localhost:8080",
		forwardCh:        make(chan *common.Sensor),
		registerSensorCh: make(chan *registerCmd),
		sensorChangedCh:  make(chan *common.Sensor),
		registerConnCh:   make(chan *websocket.Conn),
		unregisterConnCh: make(chan *websocket.Conn),
		serveSensorsCh:   make(chan *sensorsCmd),
		runningCh:        make(chan struct{}),
		termCh:           make(chan struct{}),
		termAckCh:        make(chan struct{}),
	}

	// Apply the options.
	for _, option := range options {
		option(srv)
	}

	// Prepare the request handlers.
	r := mux.NewRouter()
	r.Path("/").Methods("GET").HandlerFunc(srv.serveHome)
	r.Path("/api/sensors").Methods("GET").HandlerFunc(srv.serveSensors)
	r.Path("/changes").HandlerFunc(srv.serveSensorChanges)

	srv.handler = r

	// Pre-render the home page.
	indexTemplate, err := template.ParseFiles("app/index.html")
	if err != nil {
		return nil, err
	}
	var homePage bytes.Buffer
	err = indexTemplate.Execute(&homePage, map[string]string{
		"CanonicalUrl": srv.canonicalUrl,
	})
	if err != nil {
		return nil, err
	}
	srv.homePage = homePage.Bytes()

	// Start the loop and return the server instance.
	go srv.loop()
	return srv, nil
}

// Exported methods ------------------------------------------------------------

func SetAddr(addr string) func(*Server) {
	return func(srv *Server) {
		srv.addr = addr
	}
}

func SetCanonicalUrl(canonicalUrl string) func(*Server) {
	return func(srv *Server) {
		srv.canonicalUrl = canonicalUrl
	}
}

func ForwardDevices(addrs []string) func(*Server) {
	return func(srv *Server) {
		srv.forwardAddrs = addrs
	}
}

type registerCmd struct {
	sensor sensors.Sensor
	errCh  chan error
}

func (srv *Server) RegisterSensor(sensor sensors.Sensor) error {
	errCh := make(chan error, 1)
	srv.registerSensorCh <- &registerCmd{sensor, errCh}
	return <-errCh
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	srv.handler.ServeHTTP(w, r)
}

func (srv *Server) ListenAndServe() error {
	// Make sure the server is not running.
	select {
	case <-srv.runningCh:
		return ErrRunning
	default:
		// Mark the server as running.
		close(srv.runningCh)
	}

	// Enable Negroni.
	n := negroni.Classic()
	n.UseHandler(srv.handler)
	srv.handler = n

	// Create a listener for the chosen network address.
	listener, err := net.Listen("tcp", srv.addr)
	if err != nil {
		return err
	}
	srv.listener = listener

	// In case forwarding is enabled, start the forwarding goroutines.
	if n := len(srv.forwardAddrs); n != 0 {
		srv.forwarders = make([]*forwarder.Forwarder, 0, n)
		for _, addr := range srv.forwardAddrs {
			srv.forwarders = append(srv.forwarders, forwarder.Start(addr, srv.forwardCh))
		}
	}

	// Start serving requests.
	return http.Serve(srv.listener, srv.handler)
}

func (srv *Server) Terminate() error {
	// Make sure we are not terminating already.
	select {
	case <-srv.termCh:
		return ErrTerminating
	case <-srv.termAckCh:
		return ErrTerminated
	default:
		// Signal the loop to return.
		close(srv.termCh)
	}

	// Wait for the loop to return.
	<-srv.termAckCh

	// Stop the forwarders.
	for _, forwarder := range srv.forwarders {
		forwarder.Stop()
		<-forwarder.Dead()
	}

	// Close the listener. This will unblock ListenAndServe.
	// The listener might be unset in case only ServeHTTP is being used.
	if srv.listener != nil {
		return srv.listener.Close()
	}
	return nil
}

// Server loop -----------------------------------------------------------------

func (srv *Server) loop() {
	renderData := render.New()

Loop:
	for {
		select {
		case cmd := <-srv.registerSensorCh:
			var (
				sensor = cmd.sensor
				errCh  = cmd.errCh
			)

			// Make sure the sensor is not registered yet.
			for _, record := range srv.sensors {
				name := sensor.Name()
				if record.Name == name {
					errCh <- &ErrRegistered{name}
					continue Loop
				}
			}

			// Register the event handler for the sensor.
			err := sensor.Watch(func() {
				srv.sensorChangedCh <- &common.Sensor{sensor.Name(), sensor.State()}
			})
			if err != nil {
				errCh <- err
				continue Loop
			}

			// Add the sensor to the list.
			record := &common.Sensor{
				Name:  sensor.Name(),
				State: sensor.State(),
			}
			srv.sensors = append(srv.sensors, record)
			// Resolve the request as success.
			errCh <- nil

			// Broadcast the status in case the server is running.
			if srv.running() {
				srv.broadcastSensorChange(record)
			}

		case conn := <-srv.registerConnCh:
			// Add the connection to the list.
			srv.conns = append(srv.conns, conn)

			// Dump the internal state into the connection.
			srv.sendSensorStates(conn)

			// Start the goroutine handling the connection.
			// According to the websocket docs, it is necessary to read
			// the incoming messages to process ping and close requests.
			go func(conn *websocket.Conn) {
				for {
					if _, _, err := conn.NextReader(); err != nil {
						srv.unregisterConnCh <- conn
						return
					}
				}
			}(conn)

		case conn := <-srv.unregisterConnCh:
			// Delete the connection from the list.
			for i := 0; i < len(srv.conns); i++ {
				if srv.conns[i] == conn {
					srv.conns = append(srv.conns[:i], srv.conns[i+1:]...)
				}
			}

		case sensor := <-srv.sensorChangedCh:
			// Update the server record.
			var updated bool
			for _, record := range srv.sensors {
				if record.Name == sensor.Name {
					// Update the server record.
					record.State = sensor.State
					updated = true
				}
			}
			// Insert a record in case no record was found.
			if !updated {
				srv.sensors = append(srv.sensors, sensor)
			}

			// Broadcast the change.
			srv.broadcastSensorChange(sensor)

		case sensor := <-srv.forwardCh:
			go func() {
				select {
				case srv.sensorChangedCh <- sensor:
				case <-srv.termCh:
				}
			}()

		case cmd := <-srv.serveSensorsCh:
			// Just dump the sensor records.
			renderData.JSON(cmd.writer, http.StatusOK, srv.sensors)
			close(cmd.done)

		case <-srv.termCh:
			// Close the client connections.
			for _, conn := range srv.conns {
				if err := conn.Close(); err != nil {
					log.Println(err)
				}
			}

			// Wait for the connection goroutines to return.
			for range srv.conns {
				<-srv.unregisterConnCh
			}

			// Exit the loop.
			close(srv.termAckCh)
			return
		}
	}
}

func (srv *Server) sendSensorStates(conn *websocket.Conn) {
	// Send the sensor states as events, one by one.
	for _, sensor := range srv.sensors {
		if err := conn.WriteJSON(sensor); err != nil {
			log.Println(err)
		}
	}
}

func (srv *Server) broadcastSensorChange(sensor *common.Sensor) {
	// Broadcast the change to all connected clients.
	for _, conn := range srv.conns {
		if err := conn.WriteJSON(sensor); err != nil {
			log.Println(err)
		}
	}
}

// Helpers ---------------------------------------------------------------------

func (srv *Server) running() bool {
	select {
	case <-srv.runningCh:
		return true
	default:
		return false
	}
}

func (srv *Server) terminating() bool {
	select {
	case <-srv.termCh:
		return true
	default:
		return false
	}
}

// Request handling ------------------------------------------------------------

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (srv *Server) serveHome(w http.ResponseWriter, r *http.Request) {
	// Make sure the server is not terminating.
	if srv.terminating() {
		http.Error(w, "Server Terminating", http.StatusServiceUnavailable)
		return
	}

	// Just write the pre-rendered home page, that's it.
	w.Header().Set("Content-Type", "text/html")
	if _, err := io.Copy(w, bytes.NewReader(srv.homePage)); err != nil {
		log.Println(err)
	}
}

type sensorsCmd struct {
	writer http.ResponseWriter
	done   chan struct{}
}

func (srv *Server) serveSensors(w http.ResponseWriter, r *http.Request) {
	// Pass the request to the server and wait for the body to be written.
	// In case the server is terminating, return 503 Service Unavailable.
	cmd := &sensorsCmd{w, make(chan struct{})}
	select {
	case srv.serveSensorsCh <- cmd:
		<-cmd.done
	case <-srv.termCh:
		http.Error(w, "Server Terminating", http.StatusServiceUnavailable)
	}
}

func (srv *Server) serveSensorChanges(w http.ResponseWriter, r *http.Request) {
	// Make sure the server is not terminating.
	if srv.terminating() {
		http.Error(w, "Server Terminating", http.StatusServiceUnavailable)
		return
	}

	// Upgrade to WebSocket.
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Try to register the connection. The server state might have changed
	// since the initial check, so in case it is terminating now, close the connection.
	select {
	case srv.registerConnCh <- conn:
	case <-srv.termCh:
		if err := conn.Close(); err != nil {
			log.Println(err)
		}
	}
}

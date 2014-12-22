package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/tchap/go-pi-indicator/sensors"

	"github.com/codegangsta/negroni"
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

type Sensor struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type Server struct {
	addr         string
	canonicalUrl string

	handler  http.Handler
	homePage []byte

	listener net.Listener

	sensors []*Sensor
	conns   []*websocket.Conn

	registerSensorCh chan *registerCmd
	sensorChangedCh  chan *Sensor
	registerConnCh   chan *websocket.Conn
	unregisterConnCh chan *websocket.Conn
	serveSensorsCh   chan *sensorsCmd
	runningCh        chan struct{}
	termCh           chan struct{}
	termAckCh        chan struct{}
}

func NewServer(options ...func(*Server)) (*Server, error) {
	// Instantiate Server.
	srv := &Server{
		addr:             "localhost:8080",
		canonicalUrl:     "localhost:8080",
		registerSensorCh: make(chan *registerCmd),
		sensorChangedCh:  make(chan *Sensor),
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
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.serveHome)
	mux.HandleFunc("/api/sensors", srv.serveSensors)
	mux.HandleFunc("/changes", srv.serveSensorChanges)

	srv.handler = mux

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

func (srv *Server) SetAddr(addr string) {
	srv.addr = addr
}

func (srv *Server) SetCanonicalUrl(canonicalUrl string) {
	srv.canonicalUrl = canonicalUrl
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
	n.Use(method("GET"))
	n.UseHandler(srv.handler)
	srv.handler = n

	// Create a listener for the chosen network address.
	listener, err := net.Listen("tcp", srv.addr)
	if err != nil {
		return err
	}
	srv.listener = listener

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
				srv.sensorChangedCh <- &Sensor{sensor.Name(), sensor.State()}
			})
			if err != nil {
				errCh <- err
				continue Loop
			}

			// Add the sensor to the list.
			record := &Sensor{
				Name:  sensor.Name(),
				State: sensor.State(),
			}
			srv.sensors = append(srv.sensors, record)
			// Resolve the request as success.
			errCh <- nil

			// Broadcast the status in case the server is running.
			if srv.running() {
				srv.broadcastChange(record)
			}

		case conn := <-srv.registerConnCh:
			// Add the connection to the list.
			srv.conns = append(srv.conns, conn)

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
			// Update the server record and broadcast the change.
			for _, record := range srv.sensors {
				if record.Name == sensor.Name {
					// Update the server record.
					record.State = sensor.State

					// Broadcast the change.
					srv.broadcastChange(sensor)
				}
			}

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

func (srv *Server) broadcastChange(sensor *Sensor) {
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

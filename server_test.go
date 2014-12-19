package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// sensors.Sensor implementation for testing -----------------------------------

type testingSensor struct {
	Id      string `json:"name"`
	Status  string `json:"state"`
	watcher func()
}

func (sensor *testingSensor) Name() string {
	return sensor.Id
}

func (sensor *testingSensor) State() string {
	return sensor.Status
}

func (sensor *testingSensor) SetState(state string) {
	sensor.Status = state
	if sensor.watcher != nil {
		sensor.watcher()
	}
}

func (sensor *testingSensor) Watch(watcher func()) error {
	sensor.watcher = watcher
	return nil
}

func (sensor *testingSensor) Close() error {
	return nil
}

// Tests -----------------------------------------------------------------------

func TestServer_GetSensors(t *testing.T) {
	// Instantiate the server.
	srv, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Terminate()

	// Register sensors.
	sensors := []testingSensor{
		{Id: "sensorA", Status: "unlocked"},
		{Id: "sensorB", Status: "unlocked"},
		{Id: "sensorC", Status: "locked"},
	}
	for _, sensor := range sensors {
		func(sensor testingSensor) {
			if err := srv.RegisterSensor(&sensor); err != nil {
				t.Error(err)
				return
			}
		}(sensor)
	}

	// Handle a dummy requests and record the response.
	request, err := http.NewRequest("GET", "http://localhost:8080/api/sensors", nil)
	if err != nil {
		t.Error(err)
		return
	}

	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, request)

	// Make sure the response is as it should be.
	var response []testingSensor
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Error(err)
		return
	}

	if !reflect.DeepEqual(response, sensors) {
		t.Errorf("GET /api/sensors returned %+v, wanted %+v", response, sensors)
	}
}

package common

type SensorStateChangedEvent struct {
	SensorName  string `json:"name"`
	SensorState string `json:"state"`
}

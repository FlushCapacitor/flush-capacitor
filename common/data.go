package common

type StoryStateChangedEvent struct {
	SensorName  string `json:"name"`
	SensorState string `json:"state"`
}

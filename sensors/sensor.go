package sensors

type Sensor interface {
	Name() string
	State() string
	Watch(func()) error
	Close() error
}

// +build !linux

package sensors

func FromSpec(ds *spec.DeviceSpec) (Sensor, error) {
	panic("Not available on this architecture")
}

func FromSpecFile(filename string) (Sensor, error) {
	panic("Not available on this architecture")
}

package forwarder

func minInt64(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

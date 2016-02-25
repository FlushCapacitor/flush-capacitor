package forwarder

import "time"

func minDuration(x, y time.Duration) time.Duration {
	if x < y {
		return x
	}
	return y
}

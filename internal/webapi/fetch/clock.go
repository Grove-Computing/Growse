package fetch

import "time"

type Timer interface{ Stop() bool }
type Clock interface {
	AfterFunc(time.Duration, func()) Timer
}
type systemClock struct{}

func (systemClock) AfterFunc(delay time.Duration, callback func()) Timer {
	return time.AfterFunc(delay, callback)
}

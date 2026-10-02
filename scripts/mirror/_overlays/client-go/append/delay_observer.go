package workqueue

import (
	"time"
)

var DelayObserver func(delay time.Duration)

func ObserveDelay(delay time.Duration) {
	if delay > 0 && DelayObserver != nil {
		DelayObserver(delay)
	}
}

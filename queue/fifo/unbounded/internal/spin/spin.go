// Package spin provides objects for assisting with sleeping while waiting for
// an action to become available.  This is useful in preventing CPU starvation
// when there is nothing to do.
package spin

import (
	"runtime"
	"time"
)

// Sleeper allows sleeping for an increasing time period up to 1 second
// after giving up a goroutine 2^16 times.
// This is not thread-safe and should be thrown away once the loop the calls it
// is able to perform its function.
type Sleeper struct {
	loop uint16

	at time.Duration
}

// Sleep at minimum allows another goroutine to be scheduled and after 2^16
// calls will begin to sleep from 1 nanosecond to 1 second, with each
// call raising the sleep time by a multiple of 10.
func (s *Sleeper) Sleep() {
	const maxSleep = 1 * time.Second

	if s.loop < 65535 {
		runtime.Gosched()
		s.loop++
		return
	}

	if s.at == 0 {
		s.at = 1 * time.Nanosecond
	}

	time.Sleep(s.at)

	if s.at < maxSleep {
		s.at = s.at * 10
		if s.at > maxSleep {
			s.at = maxSleep
		}
	}
}

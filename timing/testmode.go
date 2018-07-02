// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package timing

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	l "github.com/soumya92/barista/logging"
)

type testScheduler struct {
	*scheduler
	startTime time.Time
	interval  time.Duration
}

type trigger struct {
	what *testScheduler
	when time.Time
}

type triggerList []trigger

func (l triggerList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l triggerList) Len() int           { return len(l) }
func (l triggerList) Less(i, j int) bool { return l[i].when.Before(l[j].when) }

var (
	triggers   triggerList
	triggersMu sync.Mutex
)

// nowInTest tracks the current time in test mode.
var nowInTest atomic.Value // of time.Time

func testNow() time.Time {
	return nowInTest.Load().(time.Time)
}

// TestMode sets test mode for all schedulers.
// In test mode schedulers do not fire automatically, and time
// does not pass at all, until NextTick() or Advance* is called.
func TestMode() {
	reset(func() {
		testMode = true
		Now = testNow
		// Set to non-zero time when entering test mode so that any IsZero
		// checks don't unexpectedly pass.
		nowInTest.Store(time.Date(2016, time.November, 25, 20, 47, 0, 0, time.UTC))
	})
}

// ExitTestMode exits test mode for all schedulers. Any schedulers created
// after this call will be real.
func ExitTestMode() {
	reset(func() {
		testMode = false
		Now = time.Now
	})
}

func reset(fn func()) {
	mu.Lock()
	defer mu.Unlock()
	triggersMu.Lock()
	defer triggersMu.Unlock()
	fn()
	waiters = nil
	triggers = nil
	paused = false
}

func sortedTriggers() (sorted triggerList, hasTriggers bool) {
	triggersMu.Lock()
	defer triggersMu.Unlock()
	if len(triggers) == 0 {
		return triggers, false
	}
	sort.Sort(triggers)
	return triggers, true
}

func (s *testScheduler) setNextTrigger(when time.Time) Scheduler {
	newTriggers := triggerList{}
	triggersMu.Lock()
	defer triggersMu.Unlock()
	for _, t := range triggers {
		if t.what != s {
			newTriggers = append(newTriggers, t)
		}
	}
	triggers = newTriggers
	if !when.IsZero() {
		triggers = append(triggers, trigger{s, when})
	}
	return s
}

func (s *testScheduler) nextRepeatingTick() time.Time {
	elapsedIntervals := Now().Sub(s.startTime) / s.interval
	return s.startTime.Add(s.interval * (elapsedIntervals + 1))
}

func (s *testScheduler) At(when time.Time) Scheduler {
	l.Fine("%s At[Test](%v)", l.ID(s), when)
	return s.setNextTrigger(when)
}

func (s *testScheduler) After(delay time.Duration) Scheduler {
	l.Fine("%s After[Test](%v)", l.ID(s), delay)
	return s.setNextTrigger(Now().Add(delay))
}

func (s *testScheduler) Every(interval time.Duration) Scheduler {
	l.Fine("%s Every[Test](%v)", l.ID(s), interval)
	if interval <= 0 {
		panic(errors.New("non-positive interval for Scheduler#Every"))
	}
	s.Lock()
	defer s.Unlock()
	s.startTime = Now()
	s.interval = interval
	return s.setNextTrigger(s.nextRepeatingTick())
}

func (s *testScheduler) Stop() {
	l.Fine("%s Stop[Test]", l.ID(s))
	s.setNextTrigger(time.Time{})
}

// NextTick triggers the next scheduler and returns the trigger time.
// It also advances test time to match.
func NextTick() time.Time {
	sorted, hasTriggers := sortedTriggers()
	if !hasTriggers {
		return testNow()
	}
	when := sorted[0].when
	AdvanceTo(when)
	return testNow()
}

// AdvanceBy increments the test time by the given duration,
// and triggers any schedulers that were scheduled in the meantime.
func AdvanceBy(duration time.Duration) {
	AdvanceTo(Now().Add(duration))
}

// AdvanceTo increments the test time to the given time,
// and triggers any schedulers that were scheduled in the meantime.
func AdvanceTo(newTime time.Time) {
	sorted, hasTriggers := sortedTriggers()
	if !hasTriggers {
		nowInTest.Store(newTime)
		return
	}
	nextTick := sorted[0].when
	if nextTick.After(newTime) {
		nowInTest.Store(newTime)
		return
	}
	if nextTick.After(testNow()) {
		nowInTest.Store(nextTick)
	}
	idx := 0
	for i, t := range sorted {
		if sorted[i].when.After(nextTick) {
			break
		}
		if t.what.interval > 0 {
			t.when = t.what.nextRepeatingTick()
			sorted = append(sorted, t)
		}
		idx = i + 1
		go t.what.maybeTrigger()
	}
	triggersMu.Lock()
	triggers = sorted[idx:]
	triggersMu.Unlock()
	if idx == 0 {
		nowInTest.Store(newTime)
		return
	}
	if newTime.After(testNow()) {
		// We need to give the goroutine from go trigger() some time
		// to execute, otherwise the next one will replace it, causing
		// undercounts when advancing time with a repeated scheduler.
		// TODO: Remove this hack, or decide that this is not a use-case.
		time.Sleep(time.Millisecond)
		AdvanceTo(newTime)
	}
}
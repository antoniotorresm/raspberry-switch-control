package nscontroller

import (
	"github.com/omakoto/go-common/src/common"
	"github.com/omakoto/raspberry-switch-control/nscontroller/utils"
	"github.com/pborman/getopt/v2"
	"time"
)

var (
	tickInterval = getopt.IntLong("tick", 't', 10, "Tick interval in milliseconds")
	defaultAutofireInterval = getopt.IntLong("default-autofire-interval", 'i', 16, "Default autofire interval in milliseconds")
)

type buttonState struct {
	// interval is the autofire interval for each button
	interval time.Duration

	// autoLastTimestamp is the timestamp of the last on or off autofire event.
	autoLastTimestamp time.Duration

	// autoNextTimestamp is the timestamp of the next on or off autofire event.
	autoNextTimestamp time.Duration

	// realButtonPressed is whether the button is actually pressed or not.
	realButtonPressed bool

	// autofireEnabled is whether autofire is enabled or not.
	autofireEnabled bool

	// lastEvent is the last event sent to the next consumer.
	lastEvent Event
}

type AutoFirer struct {
	syncer *utils.Synchronized
	next   Consumer
	states []buttonState
	ticker *time.Ticker
	stop   chan bool
}

var _ Worker = (*AutoFirer)(nil)

func NewAutoFirer(next Consumer) *AutoFirer {
	return &AutoFirer{
		utils.NewSynchronized(),
		next,
		make([]buttonState, NumActionButtons),
		nil,
		nil,
	}
}

func (af *AutoFirer) Run() {
	af.syncer.Run(func() {
		if af.ticker != nil {
			common.Fatal("AutoFirer already running")
			return
		}
		common.Debug("AutoFirer started")
		af.ticker = time.NewTicker(time.Duration(*tickInterval) * time.Millisecond)
		af.stop = make(chan bool)

		ticker := af.ticker
		stop := af.stop

		go func() {
		loop:
			for {
				select {
				case <-ticker.C:
					common.Debug("AutoFirer tick")
				case <-stop:
					break loop
				}
			}
			af.syncer.Run(func() {
				af.ticker = nil
				af.stop = nil
			})
			common.Debug("AutoFirer stopped")
		}()
	})
}

func (af *AutoFirer) Close() error {
	af.syncer.Run(func() {
		if af.ticker != nil {
			common.Debug("AutoFirer stopping")
			af.stop <- true
		} else {
			common.Debug("AutoFirer not running")
		}
	})
	return nil
}

func (af *AutoFirer) SetFireInterval(a Action, interval time.Duration) {
	common.OrFatalf(interval >= 0, "interval must be >= 0 but was: %d", interval)
	af.syncer.Run(func() {
		af.states[a].interval = interval
	})
}

func (af *AutoFirer) updateStateLocked(ev *Event) {
	bs := af.states[ev.Action]
	pressed := ev.Value == 1

	if bs.realButtonPressed == pressed {
		return // Button state hasn't changed; ignore.
	}

	bs.realButtonPressed = pressed

	if bs.interval == 0 {
		// Autofire off
		af.next(ev)
		bs.lastEvent = *ev
	} else {
		// Autofire enabled -- update the autofire on/off state.
		bs.autofireEnabled = pressed
	}
}

func (af *AutoFirer) Consume(ev *Event) {
	af.syncer.Run(func() {
		if ev.Action < NumActionButtons {
			af.updateStateLocked(ev)
		} else {
			// Just forward any axis events.
			af.next(ev)
		}
	})
}
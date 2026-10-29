package ui

import (
	"errors"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/library"
)

func radioArgs(a []string) error {
	if len(a) > 1 || len(a) == 1 && a[0] != "on" && a[0] != "off" {
		return errors.New("radio: give on or off")
	}
	return nil
}

// setRadio is :radio on|off, and gR; alone it switches.
func (m *Model) setRadio(c command.Call) error {
	if err := radioArgs(c.Args); err != nil {
		return err
	}
	on := !m.state.Radio
	if len(c.Args) == 1 {
		on = c.Args[0] == "on"
	}
	m.event(app.SetRadio{On: on})
	return nil
}

// steerRadio is g+ and g-: more or less like the queue row under the
// cursor, or else what plays.
func (m *Model) steerRadio(more bool) error {
	t, ok := m.steerTarget()
	if !ok {
		return errors.New("nothing plays · the radio steers by what plays")
	}
	m.event(app.SteerRadio{Track: t, More: more})
	return nil
}

func (m *Model) steerTarget() (library.Track, bool) {
	if e, ok := m.queueEntry(m.queueCur); ok && m.focus == queueWindow {
		return e.Track, true
	}
	t, _, ok := m.state.NowPlaying()
	return t, ok
}

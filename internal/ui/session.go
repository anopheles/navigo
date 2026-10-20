package ui

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/creds"
)

func onOff(a []string) error {
	if len(a) > 1 || len(a) == 1 && a[0] != "on" && a[0] != "off" {
		return errors.New("hints: give on or off")
	}
	return nil
}

// setHints is :hints on|off, for this session; alone it switches.
func (m *Model) setHints(c command.Call) error {
	if err := onOff(c.Args); err != nil {
		return err
	}
	m.hints = !m.hints
	if len(c.Args) == 1 {
		m.hints = c.Args[0] == "on"
	}
	if m.hints {
		m.setMsg("hint line on", false)
	} else {
		m.setMsg("hint line off for this session · :hints on brings it back", false)
	}
	return nil
}

// logout is :logout: playback stops, the keyring forgets the password,
// and the login form opens. Logging in again starts a fresh session.
func (m *Model) logout(command.Call) error {
	s := m.deps.Config.Server
	m.event(app.LoggedOut{})
	m.event(app.StopPlayback{})
	note := "logged out · the password is forgotten"
	switch err := m.deps.Keyring.Delete(creds.Service, creds.Key(s.URL, s.User)); {
	case s.PasswordCommand != "":
		note = "logged out · server.password_command still gives the password; remove it from " + m.deps.ConfigPath + " to stop that"
	case s.Password != "":
		note = "logged out · the password stays in " + m.deps.ConfigPath + "; remove it there to forget it"
	case err != nil && !errors.Is(err, keyring.ErrNotFound):
		note = fmt.Sprintf("logged out, but the keyring kept the password: %v", err)
	}
	m.openLogin(s.URL, s.User, note)
	return nil
}

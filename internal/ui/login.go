package ui

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/creds"
	"github.com/anopheles/navigo/internal/subsonic"
)

// loginForm asks for credentials and checks them with the server before
// saving. Its keys are a form's, not bindings.
type loginForm struct {
	fields [3]textinput.Model // URL, username, password
	focus  int
	err    string
	busy   bool
	// askPlain is set when the keyring failed and the form asks whether to
	// save the password in config.yaml instead.
	askPlain bool
	client   *subsonic.Client
	url      string
}

const (
	fieldURL = iota
	fieldUser
	fieldPassword
)

func (m *Model) openLogin(url, user, errText string) {
	var f loginForm
	for i, ph := range []string{"https://music.example.com", "username", "password"} {
		t := newInput()
		t.Prompt = ""
		t.Placeholder = ph
		t.SetWidth(40)
		f.fields[i] = t
	}
	f.fields[fieldURL].SetValue(url)
	f.fields[fieldUser].SetValue(user)
	f.fields[fieldPassword].EchoMode = textinput.EchoPassword
	f.fields[fieldPassword].EchoCharacter = '•'
	switch {
	case url == "":
		f.focus = fieldURL
	case user == "":
		f.focus = fieldUser
	default:
		f.focus = fieldPassword
	}
	f.fields[f.focus].Focus()
	f.err = errText
	m.login = f
	m.screen = screenLogin
	m.leaveLine()
	m.engine.Reset()
}

func (m *Model) loginKey(k tea.KeyPressMsg) tea.Cmd {
	f := &m.login
	if f.askPlain {
		switch k.String() {
		case "y", "Y":
			return m.save(f.url, true)
		case "n", "N", "esc":
			return m.save(f.url, false)
		}
		return nil
	}
	if f.busy {
		return nil
	}
	switch k.String() {
	case "esc", "ctrl+c":
		if m.client != nil { // :login mid-session: back to where we were
			m.screen = screenMain
			return nil
		}
		return tea.Quit
	case "tab", "down":
		m.focusField(f.focus + 1)
		return nil
	case "shift+tab", "up":
		m.focusField(f.focus + 2)
		return nil
	case "enter":
		if f.focus < fieldPassword {
			m.focusField(f.focus + 1)
			return nil
		}
		return m.submit()
	}
	var cmd tea.Cmd
	f.fields[f.focus], cmd = f.fields[f.focus].Update(k)
	return cmd
}

func (m *Model) focusField(i int) {
	f := &m.login
	f.fields[f.focus].Blur()
	f.focus = i % len(f.fields)
	f.fields[f.focus].Focus()
}

// submit checks the credentials. A URL without a scheme tries https, then
// http if that can't connect.
func (m *Model) submit() tea.Cmd {
	f := &m.login
	url := strings.TrimRight(strings.TrimSpace(f.fields[fieldURL].Value()), "/")
	user := strings.TrimSpace(f.fields[fieldUser].Value())
	password := f.fields[fieldPassword].Value()
	if url == "" || user == "" || password == "" {
		f.err = "fill in all three fields"
		return nil
	}
	f.busy, f.err = true, ""
	ctx := m.ctx
	return func() tea.Msg {
		try := []string{url}
		if !strings.Contains(url, "://") {
			try = []string{"https://" + url, "http://" + url}
		}
		var last loginMsg
		for _, u := range try {
			c := subsonic.New(subsonic.Credentials{URL: u, Username: user, Password: password})
			last = loginMsg{client: c, url: u, err: c.Login(ctx)}
			var se *subsonic.Error
			if last.err == nil || errors.As(last.err, &se) {
				break // the server answered; another scheme won't help
			}
		}
		return last
	}
}

func (m *Model) loginResult(msg loginMsg) tea.Cmd {
	f := &m.login
	f.busy = false
	switch {
	case subsonic.IsCode(msg.err, subsonic.CodeWrongCredentials):
		f.err = "wrong username or password"
		return nil
	case msg.err != nil:
		f.err = msg.err.Error()
		return nil
	}
	f.client, f.url = msg.client, msg.url
	f.fields[fieldURL].SetValue(msg.url)
	// A password already kept in config.yaml stays there.
	if m.deps.Config.Server.Password != "" {
		return m.save(msg.url, true)
	}
	user, password := f.fields[fieldUser].Value(), f.fields[fieldPassword].Value()
	if err := m.deps.Keyring.Set(creds.Service, creds.Key(msg.url, user), password); err != nil {
		f.askPlain = true
		f.err = "couldn't save the password in the keyring (" + err.Error() + ")"
		return nil
	}
	return m.save(msg.url, false)
}

// save writes the server block, with the password if plain is set.
func (m *Model) save(url string, plain bool) tea.Cmd {
	f := &m.login
	f.askPlain = false
	user := strings.TrimSpace(f.fields[fieldUser].Value())
	password := ""
	if plain {
		password = f.fields[fieldPassword].Value()
	}
	path := m.deps.ConfigPath
	return func() tea.Msg { return savedMsg{err: config.SaveServer(path, url, user, password)} }
}

func (m *Model) saved(msg savedMsg) tea.Cmd {
	f := &m.login
	s := &m.deps.Config.Server
	s.URL, s.User = f.url, strings.TrimSpace(f.fields[fieldUser].Value())
	cmd := m.ready(f.client)
	if msg.err != nil {
		m.setMsg("logged in, but couldn't save "+m.deps.ConfigPath+": "+msg.err.Error(), true)
	} else if !m.shown.err {
		m.setMsg("logged in as "+s.User, false)
	}
	return cmd
}

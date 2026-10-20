//go:build linux

package mediactl

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

// MPRIS 2.2 (https://specifications.freedesktop.org/mpris/latest/).
const (
	busName     = "org.mpris.MediaPlayer2.navigo"
	objectPath  = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	rootIface   = "org.mpris.MediaPlayer2"
	playerIface = "org.mpris.MediaPlayer2.Player"
	noTrack     = dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")
)

// New connects to the session bus and serves MPRIS there. Without a
// session bus there are no media controls, which is not an error worth
// stopping for: it returns the stub and the reason.
func New() (Controls, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return newStub(), fmt.Errorf("media controls: no session bus: %w", err)
	}
	c, err := Serve(conn)
	if err != nil {
		conn.Close()
		return newStub(), err
	}
	return c, nil
}

// mpris serves MPRIS on one connection.
type mpris struct {
	conn  *dbus.Conn
	props *prop.Properties
	cmds  chan Command

	mu   sync.Mutex
	last Status
}

// Serve exports MPRIS on conn and owns the bus name, or a per-process one
// when another navigo has it.
func Serve(conn *dbus.Conn) (Controls, error) {
	m := &mpris{conn: conn, cmds: make(chan Command, 16)}
	if err := conn.Export(root{m}, objectPath, rootIface); err != nil {
		return nil, err
	}
	// A method table, as a method named Seek on a type would look like
	// io.Seeker's to go vet.
	if err := conn.ExportMethodTable(m.playerMethods(), objectPath, playerIface); err != nil {
		return nil, err
	}
	props, err := prop.Export(conn, objectPath, m.propMap())
	if err != nil {
		return nil, err
	}
	m.props = props
	node := &introspect.Node{
		Name: string(objectPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{Name: rootIface, Methods: introspect.Methods(root{}), Properties: props.Introspection(rootIface)},
			{Name: playerIface, Methods: playerIntrospection, Properties: props.Introspection(playerIface),
				Signals: []introspect.Signal{{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}}}},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), objectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, err
	}
	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err == nil && reply != dbus.RequestNameReplyPrimaryOwner {
		reply, err = conn.RequestName(fmt.Sprintf("%s.instance%d", busName, os.Getpid()), dbus.NameFlagDoNotQueue)
	}
	if err != nil {
		return nil, fmt.Errorf("media controls: %w", err)
	}
	return m, nil
}

func (m *mpris) propMap() prop.Map {
	ro := func(v any) *prop.Prop { return &prop.Prop{Value: v, Emit: prop.EmitTrue} }
	return prop.Map{
		rootIface: {
			"CanQuit":             {Value: false, Emit: prop.EmitConst},
			"CanRaise":            {Value: false, Emit: prop.EmitConst},
			"HasTrackList":        {Value: false, Emit: prop.EmitConst},
			"Identity":            {Value: "navigo", Emit: prop.EmitConst},
			"SupportedUriSchemes": {Value: []string{}, Emit: prop.EmitConst},
			"SupportedMimeTypes":  {Value: []string{}, Emit: prop.EmitConst},
		},
		playerIface: {
			"PlaybackStatus": ro("Stopped"),
			"LoopStatus": {Value: string(LoopNone), Writable: true, Emit: prop.EmitTrue, Callback: func(c *prop.Change) *dbus.Error {
				l, _ := c.Value.(string)
				switch Loop(l) {
				case LoopNone, LoopPlaylist, LoopTrack:
					m.send(Command{Kind: SetLoop, Loop: Loop(l)})
					return nil
				}
				return dbus.MakeFailedError(fmt.Errorf("unknown LoopStatus %q", l))
			}},
			"Rate":        {Value: 1.0, Writable: true, Emit: prop.EmitTrue, Callback: func(*prop.Change) *dbus.Error { return nil }},
			"MinimumRate": {Value: 1.0, Emit: prop.EmitConst},
			"MaximumRate": {Value: 1.0, Emit: prop.EmitConst},
			// navigo's shuffle reorders the queue once (S); there is no
			// shuffle mode to report.
			"Shuffle":  {Value: false, Emit: prop.EmitConst},
			"Metadata": ro(metadata(Status{})),
			"Volume": {Value: 1.0, Writable: true, Emit: prop.EmitTrue, Callback: func(c *prop.Change) *dbus.Error {
				v, _ := c.Value.(float64)
				m.send(Command{Kind: SetVolume, Volume: int(v*100 + 0.5)})
				return nil
			}},
			"Position":      {Value: int64(0), Emit: prop.EmitFalse},
			"CanGoNext":     ro(false),
			"CanGoPrevious": ro(false),
			"CanPlay":       ro(false),
			"CanPause":      ro(false),
			"CanSeek":       ro(false),
			"CanControl":    {Value: true, Emit: prop.EmitConst},
		},
	}
}

// send passes a command on, dropping it if navigo is far behind.
func (m *mpris) send(c Command) {
	select {
	case m.cmds <- c:
	default:
	}
}

func (m *mpris) Commands() <-chan Command { return m.cmds }

func (m *mpris) Close() error {
	_, _ = m.conn.ReleaseName(busName)
	return m.conn.Close()
}

// Update sets the properties that changed, which emits
// PropertiesChanged for them, and Seeked when the position jumped.
func (m *mpris) Update(s Status) {
	m.mu.Lock()
	old := m.last
	m.last = s
	m.mu.Unlock()

	set := func(name string, v any) {
		if cur, err := m.props.Get(playerIface, name); err == nil && fmt.Sprint(cur.Value()) == fmt.Sprint(v) {
			return
		}
		m.props.SetMust(playerIface, name, v)
	}
	status := "Stopped"
	switch {
	case s.Playing && s.Paused:
		status = "Paused"
	case s.Playing:
		status = "Playing"
	}
	set("PlaybackStatus", status)
	set("LoopStatus", string(s.Loop))
	set("Metadata", metadata(s))
	set("Volume", float64(s.Volume)/100)
	set("CanGoNext", s.CanNext)
	set("CanGoPrevious", s.CanPrev)
	set("CanPlay", s.CanPlay)
	set("CanPause", s.Playing)
	set("CanSeek", s.Playing)
	m.props.SetMust(playerIface, "Position", micros(s.Position))

	// A jump other than playing on (about a second per update) is a seek.
	if s.TrackID != "" && s.TrackID == old.TrackID {
		if d := s.Position - old.Position; d < 0 || d > 2*time.Second {
			_ = m.conn.Emit(objectPath, playerIface+".Seeked", micros(s.Position))
		}
	}
}

func micros(d time.Duration) int64 { return d.Microseconds() }

// metadata is the Metadata property for a status.
func metadata(s Status) map[string]dbus.Variant {
	if s.TrackID == "" {
		return map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(noTrack)}
	}
	md := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(trackPath(s.TrackID)),
		"mpris:length":  dbus.MakeVariant(micros(s.Length)),
		"xesam:title":   dbus.MakeVariant(s.Title),
		"xesam:album":   dbus.MakeVariant(s.Album),
	}
	if len(s.Artists) > 0 {
		md["xesam:artist"] = dbus.MakeVariant(s.Artists)
	}
	return md
}

// trackPath makes a server's track ID into a D-Bus object path: letters
// and digits stay, anything else becomes _xx.
func trackPath(id string) dbus.ObjectPath {
	var b strings.Builder
	b.WriteString("/navigo/track/")
	for _, c := range []byte(id) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "_%02x", c)
		}
	}
	return dbus.ObjectPath(b.String())
}

// root is org.mpris.MediaPlayer2: navigo can't be raised or quit from
// outside.
type root struct{ m *mpris }

func (root) Raise() *dbus.Error { return nil }
func (root) Quit() *dbus.Error  { return nil }

// playerMethods are org.mpris.MediaPlayer2.Player's methods: each call
// becomes a Command.
func (m *mpris) playerMethods() map[string]any {
	cmd := func(k Kind) func() *dbus.Error {
		return func() *dbus.Error { m.send(Command{Kind: k}); return nil }
	}
	return map[string]any{
		"Next":      cmd(Next),
		"Previous":  cmd(Previous),
		"Pause":     cmd(Pause),
		"PlayPause": cmd(PlayPause),
		"Stop":      cmd(Stop),
		"Play":      cmd(Play),
		"Seek": func(offset int64) *dbus.Error {
			m.send(Command{Kind: SeekBy, Offset: time.Duration(offset) * time.Microsecond})
			return nil
		},
		// SetPosition applies only to the track that plays, as MPRIS says.
		"SetPosition": func(track dbus.ObjectPath, pos int64) *dbus.Error {
			m.mu.Lock()
			cur := m.last
			m.mu.Unlock()
			if cur.TrackID == "" || track != trackPath(cur.TrackID) || pos < 0 || time.Duration(pos)*time.Microsecond > cur.Length {
				return nil
			}
			m.send(Command{Kind: SeekTo, Position: time.Duration(pos) * time.Microsecond})
			return nil
		},
		"OpenUri": func(string) *dbus.Error {
			return dbus.MakeFailedError(fmt.Errorf("navigo doesn't open URIs"))
		},
	}
}

var playerIntrospection = []introspect.Method{
	{Name: "Next"}, {Name: "Previous"}, {Name: "Pause"}, {Name: "PlayPause"}, {Name: "Stop"}, {Name: "Play"},
	{Name: "Seek", Args: []introspect.Arg{{Name: "Offset", Type: "x", Direction: "in"}}},
	{Name: "SetPosition", Args: []introspect.Arg{{Name: "TrackId", Type: "o", Direction: "in"}, {Name: "Position", Type: "x", Direction: "in"}}},
	{Name: "OpenUri", Args: []introspect.Arg{{Name: "Uri", Type: "s", Direction: "in"}}},
}

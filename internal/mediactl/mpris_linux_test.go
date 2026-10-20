//go:build linux

package mediactl

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// privateBus starts a session bus of its own for the test and returns its
// address; it skips without dbus-daemon.
func privateBus(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("no dbus-daemon")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "bus.conf")
	conf := fmt.Sprintf(`<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:path=%s</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>`, filepath.Join(dir, "bus"))
	if err := os.WriteFile(cfg, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("dbus-daemon", "--config-file="+cfg, "--nofork", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("dbus-daemon gave no address: %v", err)
	}
	return strings.TrimSpace(addr)
}

func connect(t *testing.T, addr string) *dbus.Conn {
	t.Helper()
	c, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestMPRIS(t *testing.T) {
	addr := privateBus(t)
	ctl, err := Serve(connect(t, addr))
	if err != nil {
		t.Fatal(err)
	}
	client := connect(t, addr)
	obj := client.Object(busName, objectPath)
	get := func(iface, name string) any {
		t.Helper()
		v, err := obj.GetProperty(iface + "." + name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return v.Value()
	}

	if id := get(rootIface, "Identity"); id != "navigo" {
		t.Errorf("Identity = %v", id)
	}
	if st := get(playerIface, "PlaybackStatus"); st != "Stopped" {
		t.Errorf("PlaybackStatus at start = %v", st)
	}

	// Watch for PropertiesChanged.
	if err := client.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged")); err != nil {
		t.Fatal(err)
	}
	signals := make(chan *dbus.Signal, 10)
	client.Signal(signals)

	ctl.Update(Status{TrackID: "t-1", Title: "Low Tide", Artists: []string{"Aurora Lane"}, Album: "Blue Hour",
		Length: 185 * time.Second, Playing: true, Position: 42 * time.Second, CanNext: true, CanPlay: true, Volume: 80, Loop: LoopPlaylist})
	if st := get(playerIface, "PlaybackStatus"); st != "Playing" {
		t.Errorf("PlaybackStatus = %v", st)
	}
	md, _ := get(playerIface, "Metadata").(map[string]dbus.Variant)
	if md["xesam:title"].Value() != "Low Tide" || md["mpris:length"].Value() != int64(185_000_000) ||
		md["mpris:trackid"].Value() != dbus.ObjectPath("/navigo/track/t_2d1") || fmt.Sprint(md["xesam:artist"].Value()) != "[Aurora Lane]" {
		t.Errorf("Metadata = %v", md)
	}
	if get(playerIface, "Position") != int64(42_000_000) || get(playerIface, "Volume") != 0.8 || get(playerIface, "LoopStatus") != "Playlist" {
		t.Errorf("Position %v, Volume %v, LoopStatus %v", get(playerIface, "Position"), get(playerIface, "Volume"), get(playerIface, "LoopStatus"))
	}
	select {
	case s := <-signals:
		if s.Body[0] != playerIface {
			t.Errorf("PropertiesChanged for %v", s.Body[0])
		}
	case <-time.After(2 * time.Second):
		t.Error("no PropertiesChanged")
	}

	ctl.Update(Status{TrackID: "t-1", Playing: true, Paused: true, Length: 185 * time.Second})
	if st := get(playerIface, "PlaybackStatus"); st != "Paused" {
		t.Errorf("PlaybackStatus = %v", st)
	}

	// Calls come out as commands.
	for _, c := range []struct {
		method string
		args   []any
		want   Command
	}{
		{"PlayPause", nil, Command{Kind: PlayPause}},
		{"Next", nil, Command{Kind: Next}},
		{"Previous", nil, Command{Kind: Previous}},
		{"Seek", []any{int64(10_000_000)}, Command{Kind: SeekBy, Offset: 10 * time.Second}},
		{"SetPosition", []any{dbus.ObjectPath("/navigo/track/t_2d1"), int64(60_000_000)}, Command{Kind: SeekTo, Position: time.Minute}},
	} {
		if call := obj.Call(playerIface+"."+c.method, 0, c.args...); call.Err != nil {
			t.Fatalf("%s: %v", c.method, call.Err)
		}
		select {
		case got := <-ctl.Commands():
			if got != c.want {
				t.Errorf("%s gave %+v, want %+v", c.method, got, c.want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s gave no command", c.method)
		}
	}
	// Setting Volume and LoopStatus asks navigo too.
	if err := obj.SetProperty(playerIface+".Volume", dbus.MakeVariant(0.55)); err != nil {
		t.Fatal(err)
	}
	if got := <-ctl.Commands(); got != (Command{Kind: SetVolume, Volume: 55}) {
		t.Errorf("Volume set gave %+v", got)
	}
	if err := obj.SetProperty(playerIface+".LoopStatus", dbus.MakeVariant("Track")); err != nil {
		t.Fatal(err)
	}
	if got := <-ctl.Commands(); got != (Command{Kind: SetLoop, Loop: LoopTrack}) {
		t.Errorf("LoopStatus set gave %+v", got)
	}

	// A second navigo gets a name of its own.
	second, err := Serve(connect(t, addr))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var names []string
	if err := client.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(names, " "), busName+".instance") {
		t.Errorf("bus names %q lack a second instance", names)
	}
}

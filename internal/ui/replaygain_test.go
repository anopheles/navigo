package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/player"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func TestReplayGainCommand(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.contains("RG track") // on by default
	h.keys(":replaygain album<CR>")
	h.contains("replaygain album", "RG album")
	if c := h.lastCall(); c != "SetReplayGain(album, 0)" {
		t.Errorf("player call %q", c)
	}
	h.keys(":replaygain off<CR>")
	h.lacks("RG ")
	if c := h.lastCall(); c != "SetReplayGain(off, 0)" {
		t.Errorf("player call %q", c)
	}
	// With no argument it moves to the next mode.
	h.keys(":replaygain<CR>")
	h.contains("replaygain track", "RG track")
	h.keys(":replaygain loud<CR>")
	h.contains("replaygain: give off, track or album")
}

func TestReplayGainCompletes(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":replaygain <Tab>")
	if v := h.m.line.Value(); v != "replaygain off" {
		t.Errorf("first completion = %q", v)
	}
}

func TestReplayGainFromConfig(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.Player.ReplayGain, c.Player.ReplayGainPreamp = "album", 2.5
	})
	h.contains("RG album")
	h.keys(":replaygain off<CR>")
	if c := h.lastCall(); c != "SetReplayGain(off, 2.5)" {
		t.Errorf("the preamp was lost: %q", c)
	}
}

func TestReplayGainProblemsShow(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.Player.Problems = []config.Problem{{
			Pos: config.Pos{Line: 3, Col: 14},
			Msg: "player.replaygain: give off, track or album",
		}}
	})
	h.contains("config.yaml has problems", "config.yaml:3:14: player.replaygain")
}

func TestReloadConfigDropsTheReplayGainOverride(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		h.path,
		[]byte("player:\n  replaygain: album\n  replaygain_preamp: 1\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	h.keys(":replaygain off<CR>")
	h.lacks("RG ")
	h.keys(":reload-config<CR>")
	h.contains("config reloaded", "RG album")
	if c := h.lastCall(); c != "SetReplayGain(album, 1)" {
		t.Errorf("player call %q", c)
	}
}

func TestInfoShowsTheGainTags(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>") // Blue Hour opens
	h.keys("<CR>")   // plays its first track
	id := h.fake.Span().Current.ID
	if id == 0 {
		t.Fatal("nothing plays")
	}
	track, album := -6.5, -7.25
	h.fake.Emit(player.GainTags{Entry: id, Track: &track, Album: &album})
	h.pump()
	h.keys(":info<CR>")
	h.contains("ReplayGain", "track -6.50 dB · album -7.25 dB")
	h.keys("q")
	h.fake.Emit(player.GainTags{Entry: id})
	h.pump()
	h.keys(":info<CR>")
	h.contains("no replaygain tags")
}

func TestCheckConfigListsReplayGainProblems(t *testing.T) {
	cfg, err := config.Parse([]byte("player:\n  replaygain_preamp: 99\n"))
	if err != nil {
		t.Fatal(err)
	}
	problems, err := CheckConfig(cfg, "c.yaml")
	if err != nil || len(problems) != 1 ||
		problems[0] != "c.yaml:2:22: player.replaygain_preamp: give dB from -15 to 15" {
		t.Errorf("CheckConfig = %q, %v", problems, err)
	}
}

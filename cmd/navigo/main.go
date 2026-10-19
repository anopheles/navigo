// Command navigo is a keyboard-driven terminal client for Navidrome and
// Subsonic servers.
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/creds"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
	"github.com/anopheles/navigo/internal/subsonic"
	"github.com/anopheles/navigo/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "navigo:", err)
		os.Exit(1)
	}
}

func run() error {
	debug := flag.Bool("debug", os.Getenv("NAVIGO_DEBUG") == "1", "log at debug level")
	configPath := flag.String("config", "", "config file (default: navigo/config.yaml in the user config dir)")
	flag.Parse()

	path := *configPath
	if path == "" {
		var err error
		if path, err = config.Path(); err != nil {
			return err
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	closeLog, err := setupLog(*debug)
	if err != nil {
		return err
	}
	defer closeLog()
	slog.Info("starting", "config", path)

	// The player signs stream URLs with whichever client logged in last.
	var client atomic.Pointer[subsonic.Client]
	mpv := player.NewMPV(player.MPVOptions{
		Args: cfg.Player.MPVArgs,
		StreamURL: func(id library.TrackID) string {
			if c := client.Load(); c != nil {
				return c.StreamURL(string(id))
			}
			return ""
		},
	})
	defer mpv.Close()

	m := ui.New(ui.Deps{
		Config:     cfg,
		ConfigPath: path,
		Player:     mpv,
		Keyring:    creds.OS,
		OnLogin:    func(c *subsonic.Client) { client.Store(c) },
	})
	p := tea.NewProgram(m)
	go func() {
		for e := range mpv.Events() {
			if d, ok := e.(player.Died); ok {
				slog.Warn("player died", "err", d.Err)
			}
			p.Send(ui.PlayerMsg(e))
		}
	}()
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// setupLog sends slog to navigo.log in the user cache dir: nothing may go
// to the terminal while the TUI runs. Stream URLs and tokens are never
// logged.
func setupLog(debug bool) (func(), error) {
	level := slog.LevelWarn
	if debug {
		level = slog.LevelDebug
	}
	var w io.Writer = io.Discard
	closeFn := func() {}
	if dir, err := os.UserCacheDir(); err == nil {
		dir = filepath.Join(dir, "navigo")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			f, err := os.OpenFile(filepath.Join(dir, "navigo.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err == nil {
				w, closeFn = f, func() { _ = f.Close() }
			}
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
	return closeFn, nil
}

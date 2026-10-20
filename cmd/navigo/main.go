// Command navigo is a keyboard-driven terminal client for Navidrome and
// Subsonic servers.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/creds"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/mediactl"
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
	configPath := flag.String(
		"config",
		"",
		"config file (default: navigo/config.yaml in the user config dir)",
	)
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
	switch flag.Arg(0) {
	case "":
	case "check-config":
		return checkConfig(cfg, path)
	default:
		return fmt.Errorf("unknown command %q; the only one is check-config", flag.Arg(0))
	}
	logFile, logPath := setupLog(*debug)
	if logFile != nil {
		defer logFile.Close()
	}
	slog.Debug("starting", "config", path)

	// The player signs stream URLs with whichever client logged in last.
	var client atomic.Pointer[subsonic.Client]
	mpv := player.NewMPV(player.MPVOptions{
		Path: cfg.Player.MPVPath,
		Args: cfg.Player.MPVArgs,
		StreamURL: func(id library.TrackID) string {
			if c := client.Load(); c != nil {
				return c.StreamURL(string(id))
			}
			return ""
		},
	})
	defer mpv.Close()

	cacheDir := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(dir, "navigo")
	}
	stateDir, _ := config.StateDir() // none: history isn't kept
	media, err := mediactl.New()
	if err != nil {
		slog.Warn("no OS media controls", "err", err) // navigo works without them
	}
	defer media.Close()
	m, err := ui.New(ui.Deps{
		Config:     cfg,
		ConfigPath: path,
		Player:     mpv,
		Keyring:    creds.OS,
		CacheDir:   cacheDir,
		StateDir:   stateDir,
		Media:      media,
		LogPath:    logPath,
		OnLogin:    func(c *subsonic.Client) { client.Store(c) },
	})
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var opts []tea.ProgramOption
	if os.Getenv("NO_COLOR") != "" {
		// Any value turns colours off (no-color.org); colorprofile alone
		// would only take a true one. Bold, reverse and the rest stay.
		opts = append(opts, tea.WithColorProfile(colorprofile.ASCII))
	}
	p := tea.NewProgram(m, opts...)
	m.SetSend(p.Send)
	go func() {
		for c := range media.Commands() {
			p.Send(ui.MediaMsg(c))
		}
	}()
	go func() {
		for e := range mpv.Events() {
			if d, ok := e.(player.Died); ok {
				slog.Warn("player died", "err", d.Err)
			}
			p.Send(ui.PlayerMsg(e))
		}
	}()
	// While the TUI runs nothing may reach the terminal, so stderr goes to
	// the log: a panic's stack trace, which Bubble Tea writes there after
	// restoring the terminal, ends up in navigo.log.
	stderr := os.Stderr
	if logFile != nil {
		os.Stderr = logFile
	}
	_, err = p.Run()
	os.Stderr = stderr
	if errors.Is(err, tea.ErrProgramPanic) {
		slog.Error("navigo crashed")
		return fmt.Errorf("crashed; details in %s", logPath)
	}
	return err
}

// checkConfig is navigo check-config: it lists every problem with the
// config's bindings and fails if there are any.
func checkConfig(cfg config.Config, path string) error {
	problems, err := ui.CheckConfig(cfg, path)
	for _, p := range problems {
		fmt.Println(p)
	}
	switch {
	case err != nil:
		return fmt.Errorf("%s: %w", path, err)
	case len(problems) == 1:
		return fmt.Errorf("%s: 1 problem; navigo skips that binding", path)
	case len(problems) > 1:
		return fmt.Errorf("%s: %d problems; navigo skips those bindings", path, len(problems))
	}
	fmt.Println(path + ": fine")
	return nil
}

// setupLog sends slog to navigo.log in the user cache dir: nothing may go
// to the terminal while the TUI runs. Stream URLs and tokens are never
// logged. It returns the open file and its path, or nil and "" when there
// is nowhere to write.
func setupLog(debug bool) (*os.File, string) {
	level := slog.LevelWarn
	if debug {
		level = slog.LevelDebug
	}
	var w io.Writer = io.Discard
	var file *os.File
	var path string
	if dir, err := os.UserCacheDir(); err == nil {
		dir = filepath.Join(dir, "navigo")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			path = filepath.Join(dir, "navigo.log")
			if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				w, file = f, f
				hint := "run with --debug or NAVIGO_DEBUG=1 for more detail"
				if debug {
					hint = "debug logging on"
				}
				fmt.Fprintf(f, "# navigo started %s · %s\n", time.Now().Format(time.RFC3339), hint)
			} else {
				path = ""
			}
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
	return file, path
}

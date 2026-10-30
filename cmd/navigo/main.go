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
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/cover"
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
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(versionString())
		return nil
	}

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
	slog.Debug("starting", "version", versionString(), "config", path)

	// The player signs stream URLs with whichever client logged in last.
	var client atomic.Pointer[subsonic.Client]
	cacheDir := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(dir, "navigo")
	}
	// The screen and the player's cover art in the OS's media controls share it.
	covers := cover.OpenCache(coverDir(cacheDir), 100<<20)
	mpv := player.NewMPV(player.MPVOptions{
		Path: cfg.Player.MPVPath,
		Args: cfg.Player.MPVArgs,
		ReplayGain: player.ReplayGain{
			Mode:   player.GainMode(cfg.Player.GainMode()),
			Preamp: cfg.Player.ReplayGainPreamp,
		},
		StreamURL: func(id library.TrackID) string {
			if c := client.Load(); c != nil {
				return c.StreamURL(string(id))
			}
			return ""
		},
		CoverPath: covers.Path,
	})
	defer mpv.Close()

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
		TrueColor:  trueColor(),
		CoverArtOK: coverColours(),
		CoverKitty: cover.Supported(os.Getenv, tmuxPassthrough),
		InTmux:     os.Getenv("TMUX") != "",
		CoverCache: covers,
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
	m.FlushSaved()
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
	w := io.Discard
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

// trueColor says whether the terminal shows 24-bit colour and colours are on.
func trueColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return colorprofile.Detect(os.Stdout, os.Environ()) == colorprofile.TrueColor
}

// coverColours says the terminal shows 256 colours or more, and colours are
// on: cover art is drawn in colour and has no meaning without.
func coverColours() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return colorprofile.Detect(os.Stdout, os.Environ()) >= colorprofile.ANSI256
}

// coverDir is where the cover files are kept; none without a cache directory.
func coverDir(cacheDir string) string {
	if cacheDir == "" {
		return ""
	}
	return filepath.Join(cacheDir, "covers")
}

// tmuxPassthrough reports whether tmux forwards graphics to the terminal.
func tmuxPassthrough() bool {
	out, err := exec.Command("tmux", "show", "-gv", "allow-passthrough").Output()
	v := strings.TrimSpace(string(out))
	return err == nil && (v == "on" || v == "all")
}

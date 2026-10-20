package mpv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// baseArgs keep mpv quiet, headless and free of the user's mpv config and
// scripts, which would otherwise see navigo's signed stream URLs.
var baseArgs = []string{
	"--idle=yes",
	"--no-terminal",
	"--no-video",
	"--audio-display=no",
	"--no-config",
	"--load-scripts=no",
	"--ytdl=no",
}

// Options configure a spawned mpv.
type Options struct {
	// Path is the mpv binary; "mpv" on PATH when empty.
	Path string
	// Args are extra mpv options (player.mpv_args), placed after navigo's
	// own so they can override them, but before the IPC option.
	Args []string
}

// Process is a running mpv with an open IPC connection.
type Process struct {
	*Conn

	cmd     *exec.Cmd
	cleanup func()
	exited  chan struct{}
	waitErr error
	once    sync.Once
}

// Start spawns mpv and connects to it. It gives up when ctx ends, or after
// 5 seconds if ctx has no deadline. Stream URLs are never passed here: send
// them with loadfile, so other users can't read them from the process list.
func Start(ctx context.Context, opts Options) (*Process, error) {
	path := opts.Path
	if path == "" {
		path = "mpv"
	}
	ipc, cleanup, err := ipcPath()
	if err != nil {
		return nil, fmt.Errorf("mpv: %w", err)
	}
	args := append(append(append([]string{}, baseArgs...), opts.Args...), "--input-ipc-server="+ipc)
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("mpv: %w", err)
	}
	p := &Process{cmd: cmd, cleanup: cleanup, exited: make(chan struct{})}
	go func() {
		p.waitErr = cmd.Wait()
		close(p.exited)
	}()

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	for {
		nc, err := dialIPC(ctx, ipc)
		if err == nil {
			p.Conn = NewConn(nc)
			return p, nil
		}
		select {
		case <-p.exited:
			p.cleanup()
			return nil, fmt.Errorf(
				"mpv exited before it was ready (%v)%s",
				p.waitErr,
				diagnose(path, opts.Args),
			)
		case <-ctx.Done():
			_ = p.kill()
			return nil, fmt.Errorf("mpv: no IPC connection: %w", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Exited is closed when the mpv process has exited.
func (p *Process) Exited() <-chan struct{} { return p.exited }

// Close asks mpv to quit, kills it if it hasn't exited within a second, and
// removes its socket.
func (p *Process) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = p.Command(ctx, "quit")
	select {
	case <-p.exited:
	case <-ctx.Done():
	}
	return p.kill()
}

func (p *Process) kill() error {
	var err error
	p.once.Do(func() {
		if p.Conn != nil {
			_ = p.Conn.Close()
		}
		select {
		case <-p.exited:
		default:
			err = p.cmd.Process.Kill()
			<-p.exited
		}
		p.cleanup()
	})
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

// diagnose runs mpv again with the same extra options but with terminal
// output on, which --no-terminal silences, and returns its error lines.
// It runs only after mpv failed to start, so no stream URL is involved.
func diagnose(path string, args []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		path,
		append([]string{"--no-config", "--idle=no", "--no-video", "--ao=null"}, args...)...)
	out, _ := cmd.CombinedOutput()
	var errs []string
	for line := range strings.Lines(string(out)) {
		if strings.Contains(line, "Error") || strings.Contains(line, "failed") {
			errs = append(errs, strings.TrimSpace(line))
		}
	}
	if len(errs) == 0 {
		return ""
	}
	return ": " + strings.Join(errs, "; ")
}

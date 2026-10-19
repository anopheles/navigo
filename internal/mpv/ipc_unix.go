//go:build !windows

package mpv

import (
	"context"
	"net"
	"os"
	"path/filepath"
)

// ipcPath returns a socket path in a new 0700 directory, short enough for
// sun_path on Linux and macOS, and a func that removes the directory.
func ipcPath() (string, func(), error) {
	dir, err := os.MkdirTemp("", "navigo-mpv-")
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(dir, "mpv.sock"), func() { _ = os.RemoveAll(dir) }, nil
}

func dialIPC(ctx context.Context, path string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", path)
}

//go:build windows

package mpv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"

	"github.com/Microsoft/go-winio"
)

// ipcPath returns a named pipe unique to this run: mpv creates its pipe as
// the first instance, so a reused name would fail. The pipe goes away with
// the process, so there is nothing to clean up.
func ipcPath() (string, func(), error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf(
		`\\.\pipe\navigo-mpv-%d-%s`,
		os.Getpid(),
		hex.EncodeToString(b),
	), func() {}, nil
}

func dialIPC(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}

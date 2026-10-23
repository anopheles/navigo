package subsonic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// ErrNoCover is returned by CoverArt for an entity the server has no cover
// for.
var ErrNoCover = errors.New("no cover art")

// maxCover caps what is read of one cover; Navidrome's are well under 1 MB.
const maxCover = 16 << 20

// coverSize is the size asked for. Navidrome may ignore it and send the
// original; the caller scales either way.
const coverSize = 512

// CoverArt fetches the cover of id, an album, song or other entity the
// server has art for. It returns the image as the server sent it (WebP,
// PNG or JPEG), or ErrNoCover.
func (c *Client) CoverArt(ctx context.Context, id string) ([]byte, error) {
	const endpoint = "getCoverArt"
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, Timeout)
		defer cancel()
	}
	slog.Debug("server request", "endpoint", endpoint)
	q := c.auth()
	q.Del("f") // the answer is an image, not JSON
	q.Set("id", id)
	q.Set("size", strconv.Itoa(coverSize))
	req, err := c.newRequest(ctx, endpoint, q, false)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", endpoint, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("%s: %w", endpoint, ctx.Err())
		}
		return nil, fmt.Errorf("%s: %w", endpoint, unreachable{redact(err)})
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, ErrNoCover
	case resp.StatusCode != http.StatusOK:
		_, _ = io.Copy(io.Discard, resp.Body)
		err := fmt.Errorf("HTTP %s", resp.Status)
		if resp.StatusCode >= 500 {
			err = unreachable{err}
		}
		return nil, fmt.Errorf("%s: %w", endpoint, err)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCover+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", endpoint, unreachable{redact(err)})
	}
	if len(body) > maxCover {
		return nil, fmt.Errorf("%s: the image is over %d MB", endpoint, maxCover>>20)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		// A Subsonic error comes as JSON with a 200: "not found" is 70.
		var se *Error
		if err := decode(body, nil); errors.As(err, &se) {
			if se.Code == CodeNotFound {
				return nil, ErrNoCover
			}
			return nil, fmt.Errorf("%s: %w", endpoint, se)
		}
		return nil, fmt.Errorf("%s: %w", endpoint, unreachable{errors.New("not an image")})
	}
	return body, nil
}

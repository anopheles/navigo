// PROTOTYPE: throwaway (anopheles/navigo#60). Never merge into main.

package subsonic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// CoverArt fetches the cover for an album, song or other entity id, asking
// the server to resize it to size pixels. It returns the bytes and the
// Content-Type.
func (c *Client) CoverArt(ctx context.Context, id string, size int) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	q := c.auth()
	q.Del("f")
	q.Set("id", id)
	if size > 0 {
		q.Set("size", strconv.Itoa(size))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"getCoverArt?"+q.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("cover art: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	ct := resp.Header.Get("Content-Type")
	if err != nil {
		return nil, ct, err
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(ct, "image/") {
		return nil, ct, fmt.Errorf("cover art %q: HTTP %d, %s", id, resp.StatusCode, ct)
	}
	return b, ct, nil
}

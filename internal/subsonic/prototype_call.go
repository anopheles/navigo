package subsonic

// PROTOTYPE, throwaway: lives only on branch prototype/seeded-mix, for
// "Does cache-only closeness feel like a radio on a real library?" (#68).

import (
	"context"
	"net/url"
)

// PrototypeCall exposes the raw call so the seeded-Mix prototype can ask
// for getSimilarSongs2, getTopSongs and getArtistInfo2.
func (c *Client) PrototypeCall(ctx context.Context, endpoint string, params url.Values, v any) error {
	return c.call(ctx, endpoint, params, v)
}

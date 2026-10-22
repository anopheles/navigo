package ui

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/listenbrainz"
)

// recsDue is how old the stored Recommendations get before the startup
// check fetches them again; ListenBrainz makes a new list about daily.
const recsDue = 24 * time.Hour

// runRecs runs the effects for Recommendations. The stored list lives in
// the cache directory, under the Subsonic server and user.
func (m *Model) runRecs(e app.Effect) (tea.Cmd, bool) {
	dir, srv, ctx, clock := m.deps.CacheDir, m.deps.Config.Server, m.ctx, m.clock
	switch e := e.(type) {
	case app.LoadRecs:
		return func() tea.Msg {
			if dir == "" {
				return eventMsg{app.RecsLoaded{}}
			}
			st, err := listenbrainz.Load(dir, srv.URL, srv.User)
			if err != nil {
				return eventMsg{app.RecsLoaded{}}
			}
			return eventMsg{app.RecsLoaded{Stored: &st}}
		}, true
	case app.FetchRecs:
		return func() tea.Msg {
			out := app.RecsFetched{User: e.User, API: e.API}
			now := clock.Now()
			if !e.Force && e.Old != nil && now.Sub(e.Old.FetchedAt) < recsDue {
				out.Outcome = app.FetchNotDue
				return eventMsg{out}
			}
			st, changed, err := listenbrainz.New(e.API).Fetch(ctx, e.User, e.Old)
			st.FetchedAt = now
			switch {
			case errors.Is(err, listenbrainz.ErrNoUser):
				out.Outcome = app.FetchNoUser
			case errors.Is(err, listenbrainz.ErrNotGenerated):
				out.Outcome = app.FetchNotGenerated
			case err != nil:
				out.Outcome = app.FetchFailed
				out.Err = err
			case changed:
				out.Outcome, out.Stored = app.FetchNew, st
			default:
				out.Outcome, out.Stored = app.FetchUnchanged, st
			}
			return eventMsg{out}
		}, true
	case app.SaveRecs:
		return func() tea.Msg {
			if dir == "" {
				return nil
			}
			if err := listenbrainz.Save(dir, srv.URL, srv.User, e.Stored); err != nil {
				return eventMsg{app.Failed{What: "couldn't save the Recommendations", Err: err}}
			}
			return nil
		}, true
	case app.DeleteRecs:
		return func() tea.Msg {
			if dir == "" {
				return nil
			}
			if err := listenbrainz.Delete(dir, srv.URL, srv.User); err != nil {
				return eventMsg{app.Failed{What: "couldn't delete the Recommendations", Err: err}}
			}
			return nil
		}, true
	}
	return nil, false
}

// lbOf is the ListenBrainz user and API in effect, with an empty user when
// Recommendations are off.
func lbOf(l config.ListenBrainz) (user, api string) {
	if !l.On() {
		return "", ""
	}
	return l.User, l.API()
}

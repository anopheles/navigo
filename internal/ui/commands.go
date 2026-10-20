package ui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
)

type cmd = command.Command[*Model]

// commands is the registry. Labels are what the hint line shows next to a
// key; Desc is for the key menu and help.
func commands() *command.Registry[*Model] {
	r := command.NewRegistry[*Model]()
	for _, c := range []cmd{
		{Name: "down", Label: "down", Desc: "Move the cursor down.", Motion: true, Run: func(m *Model, c command.Call) error { m.move(c.N()); return nil }},
		{Name: "up", Label: "up", Desc: "Move the cursor up.", Motion: true, Run: func(m *Model, c command.Call) error { m.move(-c.N()); return nil }},
		{Name: "first", Jump: true, Label: "first row", Desc: "Go to the first row, or to row N with a count.", Motion: true, Run: func(m *Model, c command.Call) error { m.goRow(c.Count, 0); return nil }},
		{Name: "last", Jump: true, Label: "last row", Desc: "Go to the last row, or to row N with a count.", Motion: true, Run: func(m *Model, c command.Call) error { m.goRow(c.Count, -1); return nil }},
		{Name: "half-page-down", Label: "half page down", Desc: "Move down half a page.", Motion: true, Run: func(m *Model, c command.Call) error { m.move(c.N() * m.bodyHeight() / 2); return nil }},
		{Name: "half-page-up", Label: "half page up", Desc: "Move up half a page.", Motion: true, Run: func(m *Model, c command.Call) error { m.move(-c.N() * m.bodyHeight() / 2); return nil }},
		{Name: "page-down", Label: "page down", Desc: "Move down a page.", Motion: true, Run: func(m *Model, c command.Call) error { m.page(c, 1); return nil }},
		{Name: "page-up", Label: "page up", Desc: "Move up a page.", Motion: true, Run: func(m *Model, c command.Call) error { m.page(c, -1); return nil }},
		{Name: "scroll-down", Label: "scroll down", Desc: "Scroll the list down a row; the cursor stays unless it would leave the screen.", Motion: true, Run: func(m *Model, c command.Call) error { m.scrollBy(c, 1); return nil }},
		{Name: "scroll-up", Label: "scroll up", Desc: "Scroll the list up a row; the cursor stays unless it would leave the screen.", Motion: true, Run: func(m *Model, c command.Call) error { m.scrollBy(c, -1); return nil }},
		{Name: "screen-top", Label: "top of screen", Desc: "Go to the top row on screen, or the Nth from the top.", Motion: true, Run: func(m *Model, c command.Call) error { m.screenRow(c, -1); return nil }},
		{Name: "screen-middle", Label: "middle of screen", Desc: "Go to the middle row on screen.", Motion: true, Run: func(m *Model, c command.Call) error { m.screenRow(c, 0); return nil }},
		{Name: "screen-bottom", Label: "bottom of screen", Desc: "Go to the bottom row on screen, or the Nth from the bottom.", Motion: true, Run: func(m *Model, c command.Call) error { m.screenRow(c, 1); return nil }},
		{Name: "scroll-center", Label: "cursor to middle", Desc: "Scroll so the cursor row is in the middle of the screen.", Run: func(m *Model, _ command.Call) error { m.scrollTo(0); return nil }},
		{Name: "scroll-top", Label: "cursor to top", Desc: "Scroll so the cursor row is at the top of the screen.", Run: func(m *Model, _ command.Call) error { m.scrollTo(-1); return nil }},
		{Name: "scroll-bottom", Label: "cursor to bottom", Desc: "Scroll so the cursor row is at the bottom of the screen.", Run: func(m *Model, _ command.Call) error { m.scrollTo(1); return nil }},
		{Name: "percent", Label: "percent", Desc: "Go to the row N percent of the way down: 50%.", Motion: true, Run: (*Model).percent},
		{Name: "find", Label: "find", Desc: "Find a row in this list by its text; a capital letter matches case.", Run: func(m *Model, _ command.Call) error { m.startFind(false); return nil }},
		{Name: "find-back", Label: "find back", Desc: "Find a row by its text, searching up the list.", Run: func(m *Model, _ command.Call) error { m.startFind(true); return nil }},
		{Name: "find-next", Jump: true, Label: "next match", Desc: "Go to the next match of the last find.", Motion: true, Run: func(m *Model, c command.Call) error { return m.findAgain(c, false) }},
		{Name: "find-prev", Jump: true, Label: "previous match", Desc: "Go to the previous match of the last find.", Motion: true, Run: func(m *Model, c command.Call) error { return m.findAgain(c, true) }},
		{Name: "noh", Label: "clear highlights", Desc: "Clear the highlights of the last find; n and N still use it.", Run: func(m *Model, _ command.Call) error { m.finding.highlight = false; return nil }},
		{Name: "jump-back", Label: "go back", Desc: "Go back to where you were before the last jump.", Run: (*Model).jumpBack},
		{Name: "forward", Label: "go forward", Desc: "Go forward again in the jump list, after going back.", Run: (*Model).jumpForward},
		{Name: "go-playing", Jump: true, Label: "playing", Desc: "Go to the entry that plays, in the queue.", Run: (*Model).goPlaying},
		{Name: "next-group", Label: "next group", Desc: "Go to the start of the next group, such as an artist's Appears on albums.", Motion: true, Run: func(m *Model, c command.Call) error { m.group(1, c.N()); return nil }},
		{Name: "prev-group", Label: "previous group", Desc: "Go to the start of this group, or of the one before.", Motion: true, Run: func(m *Model, c command.Call) error { m.group(-1, c.N()); return nil }},

		{Name: "open", Label: "open", Desc: "Go into the row under the cursor.", Run: (*Model).open},
		{Name: "back", Label: "back", Desc: "Go back out to the list you came from.", Run: func(m *Model, _ command.Call) error { m.back(); return nil }},
		{Name: "go-album", Jump: true, Label: "album", Desc: "Go to the album of the track or album under the cursor.", Run: (*Model).goAlbum},
		{Name: "go-artist", Jump: true, Label: "artist", Desc: "Go to the artist of the track or album under the cursor.", Run: (*Model).goArtist},
		{Name: "home", Jump: true, Label: "home", Desc: "Show Home: Mix, then recently added, most and recently played, random and starred albums.", Run: func(m *Model, _ command.Call) error { m.showSection(homeList); return nil }},
		{Name: "artists", Jump: true, Label: "artists", Desc: "Show the Artists section.", Run: func(m *Model, _ command.Call) error { m.showSection(artistList); return nil }},
		{Name: "albums", Jump: true, Label: "albums", Desc: "Show the Albums section.", Run: func(m *Model, _ command.Call) error { m.showSection(albumList); return nil }},
		{Name: "playlists", Jump: true, Label: "playlists", Desc: "Show the Playlists section.", Run: func(m *Model, _ command.Call) error { m.showSection(playlistList); return nil }},
		{Name: "playlist-add", Args: someArgs, Label: "add to playlist", Desc: "Add the selection or the row under the cursor to the end of a playlist: playlist-add Rock.", Run: (*Model).playlistAdd, Complete: (*Model).playlistNames},
		{Name: "playlist-new", Args: someArgs, Label: "new playlist", Desc: "Make a playlist of the selection or the row under the cursor: playlist-new Rock.", Run: (*Model).playlistNew},
		{Name: "playlist-delete", Label: "delete playlist", Desc: "Delete the playlist under the cursor, after asking.", Run: (*Model).deletePlaylist},
		{Name: "playlist-undo", Label: "undo", Desc: "Explain that playlist changes can't be undone, and how to put a removal back.", Run: (*Model).playlistUndo},
		{Name: "search", Jump: true, Args: anyArgs, Label: "search", Desc: "Search the library, or type the text to search for: search blue hour.", Run: (*Model).search, Complete: func(m *Model) []string {
			s := slices.Clone(m.searchHist.items)
			slices.Reverse(s) // newest first
			return s
		}},
		{Name: "queue", Jump: true, Label: "queue", Desc: "Move to the queue window.", Run: func(m *Model, _ command.Call) error { m.focus = queueWindow; return nil }},
		{Name: "window", Label: "other window", Desc: "Move to the other window.", Run: func(m *Model, _ command.Call) error { m.focus = 1 - m.focus; return nil }},

		{Name: "play-now", Label: "play", Desc: "Replace the queue with this list and play from the track under the cursor.", Run: (*Model).playNow},
		{Name: "add-end", Label: "add to queue", Desc: "Add the track under the cursor to the end of the queue.", Run: func(m *Model, c command.Call) error { return m.add(c, false) }},
		{Name: "add-next", Label: "play next", Desc: "Add the track under the cursor after the current track.", Run: func(m *Model, c command.Call) error { return m.add(c, true) }},
		{Name: "play-row", Label: "play", Desc: "Play the queue row under the cursor.", Run: func(m *Model, _ command.Call) error { m.event(app.PlayRow{Index: m.queueCur}); return nil }},
		{Name: "play-pause", Label: "play/pause", Desc: "Pause or resume playback, or start the queue.", Run: func(m *Model, _ command.Call) error { m.event(app.PlayPause{}); return nil }},
		{Name: "next-track", Label: "next track", Desc: "Play the next track in the queue.", Run: func(m *Model, _ command.Call) error { m.event(app.NextTrack{}); return nil }},
		{Name: "prev-track", Label: "previous track", Desc: "Play the previous track, or restart this one after 3 seconds.", Run: func(m *Model, _ command.Call) error { m.event(app.PrevTrack{}); return nil }},
		{Name: "seek", Args: seekArgs, Label: "seek", Desc: "Seek by a number of seconds, such as seek +10; a count multiplies it.", Run: seek},
		{Name: "reconnect", Label: "reconnect", Desc: "Try the server now instead of waiting for the next retry.", Run: func(m *Model, _ command.Call) error { m.event(app.Reconnect{}); return nil }},
		{Name: "sync", Label: "sync", Desc: "Sync the whole library from the server.", Run: func(m *Model, _ command.Call) error { m.event(app.SyncNow{}); return nil }},
		{Name: "volume", Args: volumeArgs, Label: "volume", Desc: "Change the volume by points, such as volume +5, or set it: volume 60.", Run: volume},
		{Name: "replaygain", Args: replaygainArgs, Label: "ReplayGain mode", Desc: "Set the ReplayGain mode for this session: replaygain off|track|album; alone, move to the next one.", Run: replaygain, Complete: gainWords},
		{Name: "mix", Label: "mix", Desc: "Replace the queue with 50 tracks from the whole library, or as many as a count says, leaning toward starred, well-rated tracks not played lately; u undoes it.", Run: func(m *Model, c command.Call) error { m.mix(c); return nil }},
		{Name: "shuffle", Label: "shuffle", Desc: "Shuffle the queue once, keeping the current track first; u undoes it.", Run: func(m *Model, _ command.Call) error { m.event(app.Shuffle{Seed: m.seed()}); return nil }},
		{Name: "repeat", Label: "repeat", Desc: "Cycle repeat: off, all (the queue again), one (the same track again).", Run: func(m *Model, _ command.Call) error { m.event(app.CycleRepeat{}); return nil }},
		{Name: "info", Label: "info", Desc: "Show details of the track, album or artist under the cursor, or of what plays.", Run: (*Model).info},
		{Name: "star", Label: "star", Desc: "Star the track, album or artist under the cursor, or the selection; again unstars.", Run: (*Model).star},
		{Name: "stop", Label: "stop", Desc: "Stop playback.", Run: func(m *Model, _ command.Call) error { m.event(app.StopPlayback{}); return nil }},
		{Name: "queue-clear", Label: "clear queue", Desc: "Empty the queue; what plays plays on to its end. u brings it back.", Run: func(m *Model, _ command.Call) error { m.event(app.ClearQueue{}); return nil }},
		{Name: "visual", Label: "select", Desc: "Select rows: start Visual mode, or end it.", Run: func(m *Model, _ command.Call) error { m.startVisual(); return nil }},
		{Name: "yank", Label: "yank", Desc: "Yank the rows a motion covers (yy: this row) into the register.", Operator: true, Run: (*Model).yank},
		{Name: "delete", Label: "remove", Desc: "Remove the queue rows a motion covers (dd: this row) into the register.", Operator: true, Run: (*Model).remove},
		{Name: "remove", Label: "remove", Desc: "Remove the queue row under the cursor, or N rows, into the register.", Run: (*Model).remove},
		{Name: "paste-after", Label: "paste", Desc: "Paste the register into the queue after the cursor row.", Run: func(m *Model, c command.Call) error { return m.paste(c, true) }},
		{Name: "paste-before", Label: "paste before", Desc: "Paste the register into the queue before the cursor row.", Run: func(m *Model, c command.Call) error { return m.paste(c, false) }},
		{Name: "move-down", Label: "move down", Desc: "Move the queue row or the selection down.", Run: func(m *Model, c command.Call) error { return m.moveRows(c, true) }},
		{Name: "move-up", Label: "move up", Desc: "Move the queue row or the selection up.", Run: func(m *Model, c command.Call) error { return m.moveRows(c, false) }},
		{Name: "undo", Label: "undo", Desc: "Undo the last change to the queue.", Run: func(m *Model, _ command.Call) error { m.event(app.Undo{}); return nil }},
		{Name: "redo", Label: "redo", Desc: "Redo the last undone change to the queue.", Run: func(m *Model, _ command.Call) error { m.event(app.Redo{}); return nil }},

		{Name: "cmdline", Label: "command", Desc: "Type a command.", Run: func(m *Model, _ command.Call) error { m.enterLine(); return nil }},
		{Name: "cancel", Label: "cancel", Desc: "Cancel pending keys and the selection, and clear the message.", Run: func(m *Model, _ command.Call) error {
			m.endVisual()
			m.clearMsg()
			m.finding.highlight = false
			return nil
		}},
		{Name: "close", Label: "close", Desc: "Close the popup.", Run: func(m *Model, _ command.Call) error { m.closePopup(); return nil }},
		{Name: "choose", Label: "choose", Desc: "Choose the popup row under the cursor.", Run: func(m *Model, _ command.Call) error { return m.choose() }},
		{Name: "sort", Args: anyArgs, Label: "sort", Desc: "Sort this list by a field, such as sort year desc; alone, put the order back.", Run: (*Model).sortList, Complete: (*Model).sortWords},
		{Name: "filter", Args: anyArgs, Label: "filter", Desc: "Show only rows of a genre or years: filter genre rock, filter year 1990-1999; alone, show all.", Run: (*Model).filterList, Complete: (*Model).filterWords},
		{Name: "hints", Args: onOff, Label: "hint line", Desc: "Turn the hint line on or off for this session: hints off.", Run: (*Model).setHints},
		{Name: "logout", Label: "log out", Desc: "Forget the saved password and go back to the login form.", Run: (*Model).logout},
		{Name: "nop", Label: "nothing", Desc: "Do nothing: bind it to make a dead key.", Run: func(*Model, command.Call) error { return nil }},
		{Name: "nmap", Args: anyArgs, Label: "map", Desc: "Bind keys in Normal mode for this session: nmap [--in queue] <leader>x seek +60.", Run: sessionMapIn("normal")},
		{Name: "vmap", Args: anyArgs, Label: "map", Desc: "Bind keys in Visual mode for this session: vmap [--in scope] keys command.", Run: sessionMapIn("visual")},
		{Name: "cmap", Args: anyArgs, Label: "map", Desc: "Bind a key on the command line for this session: cmap <C-k> kill-to-end.", Run: sessionMapIn("cmdline")},
		{Name: "nunmap", Args: anyArgs, Label: "unmap", Desc: "Unbind keys in Normal mode for this session: nunmap [--in scope] keys.", Run: sessionUnmapIn("normal")},
		{Name: "vunmap", Args: anyArgs, Label: "unmap", Desc: "Unbind keys in Visual mode for this session: vunmap [--in scope] keys.", Run: sessionUnmapIn("visual")},
		{Name: "cunmap", Args: anyArgs, Label: "unmap", Desc: "Unbind a command-line key for this session: cunmap <C-k>.", Run: sessionUnmapIn("cmdline")},
		{Name: "map", Label: "bindings", Desc: "List the bindings that differ from the defaults.", Run: (*Model).showMap},
		{Name: "reload-config", Label: "reload config", Desc: "Read config.yaml again and rebuild the bindings, dropping this session's mappings.", Run: (*Model).reloadConfig},
		{Name: "help", Jump: true, Label: "help", Desc: "Show every key and command, from the bindings in effect.", Run: func(m *Model, _ command.Call) error { m.showHelp(); return nil }},
		{Name: "help-filter", Label: "filter", Desc: "Filter Help to the keys and commands that match what you type.", Run: func(m *Model, _ command.Call) error { m.startHelpFilter(); return nil }},
		{Name: "cover", Label: "cover", Desc: "Show the playing track's cover art large.", Run: func(m *Model, _ command.Call) error { return m.openCover() }},
		{Name: "key-menu", Label: "keys", Desc: "Show the keys for what is under the cursor; press one to run it.", Run: func(m *Model, _ command.Call) error { m.openKeyMenu(); return nil }},
		{Name: "messages", Label: "messages", Desc: "Show the messages of this session.", Run: func(m *Model, _ command.Call) error { m.showMessages(); return nil }},
		{Name: "interrupt", Label: "cancel", Desc: "Like cancel; reminds you how to quit.", Run: func(m *Model, _ command.Call) error { m.setMsg("type :q to quit", false); return nil }},
		{Name: "login", Label: "log in", Desc: "Log in to a server, or as another user.", Run: func(m *Model, _ command.Call) error {
			s := m.deps.Config.Server
			m.openLogin(s.URL, s.User, "")
			return nil
		}},
		{Name: "quit", Aliases: []string{"q", "qa", "q!", "qa!"}, Label: "quit", Desc: "Quit navigo.", Run: func(m *Model, _ command.Call) error { m.then(tea.Quit); return nil }},
	} {
		r.Add(c)
	}
	return r
}

// then queues a tea.Cmd to run after the current command.
func (m *Model) then(c tea.Cmd) { m.pendingCmd = tea.Batch(m.pendingCmd, c) }

// event sends an event to the core from a command.
func (m *Model) event(e app.Event) { m.then(m.dispatch(e)) }

func (m *Model) move(d int) {
	if m.popup != nil {
		m.scrollPopup(d)
		return
	}
	if m.focus == queueWindow {
		m.queueCur = min(max(m.queueCur+d, 0), max(len(m.state.Queue)-1, 0))
		return
	}
	f := m.top()
	f.cur = min(max(f.cur+d, 0), max(m.rows(f)-1, 0))
	m.settle(f, d)
}

// goRow moves to row count (1-based) when a count was typed, else to def
// (-1 for the last row).
func (m *Model) goRow(count, def int) {
	if m.popup != nil {
		m.popup.off = 0
		if def < 0 {
			m.scrollPopup(len(m.popup.lines))
		}
		return
	}
	n := m.rows(m.top())
	if m.focus == queueWindow {
		n = len(m.state.Queue)
	}
	row := def
	if count > 0 {
		row = count - 1
	} else if def < 0 {
		row = n - 1
	}
	row = min(max(row, 0), max(n-1, 0))
	if m.focus == queueWindow {
		m.queueCur = row
	} else {
		m.top().cur = row
		dir := 1
		if def < 0 {
			dir = -1
		}
		m.settle(m.top(), dir)
	}
}

func (m *Model) open(c command.Call) error {
	f := m.top()
	switch f.kind {
	case sectionList:
		switch sections[f.cur].kind {
		case searchList:
			m.startSearch()
		case helpList:
			m.showHelp()
		default:
			m.showSection(sections[f.cur].kind)
		}
	case artistList:
		if lib := m.state.Library; lib != nil && f.cur < len(lib.Artists) {
			m.openArtist(m.artistsOf(f)[f.cur])
		}
	case homeList:
		m.openHomeList(f.cur, c)
	case albumList, artistPage, homeAlbums:
		if a, ok := m.rowAlbum(); ok {
			m.openAlbum(a, f.artist)
		}
	case playlistList:
		if lib := m.state.Library; lib != nil && f.cur < len(lib.Playlists) {
			p := lib.Playlists[f.cur]
			m.lib = append(m.lib, frame{kind: playlistTracks, playlist: p.ID, title: p.Name})
		}
	case searchList:
		v := f.search
		if a, ok := v.artist(f.cur); ok {
			m.openArtist(a)
		} else if a, ok := v.album(f.cur); ok {
			m.openAlbum(a, "")
		} else if p, ok := v.playlist(f.cur); ok {
			m.lib = append(m.lib, frame{kind: playlistTracks, playlist: p.ID, title: p.Name})
		} else if r := v.row(f.cur); r.more > 0 {
			v.expanded[r.group] = true
			v.layout()
		}
	}
	return nil
}

func (m *Model) back() {
	if m.focus == libraryWindow && len(m.lib) > 1 {
		m.lib = m.lib[:len(m.lib)-1]
	}
}

// trackRow returns the album of the track under the cursor in the library
// window, and the track's row in it. An album still loading has no rows
// yet, which is not an error.
func (m *Model) trackRow() ([]library.Track, int, error) {
	f := m.top()
	lib := m.state.Library
	if m.focus == libraryWindow && f.kind == searchList {
		t, ok := f.search.track(f.cur)
		if !ok || lib == nil {
			return nil, 0, errNotOnTrack
		}
		ts := lib.AlbumTracks(t.AlbumID)
		if i := slices.IndexFunc(ts, func(o library.Track) bool { return o.ID == t.ID }); i >= 0 {
			return ts, i, nil
		}
		return []library.Track{t}, 0, nil
	}
	if m.focus != libraryWindow || f.kind != albumTracks && f.kind != playlistTracks {
		return nil, 0, errNotOnTrack
	}
	var ts []library.Track
	switch {
	case lib == nil:
	case f.kind == albumTracks:
		ts = m.tracksOf(f)
	default:
		ts = lib.PlaylistTracks(f.playlist)
	}
	if f.cur >= len(ts) {
		return nil, 0, errNoRows
	}
	return ts, f.cur, nil
}

var (
	errNotOnTrack = errors.New("not on a track")
	errNoRows     = errors.New("no rows yet")
)

func (m *Model) playNow(command.Call) error {
	ts, i, err := m.trackRow()
	if err != nil {
		return quiet(err)
	}
	m.event(app.PlayNow{Tracks: ts, Start: i})
	return nil
}

// add adds the selection, or the track or album under the cursor, count
// times. Artist rows are left out with a note.
func (m *Model) add(c command.Call, next bool) error {
	if m.focus == queueWindow {
		m.endVisual()
		return errors.New("those are in the queue already · y and p copy them")
	}
	if _, _, err := m.trackRow(); errors.Is(err, errNoRows) {
		return nil
	}
	one, artists, err := m.chosenTracks()
	if err != nil {
		return err
	}
	defer m.skippedArtists(artists)
	var add []library.Track
	for range c.N() {
		add = append(add, one...)
	}
	if next {
		m.event(app.AddNext{Tracks: add})
	} else {
		m.event(app.AddEnd{Tracks: add})
	}
	return nil
}

// quiet drops errNoRows: a key pressed before the list has loaded does
// nothing.
func quiet(err error) error {
	if errors.Is(err, errNoRows) {
		return nil
	}
	return err
}

func seek(m *Model, c command.Call) error {
	if len(c.Args) != 1 {
		return errors.New("seek: give seconds, such as seek +10")
	}
	d, err := seconds(c.Args[0])
	if err != nil {
		return err
	}
	m.event(app.SeekBy{D: d * time.Duration(c.N())})
	return nil
}

func (m *Model) enterLine() {
	m.mode = keymap.Cmdline // a Visual selection stays for the command
	m.line.Reset()
	m.line.Prompt = ":"
	m.fitLine()
	m.cmdHist.begin(false)
	m.then(m.line.Focus())
}

// fitLine sizes the line editor to the status line after its prompt.
func (m *Model) fitLine() { m.line.SetWidth(max(m.w-lipgloss.Width(m.line.Prompt)-1, 1)) }

// onTrack reports whether the cursor is on a track in the library
// window, or in an album whose tracks are still loading.
func (m *Model) onTrack() bool {
	_, _, err := m.trackRow()
	return err == nil || errors.Is(err, errNoRows)
}

// Argument checks for binding commands to keys.
func anyArgs([]string) error { return nil }

func someArgs(a []string) error {
	if len(a) == 0 {
		return errors.New("give it a name")
	}
	return nil
}

func seekArgs(a []string) error {
	if len(a) != 1 {
		return errors.New("seek: give seconds, such as seek +10")
	}
	_, err := seconds(a[0])
	return err
}

func volumeArgs(a []string) error {
	if len(a) != 1 {
		return errors.New("volume: give points, such as volume +5, or a level: volume 60")
	}
	if _, err := strconv.Atoi(strings.TrimPrefix(a[0], "+")); err != nil {
		return fmt.Errorf("volume: %q is not a number", a[0])
	}
	return nil
}

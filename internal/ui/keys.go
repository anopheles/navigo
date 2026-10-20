package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/keymap"
)

// scopeNames are the scopes a binding can name.
var scopeNames = []string{
	"global",
	"popup",
	"sections",
	"home",
	"artists",
	"albums",
	"playlists",
	"containers",
	"album-tracks",
	"playlist-tracks",
	"tracks",
	"queue",
	"help",
	"search",
}

// lineCommands are what Command-line, Search and Prompt keys run: line
// editing, with what Help says of them.
var lineCommands = map[string]string{
	"accept":          "Run the command line, or accept the text.",
	"cancel":          "Leave the line without running it.",
	"complete":        "Complete the command name or argument; again for the next.",
	"complete-back":   "Go back to the previous completion.",
	"history-back":    "Show the line before this one in the history.",
	"history-forward": "Show the next line in the history.",
	"line-start":      "Move to the start of the line.",
	"line-end":        "Move to the end of the line.",
	"kill-word":       "Delete the word before the cursor.",
	"kill-to-start":   "Delete everything before the cursor.",
	"kill-to-end":     "Delete everything after the cursor.",
	"nop":             "Do nothing: a dead key.",
}

// keySettings are the keys: block's options.
type keySettings struct {
	timeout     time.Duration // before an ambiguous prefix runs its shorter binding
	hintDelay   time.Duration // before a prefix menu shows
	prefixMenus bool
	leader      string
}

func settingsFrom(k config.Keys) keySettings {
	s := keySettings{
		timeout:     time.Second,
		hintDelay:   500 * time.Millisecond,
		prefixMenus: k.HintsOn(),
		leader:      k.LeaderKey(),
	}
	if k.Timeout > 0 {
		s.timeout = time.Duration(k.Timeout) * time.Millisecond
	}
	if k.HintDelay > 0 {
		s.hintDelay = time.Duration(k.HintDelay) * time.Millisecond
	}
	return s
}

// errNoQuit is why navigo won't start with a table that can't quit.
//
//nolint:staticcheck // ST1005: the message ends in a keys.yaml path
var errNoQuit = errors.New(
	"no key quits navigo: bind quit, or cmdline for :q, in keys: normal: global:",
)

// buildKeys makes the binding table: the defaults unless defaults: false,
// then each binding of the keys: block. A bad binding is skipped and
// becomes a problem at its place. errNoQuit means the table must not be
// used.
func buildKeys(
	k config.Keys,
	reg *command.Registry[*Model],
) (*keymap.Table, keySettings, []config.Problem, error) {
	t := keymap.NewTable()
	if k.DefaultsOn() {
		t = defaultTable()
	}
	set := settingsFrom(k)
	problems := slices.Clone(k.Problems)
	type place struct{ mode, scope, seq string }
	seen := map[place]config.Pos{}
	for _, b := range k.Bindings {
		scope := b.Scope
		if b.Mode == "cmdline" {
			scope = "global"
		}
		if tokens, err := keymap.Parse(expandLeader(b.Keys, set.leader)); err == nil {
			p := place{b.Mode, scope, strings.Join(tokens, "")}
			if first, dup := seen[p]; dup {
				problems = append(
					problems,
					config.Problem{
						Pos: b.Pos,
						Msg: fmt.Sprintf("%s is bound already, at line %d", b.Keys, first.Line),
					},
				)
				continue
			}
			seen[p] = b.Pos
		}
		if err := bindOne(
			t,
			reg,
			set.leader,
			b.Mode,
			b.Scope,
			b.Keys,
			b.Command,
			b.Delete,
		); err != nil {
			problems = append(problems, config.Problem{Pos: b.Pos, Msg: err.Error()})
		}
	}
	slices.SortStableFunc(problems, func(a, b config.Problem) int { return a.Line - b.Line })
	if !canQuit(t) {
		return nil, set, problems, errNoQuit
	}
	return t, set, problems, nil
}

// canQuit reports whether some key quits from anywhere: quit itself, or
// : for :q.
func canQuit(t *keymap.Table) bool {
	return slices.ContainsFunc(
		t.Effective(keymap.Normal, []string{"global"}),
		func(b keymap.Binding) bool {
			return b.Name() == "quit" || b.Name() == "cmdline"
		},
	)
}

// expandLeader writes the leader for <leader>, in any case.
func expandLeader(seq, leader string) string {
	var out strings.Builder
	for {
		i := strings.Index(strings.ToLower(seq), "<leader>")
		if i < 0 {
			out.WriteString(seq)
			return out.String()
		}
		out.WriteString(seq[:i] + leader)
		seq = seq[i+len("<leader>"):]
	}
}

// bindOne checks one binding and applies it to t: mode, scope, keys,
// then the command and its arguments; del removes it instead.
func bindOne(
	t *keymap.Table,
	reg *command.Registry[*Model],
	leader, mode, scope, keys, line string,
	del bool,
) error {
	var m keymap.Mode
	switch mode {
	case "normal":
		m = keymap.Normal
	case "visual":
		m = keymap.Visual
	case "cmdline":
		m, scope = keymap.Cmdline, "global"
	default:
		return fmt.Errorf("unknown mode %q; modes are normal, visual and cmdline", mode)
	}
	if !slices.Contains(scopeNames, scope) {
		return fmt.Errorf("unknown scope %q; scopes are %s", scope, strings.Join(scopeNames, ", "))
	}
	seq := expandLeader(keys, leader)
	if err := keymap.Check(seq); err != nil {
		return err
	}
	tokens, err := keymap.Parse(seq)
	if err != nil {
		return err
	}
	switch {
	case tokens[0] == `"`:
		return fmt.Errorf(
			`%s: keys can't start with ", which names a register; navigo has one`,
			keys,
		)
	case m == keymap.Cmdline && len(tokens) > 1:
		return fmt.Errorf("%s: command-line bindings are single keys", keys)
	}
	where := mode + "." + scope
	if m == keymap.Cmdline {
		where = mode
	}
	if del {
		if ok, _ := t.Unbind(m, scope, seq); !ok {
			return fmt.Errorf("%s isn't bound in %s, so there is nothing to delete", keys, where)
		}
		return nil
	}

	call, err := command.Parse(line)
	if err != nil {
		return fmt.Errorf("%s: %w", keys, err)
	}
	if m == keymap.Cmdline {
		if _, ok := lineCommands[call.Name]; !ok {
			return fmt.Errorf(
				"%q isn't a command-line command; they are %s",
				call.Name,
				strings.Join(slices.Sorted(mapKeys(lineCommands)), ", "),
			)
		}
		if len(call.Args) > 0 {
			return fmt.Errorf("%s takes no arguments", call.Name)
		}
		return t.Bind(m, scope, seq, call.Name)
	}
	c, ok := reg.Get(call.Name)
	if !ok {
		if _, line := lineCommands[call.Name]; line {
			//nolint:staticcheck // ST1005: the message ends in a keys.yaml key
			return fmt.Errorf("%s edits the command line: bind it under cmdline:", call.Name)
		}
		return fmt.Errorf("unknown command %q", call.Name)
	}
	switch {
	case c.Args == nil && len(call.Args) > 0:
		return fmt.Errorf("%s takes no arguments", c.Name)
	case c.Args != nil:
		if err := c.Args(call.Args); err != nil {
			return err
		}
	}
	return t.Bind(m, scope, seq, strings.Join(append([]string{c.Name}, call.Args...), " "))
}

func mapKeys[V any](m map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// CheckConfig checks a config's bindings as navigo would at startup, for
// navigo check-config: the problems, each as path:LINE:COL: message, and
// errNoQuit when navigo would refuse to start.
func CheckConfig(cfg config.Config, path string) ([]string, error) {
	_, _, problems, err := buildKeys(cfg.Keys, commands())
	var out []string
	for _, p := range append(problems, cfg.Problems()...) {
		out = append(out, p.Format(path))
	}
	return out, err
}

// showProblems lists the keys: block's problems in a popup.
func (m *Model) showProblems(problems []config.Problem) {
	if len(problems) == 0 {
		return
	}
	lines := make([]string, len(problems))
	for i, p := range problems {
		lines[i] = errStyle.Render(p.Format(m.deps.ConfigPath))
	}
	lines = append(
		lines,
		"",
		dimStyle.Render(
			"These settings were skipped; the rest apply. :reload-config reads the file again.",
		),
	)
	m.openPopup("config.yaml has problems", lines)
}

// setTable puts a new table in use.
func (m *Model) setTable(t *keymap.Table) {
	m.engine = keymap.NewEngine(t)
	m.engine.SetKinds(keymap.Kinds{
		Operator: func(n string) bool { c, ok := m.registry.Get(n); return ok && c.Operator },
		Motion:   func(n string) bool { c, ok := m.registry.Get(n); return ok && c.Motion },
	})
}

// sessionMapIn is :nmap, :vmap and :cmap: a binding for this session only.
func sessionMapIn(mode string) func(*Model, command.Call) error {
	return func(m *Model, c command.Call) error {
		scope, args, err := inScope(c.Name, mode, c.Args)
		if err != nil {
			return err
		}
		if len(args) < 2 {
			return fmt.Errorf(
				"%s: give keys and a command, such as %s <leader>x seek +60",
				c.Name,
				c.Name,
			)
		}
		line := strings.Join(args[1:], " ")
		return m.changeTable(func(t *keymap.Table) error {
			return bindOne(t, m.registry, m.keys.leader, mode, scope, args[0], line, false)
		}, fmt.Sprintf("%s now runs %s (this session)", args[0], line))
	}
}

// sessionUnmapIn is :nunmap, :vunmap and :cunmap.
func sessionUnmapIn(mode string) func(*Model, command.Call) error {
	return func(m *Model, c command.Call) error {
		scope, args, err := inScope(c.Name, mode, c.Args)
		if err != nil {
			return err
		}
		if len(args) != 1 {
			return fmt.Errorf("%s: give the keys, such as %s J", c.Name, c.Name)
		}
		return m.changeTable(func(t *keymap.Table) error {
			return bindOne(t, m.registry, m.keys.leader, mode, scope, args[0], "", true)
		}, fmt.Sprintf("%s unbound (this session)", args[0]))
	}
}

// inScope takes --in <scope> off the arguments; global by default.
func inScope(name, mode string, args []string) (string, []string, error) {
	if len(args) >= 1 && args[0] == "--in" {
		if mode == "cmdline" {
			return "", nil, fmt.Errorf("%s: the command line has no scopes", name)
		}
		if len(args) < 2 {
			return "", nil, fmt.Errorf("%s: --in needs a scope, such as --in queue", name)
		}
		return args[1], args[2:], nil
	}
	return "global", args, nil
}

// changeTable changes a copy of the table and uses it, unless the change
// fails or leaves no way to quit.
func (m *Model) changeTable(change func(*keymap.Table) error, done string) error {
	t := m.engine.Table().Clone()
	if err := change(t); err != nil {
		return err
	}
	if !canQuit(t) {
		return errors.New("that leaves no key to quit, so nothing changed")
	}
	m.setTable(t)
	m.setMsg(done, false)
	return nil
}

// reloadConfig is :reload-config: read config.yaml again and rebuild the
// table, which drops the session's mappings.
func (m *Model) reloadConfig(command.Call) error {
	cfg, err := config.Load(m.deps.ConfigPath)
	if err != nil {
		return err
	}
	t, set, problems, err := buildKeys(cfg.Keys, m.registry)
	if err != nil {
		return fmt.Errorf("%s: %w; the old bindings stay", m.deps.ConfigPath, err)
	}
	m.deps.Config.Keys, m.keys = cfg.Keys, set
	m.deps.Config.Player, m.deps.Config.Resume = cfg.Player, cfg.Resume
	m.setTable(t)
	m.event(app.ConfigureReplayGain{ReplayGain: gainOf(cfg.Player)})
	m.event(app.ConfigureResume{On: cfg.ResumeOn()})
	m.setMsg("config reloaded", false)
	m.showProblems(append(problems, cfg.Problems()...))
	return nil
}

// showMap is :map: the bindings that differ from the defaults.
func (m *Model) showMap(command.Call) error {
	cur, def := m.engine.Table(), defaultTable()
	var lines []string
	for _, mode := range []keymap.Mode{keymap.Normal, keymap.Visual, keymap.Cmdline} {
		scopes := slices.Compact(
			slices.Sorted(slices.Values(append(cur.Scopes(mode), def.Scopes(mode)...))),
		)
		for _, scope := range scopes {
			where := string(mode) + "." + scope
			if mode == keymap.Cmdline {
				where = string(mode)
			}
			now, was := cur.Bindings(mode, scope), def.Bindings(mode, scope)
			for _, b := range now {
				if !slices.ContainsFunc(
					was,
					func(o keymap.Binding) bool { return o.Seq() == b.Seq() && o.Command == b.Command },
				) {
					lines = append(
						lines,
						fmt.Sprintf(
							"%-22s %s  %s",
							where,
							keyStyle.Render(fit(keyLabel(b.Seq()), 10)),
							b.Command,
						),
					)
				}
			}
			for _, b := range was {
				if !slices.ContainsFunc(
					now,
					func(o keymap.Binding) bool { return o.Seq() == b.Seq() },
				) {
					lines = append(
						lines,
						fmt.Sprintf(
							"%-22s %s  %s",
							where,
							keyStyle.Render(fit(keyLabel(b.Seq()), 10)),
							dimStyle.Render("deleted (was "+b.Command+")"),
						),
					)
				}
			}
		}
	}
	if len(lines) == 0 {
		lines = []string{
			dimStyle.Render("no bindings differ from the defaults · '? lists them all"),
		}
	}
	m.openPopup("Bindings changed from the defaults", lines)
	return nil
}

// lineBound is the line command bound to k, if any.
func (m *Model) lineBound(k tea.KeyPressMsg) (string, bool) {
	tok := keymap.FromTerminal(k.String(), k.Text)
	for _, b := range m.engine.Table().Effective(keymap.Cmdline, []string{"global"}) {
		if len(b.Keys) == 1 && b.Keys[0] == tok {
			return b.Command, true
		}
	}
	return "", false
}

// editLine runs a line command that edits the text or moves the cursor.
func (m *Model) editLine(action string) {
	before := m.line.Value()
	if m.selected { // a selected query goes as a whole
		m.selected = false
		if strings.HasPrefix(action, "kill") {
			m.line.SetValue("")
			m.lineChanged(before)
			return
		}
	}
	v, p := []rune(before), m.line.Position()
	switch action {
	case "line-start":
		m.line.CursorStart()
	case "line-end":
		m.line.CursorEnd()
	case "kill-to-start":
		m.line.SetValue(string(v[p:]))
		m.line.SetCursor(0)
	case "kill-to-end":
		m.line.SetValue(string(v[:p]))
	case "kill-word":
		i := p
		for i > 0 && v[i-1] == ' ' {
			i--
		}
		for i > 0 && v[i-1] != ' ' {
			i--
		}
		m.line.SetValue(string(v[:i]) + string(v[p:]))
		m.line.SetCursor(i)
	}
	m.lineChanged(before)
}

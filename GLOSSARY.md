# navigo

A keyboard-driven terminal client for Navidrome/Subsonic music servers. These are the words navigo uses for its own concepts.

## Input

**Mode**:
One of Normal, Visual, Command-line, Search or Prompt; it decides which bindings apply to a key press.
_Avoid_: state, insert mode

**Command**:
A named action the user can type at the `:` line, such as `play-pause` or `seek +10`. Every key does exactly one command.
_Avoid_: action, shortcut

**Binding**:
A key sequence paired with a command line, within one scope.
_Avoid_: shortcut, keymap entry, mapping

**Key sequence**:
One or more keys typed in order that together match a binding, such as `gg` or `<C-d>`.
_Avoid_: combo, chord

**Pure prefix**:
A key sequence that only begins longer bindings and is not bound on its own, so navigo waits for the next key without a time limit.

**Ambiguous prefix**:
A key sequence that is bound on its own and also begins a longer binding; it resolves after the timeout.

**Motion**:
A command that moves the cursor through a list and can follow an operator, such as `j`, `G` or `}`.

**Operator**:
A command that acts on the rows a motion covers, or on the current row when doubled. In v1 the operators are `d` (remove) and `y` (yank).

**Count**:
A number typed before a motion or command that repeats or scales it, such as `5j` or `3.`.

**Scope**:
Where a binding applies: a popup, a kind of list, or the whole mode. Scopes form a chain from narrow to wide (a playlist's tracks, then any track list, then global), and the narrowest scope that matches wins.

**Leader**:
A user-configurable prefix key (`\` by default) that is free for the user's own bindings.

**Register**:
The single clipboard that `y` and `d` fill and `p`/`P` paste from, shared by every view.
_Avoid_: clipboard, buffer

**Confirmation**:
A y/n question asked before a destructive server change; its keys are fixed and cannot be rebound.

**Jump list**:
The history of where the user has navigated, walked back and forth with `<C-o>`/`<C-i>`.

## Lists

**Container list**:
A list of things that hold tracks (artists, albums, playlists); you go into a row with `l` or `<CR>` and come back out with `h`.

**Track list**:
A list of individual tracks, such as an album's or a playlist's tracks.
_Avoid_: song list

**Queue**:
The ordered tracks navigo will play, edited like a vim buffer.
_Avoid_: playlist (a playlist lives on the server), now playing list

**Group**:
A run of consecutive tracks from the same album in a track list or the queue; `{`/`}` jump between groups.

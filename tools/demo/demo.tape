# Records the README's demo: tools/demo/demo.sh runs this in Docker, against
# a throwaway Navidrome serving the synthetic fixture library with generated
# covers. Nothing here is a real library.
Output demo.gif

Set Shell bash
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set FontSize 15
Set Width 1360
Set Height 780
Set Padding 8
Set Theme "GruvboxDark"
Set TypingSpeed 140ms
Set Framerate 15

Env COLORTERM truecolor
Env TERM xterm-256color

# The first run syncs the library; do it out of sight, so the recording
# starts on a library that opens at once, as the second run does.
Hide
Type "navigo"
Enter
Sleep 12s
Type ":q"
Enter
Sleep 1s
Type "clear"
Enter
Show

Type "navigo"
Enter
Sleep 3s

# Albums, open one, play a track: its cover heads the queue window.
Type "'b"
Sleep 2s
Type "jjj"
Sleep 1s
Enter
Sleep 2s
Type "j"
Sleep 500ms
Enter
Sleep 4s

# The cover large, then seek forward and back.
Type "gc"
Sleep 3s
Type "q"
Sleep 1s
Type "..."
Sleep 2s
Type ","
Sleep 2s

# Select a few rows in Visual mode, then find in the list, then the keys
# that apply here.
Type "Vjj"
Sleep 2s
Escape
Sleep 1s
Type "/blue"
Sleep 1s
Enter
Sleep 2s
Type "?"
Sleep 4s
Escape
Sleep 1s

# Home, and the queue window.
Type "'h"
Sleep 2s
Tab
Sleep 2s
Type ":q"
Enter
Sleep 1s

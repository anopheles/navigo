package ui

import "runtime"

// installHint is how to install mpv on this OS, for play keys while mpv
// is missing or too old.
func installHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install mpv"
	case "windows":
		return "winget install mpv, or scoop install mpv"
	}
	return "install it with your package manager, such as sudo apt install mpv"
}

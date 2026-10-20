//go:build !linux

package mediactl

// New returns the OS's media controls. On macOS and Windows mpv provides
// them itself (Now Playing, SMTC), from the media title navigo sets per
// file, so this is a stub.
func New() (Controls, error) { return newStub(), nil }

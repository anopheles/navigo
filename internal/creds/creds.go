// Package creds finds the password for the configured server and stores
// one the login form checked. The password comes from server.password_command,
// then server.password, then the OS keyring, and is never written as a
// token.
package creds

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/anopheles/navigo/internal/config"
)

// Service is the keyring service navigo stores passwords under.
const Service = "navigo"

// Keyring stores secrets; OS is the real one.
type Keyring interface {
	Get(service, key string) (string, error)
	Set(service, key, secret string) error
}

// OS is the operating system's keyring: Secret Service on Linux, the
// login keychain on macOS, Credential Manager on Windows.
var OS Keyring = osKeyring{}

type osKeyring struct{}

func (osKeyring) Get(service, key string) (string, error) { return keyring.Get(service, key) }
func (osKeyring) Set(service, key, secret string) error   { return keyring.Set(service, key, secret) }

// Key is the keyring key for a server and user.
func Key(url, user string) string { return user + "@" + url }

// ErrNoPassword means none of the sources has a password.
var ErrNoPassword = errors.New("no password configured")

// Password finds the password for s.
func Password(ctx context.Context, s config.Server, kr Keyring) (string, error) {
	if s.PasswordCommand != "" {
		return run(ctx, s.PasswordCommand)
	}
	if s.Password != "" {
		return s.Password, nil
	}
	p, err := kr.Get(Service, Key(s.URL, s.User))
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return "", ErrNoPassword
	case err != nil:
		return "", fmt.Errorf("keyring: %w", err)
	}
	return p, nil
}

// run runs a password_command through the shell and returns its first
// line of output.
func run(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("password_command: %w", err)
	}
	line, _, _ := strings.Cut(string(out), "\n")
	line = strings.TrimRight(line, "\r")
	if line == "" {
		return "", errors.New("password_command printed nothing")
	}
	return line, nil
}

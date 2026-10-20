package creds

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/anopheles/navigo/internal/config"
)

type memKeyring map[string]string

func (m memKeyring) Get(service, key string) (string, error) {
	if p, ok := m[service+"/"+key]; ok {
		return p, nil
	}
	return "", keyring.ErrNotFound
}

func (m memKeyring) Delete(service, key string) error {
	delete(m, service+"/"+key)
	return nil
}

func (m memKeyring) Set(service, key, secret string) error {
	m[service+"/"+key] = secret
	return nil
}

func TestPasswordOrder(t *testing.T) {
	ctx := context.Background()
	kr := memKeyring{}
	s := config.Server{URL: "https://m.example", User: "ann"}
	if _, err := Password(ctx, s, kr); !errors.Is(err, ErrNoPassword) {
		t.Errorf("no sources: %v", err)
	}
	_ = kr.Set(Service, Key(s.URL, s.User), "from-keyring")
	if p, _ := Password(ctx, s, kr); p != "from-keyring" {
		t.Errorf("keyring: %q", p)
	}
	s.Password = "from-config"
	if p, _ := Password(ctx, s, kr); p != "from-config" {
		t.Errorf("config before keyring: %q", p)
	}
	if runtime.GOOS == "windows" {
		return
	}
	s.PasswordCommand = "printf 'from-command\\nsecond line'"
	if p, err := Password(ctx, s, kr); p != "from-command" || err != nil {
		t.Errorf("command first: %q, %v", p, err)
	}
	s.PasswordCommand = "exit 3"
	if _, err := Password(ctx, s, kr); err == nil {
		t.Error("a failing command gave a password")
	}
}

package mpv

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func needMPV(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv not on PATH")
	}
}

func TestStartRealMPV(t *testing.T) {
	needMPV(t)
	p, err := Start(context.Background(), Options{Args: []string{"--ao=null"}})
	if err != nil {
		t.Fatal(err)
	}
	var version string
	if err := p.Get(context.Background(), "mpv-version", &version); err != nil || !strings.HasPrefix(version, "mpv") {
		t.Errorf("mpv-version = %q, %v", version, err)
	}
	var idle bool
	if err := p.Get(context.Background(), "idle-active", &idle); err != nil || !idle {
		t.Errorf("idle-active = %v, %v; want an idle mpv", idle, err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	select {
	case <-p.Exited():
	default:
		t.Error("mpv still running after Close")
	}
}

func TestStartBadOption(t *testing.T) {
	needMPV(t)
	_, err := Start(context.Background(), Options{Args: []string{"--no-such-option"}})
	if err == nil || !strings.Contains(err.Error(), "no-such-option") {
		t.Fatalf("Start = %v, want an error naming the bad option", err)
	}
}

func TestStartMissingBinary(t *testing.T) {
	if _, err := Start(context.Background(), Options{Path: "navigo-no-such-mpv"}); err == nil {
		t.Fatal("Start succeeded without an mpv binary")
	}
}

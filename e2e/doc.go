// Package e2e is layer B: it builds navigo and drives the real binary in a
// pseudo-terminal, against a real Navidrome and a real mpv, reading the
// screen through a terminal emulator. The tests only build with the e2e
// tag:
//
//	go test -tags e2e ./e2e
//
// They read NAVIGO_E2E_URL, NAVIGO_E2E_USER and NAVIGO_E2E_PASSWORD from
// the environment, falling back to .env.e2e at the repo root, and skip
// when neither has them. tools/e2e/run.sh runs them against a throwaway
// Navidrome serving the synthetic fixture library, as CI does.
package e2e

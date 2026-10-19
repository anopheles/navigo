# Fake the server at the HTTP level, not behind a Go interface

There is no `Server` interface. Tests run the real `internal/subsonic` client against an `httptest` fake Subsonic server that serves recorded JSON fixtures, and sync and core-runner tests use the same fake. The client is thin, so an interface would have only one real adapter, and a hand-written fake would skip exactly the code (auth signing, JSON decoding, error envelopes) most likely to break. The live Navidrome server is used only for e2e runs.

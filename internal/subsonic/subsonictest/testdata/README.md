# Recorded Subsonic fixtures

Real responses from a throwaway Navidrome serving the synthetic fixture
library (`tools/internal/fixturelib`). Nothing here comes from a real
library. Don't edit them by hand: re-record with

    tools/fixtures.sh up
    tools/fixtures.sh record
    tools/fixtures.sh down

`<endpoint>.<sorted query>.json` is the response to that request, without
its auth parameters. `error-40.json` (wrong credentials) and
`error-70.json` (not found) are served when a request fails auth or has no
fixture.

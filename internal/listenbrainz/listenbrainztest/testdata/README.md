# ListenBrainz fixtures

Hand-scrubbed copies of real replies from `api.listenbrainz.org`, fetched on 2026-10-23:

- `recommendations.json`: `GET /1/cf/recommendation/user/<user>/recording?count=1000`, cut to four entries.
- `metadata.json`: `POST /1/metadata/recording/` with the body `{"recording_mbids": [...], "inc": "artist"}`. Recording 4 is left out, as ListenBrainz does for an MBID it doesn't know.

Every field and type is as ListenBrainz sent it, including `latest_listened_at` as a date string or `null`. The user name, MBIDs, model links, recordings, artists, dates and ISRCs are replaced. Recording 3, Undertow by The Quiet Ones, is a track in the synthetic fixture library.

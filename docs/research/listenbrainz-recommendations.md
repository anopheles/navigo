# ListenBrainz recommendation endpoints

Question (#78): what ListenBrainz documents and does for
`GET /1/cf/recommendation/user/{user}/recording` and `GET /1/metadata/recording/`.

Sources, read on 2026-10-23:

- `metabrainz/listenbrainz-server` at commit `7d9a3d6f9d` (abbreviated as `LB@7d9a3d6f9d` below).
- `metabrainz/brainzutils-python` tag `v3.0.0`, the version that `requirements.txt` in `LB@7d9a3d6f9d` pins.
- The API docs: <https://listenbrainz.readthedocs.io/en/latest/users/api/index.html> and
  <https://listenbrainz.readthedocs.io/en/latest/users/api/recommendation.html>.
- Live calls to `api.listenbrainz.org`, made without a token on 2026-10-22 at about 22:34 UTC.

## How often recommendations are regenerated

- Production cron runs `spark cron_request_recommendations` daily at 02:01, in the container's local time.
  The comment above it says it runs "after dump and mapping has been imported into the spark cluster".
  Source: `docker/services/cron/crontab` lines 29–30 (`LB@7d9a3d6f9d`).
- That command chains four Spark requests: build dataframes, train the model, compute recording discovery,
  and generate recommendations (`listenbrainz/spark/request_manage.py`, `cron_request_recommendations`).
- The dataframes use the last 180 days of listens (`request_dataframes --days` defaults to 180).
  Listens without a `recording_mbid` are dropped.
  Sources: `request_manage.py`, and the docstring of `listenbrainz_spark/recommendations/recording/create_dataframes.py`.
- Each user gets the top 1,000 recordings by score (`request_recommendations --raw` defaults to 1000).
  Source: `request_manage.py`, and `process_recommendations` in `listenbrainz_spark/recommendations/recording/recommend.py`.
- The server stores one row per user and upserts it with `created = NOW()`
  (`insert_user_recommendation` in `listenbrainz/db/recommendations_cf_recording.py`).
  The API returns that timestamp as `last_updated`, in unix seconds
  (`listenbrainz/webserver/views/recommendations_cf_recording_api.py`).
  So `last_updated` is the time the server wrote the rows. It is not when the model was trained.
- Users who have no listens in the 180-day window are left out of the run. Nothing deletes their old row,
  so they keep stale recommendations and an old `last_updated`.
  Source: the upsert-only handler `handle_recommendations` in `listenbrainz/spark/handlers.py`.
- Live: for `anopheles`, `last_updated` was 1792692721 (2026-10-22 18:12 UTC).
  The model ran at 16:14 that day (taken from the `model_url` name). The rows land many hours after the 02:01 trigger.
- The docs say nothing about the schedule. The cron file is the only source.

## The 204 answer

- The docs list `204: Recommendations for the user haven't been generated, empty response will be returned`
  (the endpoint docstring, rendered on the recommendation API page).
- In code there are two 204 cases. Either the user has no row
  ("No recommendations due to absence of recent listening history"),
  or the row has an empty list ("please try again later"; the payload would include `last_updated`).
  Source: `recommendations_cf_recording_api.py`.
- Live: an existing user with 0 listens (`demo`) got `HTTP 204` with an empty body (0 bytes).
  The headers still had `X-RateLimit-*`. The error message and the payload do not reach the client.
- A user who does not exist gets `404 {"code":404,"error":"Cannot find user: …"}`. This was checked live.
- How long a new user waits: nothing documents it. From the cron file:
  1. The incremental dump runs at 00:00.
  2. Spark imports it at 01:00.
  3. Recommendations are requested at 02:01, and the rows land after training (18:12 UTC in the live sample).

  So a user's first mapped listens can appear in recommendations about 1–2 days later.
  This is inferred from `docker/services/cron/crontab` lines 14–17 and 29–30.

## Rate limits

- The docs ask for "never more than ONE call per second" per client application.
  They name four headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset-In`
  (recommended), and `X-RateLimit-Reset`. Going over the limit returns `429 Too Many Requests`.
  Source: `docs/users/api/index.rst`, "Rate limiting" (the API index page linked above).
- Both endpoints use `@ratelimit()` with no scope. So all API calls from one IP share one bucket
  (`brainzutils/ratelimit.py` `ratelimit` at `v3.0.0`).
- The library defaults are 30 requests per IP per 10-second window (`ratelimit_defaults`).
  ListenBrainz keeps the per-IP default. It raises the per-token limit to 100,000, but only for
  whitelisted tokens (`WHITELISTED_AUTH_TOKENS`). An ordinary user token gets the per-IP limit.
  Source: `listenbrainz/webserver/__init__.py` (`RATELIMIT_PER_TOKEN`, `check_ratelimit_token_whitelist`, `set_rate_limits`).
  The docs only say a valid token "may" get higher limits.
- Live: `x-ratelimit-limit: 30` and `x-ratelimit-reset-in: 10`. `X-RateLimit-Remaining` dropped across
  calls to different endpoints, which confirms the shared bucket.
- The 429 body says "You have exceeded your rate limit. See the X-RateLimit-* response headers…"
  (`on_over_limit`). The server sends no `Retry-After` header. Navidrome's ListenBrainz adapter also reads
  `X-RateLimit-Reset-In` on a 429 (`adapters/listenbrainz/client.go` at `navidrome/navidrome@56cc32aede`).

## Paging past 1,000

- It is not possible. Spark stores only 1,000 per user. The view also caps `count` at 1000:
  "For the purpose of experimenting with recommendations, we're allowing to fetch at most 1K"
  (`recommendations_cf_recording_api.py`, `_process_recommendations`).
- The default `count` is 25 (`DEFAULT_ITEMS_PER_GET` in `listenbrainz/webserver/views/api_tools.py`).
- Live: `count=5000` returned 1000 with `total_mbid_count: 1000`.
  `count=5&offset=998` returned 2 items, both with `"latest_listened_at": null`.

## Metadata endpoint

- `GET /1/metadata/recording/?recording_mbids=a,b,…&inc=artist` has no MBID cap in code.
  The URL length is the only limit. `POST` with a JSON body accepts up to 1,000 MBIDs
  (`MAX_ITEMS_PER_GET`). Source: `listenbrainz/webserver/views/metadata_api.py`.
- Allowed `inc` values are `artist tag release recording release_group`, separated by spaces.
- The server caches each recording for 24 hours (`RECORDING_METADATA_CACHE_TTL`).
  `GET` responses carry `cache-control: public, max-age=0, s-maxage=120` (also seen live).
- Live: an unknown MBID is left out of the response map. It is not an error.

## What "experimental" means

- The endpoint docstring says: "This endpoint is experimental and probably will change in the future."
  ListenBrainz publishes no versioning or deprecation policy for it.
- The history shows breaking changes. In July 2023, commits `24a6af62f9` and `a822aaadf6` removed the
  top-artist and similar-artist candidate sets. That removed the `type` variants.
  The documented sample still shows `"type"` and does not show `model_id`, `model_url` or
  `latest_listened_at`, which the live response returns. The docs lag the code.
- Since 2023 the file has changed only through refactors (2024–2025), and the response shape stayed the same.
  Source: commit history of `recommendations_cf_recording_api.py`.

## What this means for navigo

- **Refetch:** at most once a day. Cache the list with its `last_updated`, and skip the work when
  `last_updated` has not changed. A `last_updated` that is weeks old means the user has no recent
  listens. The list is stale but usable.
- **204:** treat it as "no recommendations yet" and expect no body. Tell the user it can take a day or
  two after their first listens. Do not retry within the session.
  A 404 means a wrong username. That is a configuration error.
- **429 and other errors:** wait `X-RateLimit-Reset-In` seconds, then retry once.
  For 5xx or network errors, keep the cached list.
- **Budget:** 1,000 recommendations take 1 call (`count=1000`). Metadata takes 10 `GET`s of 100 MBIDs each,
  or 1 `POST` of all 1,000. That is 11 requests (or 2), well under 30 per 10 s.
  The documented one-call-per-second rule spreads 11 calls over about 11 s.
  Pace with `X-RateLimit-Remaining`, because the bucket is per IP and shared with all other ListenBrainz calls.
- **Missing metadata:** expect some MBIDs to be absent from the metadata response. Drop them.
- **Stability:** this endpoint is labelled experimental and has broken compatibility before.
  Decode leniently: require only `mbids[].recording_mbid`, and treat `score`, `latest_listened_at`
  and `model_*` as optional. Keep the feature optional, so a change on their side degrades to
  "no recommendations" and does not break navigo.

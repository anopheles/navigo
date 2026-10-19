# The CI check environment, for running the gate locally with tools/ci/local.sh:
# Go plus mpv (Debian's 0.40 meets navigo's 0.38 minimum) and go-licenses.
FROM golang:1.26
RUN apt-get update \
	&& apt-get install -y --no-install-recommends mpv \
	&& rm -rf /var/lib/apt/lists/*
RUN go install github.com/google/go-licenses/v2@v2.0.1

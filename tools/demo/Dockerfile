# VHS with mpv, to record the demo: navigo starts mpv itself, and the VHS
# image doesn't have it. Everything else (ttyd, ffmpeg, a browser and fonts)
# comes from VHS.
FROM ghcr.io/charmbracelet/vhs:v0.12.1
RUN apt-get update \
	&& apt-get install -y --no-install-recommends mpv \
	&& rm -rf /var/lib/apt/lists/*

# syntax=docker/dockerfile:1

FROM golang:1.26.6-bookworm AS build

WORKDIR /src

# VexBot's Opus decoder uses cgo and links against libopus.
RUN apt-get update \
    && apt-get install --yes --no-install-recommends build-essential libopus-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/vexbot .

FROM debian:bookworm-slim AS runtime

# ffmpeg prepares turn audio. libgomp1 and libstdc++6 support standard
# CPU-only Whisper.cpp Linux builds mounted into the container at runtime.
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates ffmpeg libgomp1 libopus0 libstdc++6 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 vexbot \
    && useradd --system --uid 10001 --gid vexbot --home-dir /app --create-home vexbot \
    && mkdir --parents /app/recordings /opt/whisper \
    && chown --recursive vexbot:vexbot /app

WORKDIR /app

COPY --from=build --chown=vexbot:vexbot /out/vexbot /usr/local/bin/vexbot

USER vexbot

# The image supplies ffmpeg. Mount a Linux Whisper.cpp CLI and model at
# /opt/whisper, then configure their paths through the environment.
ENV FFMPEG_PATH=ffmpeg

VOLUME ["/app/recordings"]

ENTRYPOINT ["/usr/local/bin/vexbot"]

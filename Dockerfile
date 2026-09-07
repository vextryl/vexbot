# syntax=docker/dockerfile:1

# Keep both stages on Trixie so their native library versions are compatible.
FROM golang:1.26.6-trixie AS build

WORKDIR /src

# Install compilation tools and Opus headers here; libdave is installed below.
RUN apt-get update \
    && apt-get install --yes --no-install-recommends build-essential libopus-dev libopusfile-dev pkg-config curl unzip \
    && rm -rf /var/lib/apt/lists/*

# GoDave v0.3.0 requires libdave v1.1.0. Its official binaries need
# glibc >= 2.38 and a newer libstdc++ than Bookworm supplies.
# When updating libdave, update the release URL, both archive checksums, and
# pkg-config version together. Preserve the bundled dependency licenses.
# BuildKit supplies TARGETARCH; only amd64 and arm64 releases are supported.
ARG TARGETARCH
RUN set -eu; \
    case "$TARGETARCH" in \
      amd64) arch=X64; checksum=33157b8cbadcdd3c6cb4df3be18e8bbe7b86e3c65d57d0f1bc9dadc62a768d6a ;; \
      arm64) arch=ARM64; checksum=fc119512aefac9cb7652634c16ecb0833a3c4bf91d87b9ec8d6acda03d454349 ;; \
      *) echo "Unsupported libdave architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/discord/libdave/releases/download/v1.1.0/cpp/libdave-Linux-${arch}-boringssl.zip" -o /tmp/libdave.zip; \
    echo "$checksum  /tmp/libdave.zip" | sha256sum -c -; \
    unzip -q /tmp/libdave.zip -d /tmp/libdave; \
    install -m 644 /tmp/libdave/include/dave/dave.h /usr/local/include/dave.h; \
    install -m 755 /tmp/libdave/lib/libdave.so /usr/local/lib/libdave.so; \
    mkdir -p /usr/local/share/licenses/libdave /usr/local/lib/pkgconfig; \
    cp /tmp/libdave/licenses/* /usr/local/share/licenses/libdave/; \
    printf '%s\n' 'Name: dave' 'Description: Discord DAVE library' 'Version: 1.1.0' \
      'Libs: -L/usr/local/lib -ldave' 'Cflags: -I/usr/local/include' \
      > /usr/local/lib/pkgconfig/dave.pc; \
    ldconfig; \
    rm -rf /tmp/libdave /tmp/libdave.zip

# Cache Go dependencies separately from application source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/vexbot .

FROM debian:trixie-slim AS runtime

# ffmpeg prepares turn audio; Opus and Opusfile are linked by the bot.
# libstdc++6 supports libdave and Whisper; libgomp1 supports Whisper's OpenMP.
# Build the mounted CPU-only Whisper CLI as described in README.md so it
# includes Whisper/GGML instead of requiring their separate shared libraries.
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates ffmpeg libgomp1 libopus0 libopusfile0 libstdc++6 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 vexbot \
    && useradd --system --uid 10001 --gid vexbot --home-dir /app --create-home vexbot \
    && mkdir --parents /app/recordings /opt/whisper \
    && chown --recursive vexbot:vexbot /app

WORKDIR /app

COPY --from=build --chown=vexbot:vexbot /out/vexbot /usr/local/bin/vexbot
# Only the native runtime library and licenses are needed in the final image.
# Register /usr/local/lib with the dynamic loader before dropping privileges.
COPY --from=build /usr/local/lib/libdave.so /usr/local/lib/libdave.so
COPY --from=build /usr/local/share/licenses/libdave /usr/local/share/licenses/libdave
RUN ldconfig

USER vexbot

# The image supplies ffmpeg. Mount the Whisper CLI and model read-only at
# /opt/whisper and pass their container paths plus Discord configuration with
# --env-file. Credentials, Whisper binaries, and models are supplied at runtime.
ENV FFMPEG_PATH=ffmpeg

# Mount persistent host storage here, writable by container UID/GID 10001.
# Image ownership does not override the permissions of a host bind mount.
VOLUME ["/app/recordings"]

# Run the bot directly so it receives SIGTERM and finalizes recordings.
ENTRYPOINT ["/usr/local/bin/vexbot"]

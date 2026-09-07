# VexBot

VexBot is a fully self-hosted Discord voice-recording and transcription bot.
It records speaker-separated audio in a voice channel, transcribes completed
sessions locally, and writes a readable chronological conversation log.

## Running locally

For a fresh Linux installation, follow the Docker guide below. Running directly
from source requires Go 1.26.6 or newer, a C/C++ toolchain, `pkg-config`, Opus
and Opusfile development libraries, and native libdave 1.1.0. Follow
[GoDave's native library setup](https://github.com/disgoorg/godave#libdave-installation)
and ensure `pkg-config --modversion opus opusfile dave` succeeds. Local
transcription also requires ffmpeg and Whisper.cpp.

With those dependencies installed, create `.env` from `.env.example` in the
repository root, set the required values, then run:

```sh
go run .
```

VexBot registers three guild commands:

- `/ping` checks that the bot is responding.
- `/join` joins the caller's current voice channel and begins recording.
- `/stop` stops the recording, leaves the channel, and begins local
  transcription when Whisper is configured.

## Docker deployment: step-by-step Linux setup

This guide targets an Ubuntu 24.04 homelab host using Docker Engine. Audio is
received from Discord, recorded and transcribed locally, and the completed
transcript is posted back to Discord. No cloud transcription service is used.
Setup needs internet access for source, packages, images, and the Whisper model;
normal operation needs a Discord connection.

Run commands as your normal SSH login user with `sudo` access. If an
administrator runs a command from a root shell, omit `sudo` and replace
`$USER` with the intended login user wherever ownership or group membership
is changed. A host account named `vexbot` is optional; the image creates its
own independent `vexbot` user.

The production image contains VexBot, `ffmpeg`, and the Linux runtime libraries
needed by typical CPU-only Whisper.cpp builds. It runs as the non-root `vexbot`
user (UID/GID `10001`). Discord credentials, Whisper models, recordings, and
the Whisper executable are never copied into the image.

The image uses Debian Trixie and installs Opus, Opusfile, and the pinned
libdave 1.1.0 native library required by GoDave. These bot dependencies are
installed by Docker; you do not need to install them on the host. Linux amd64
and arm64 builds use checksum-verified upstream libdave binaries.

### 1. Prepare the host

Install Git, Docker Engine, and the small set of packages used to build a local
Whisper.cpp executable. On a Debian or Ubuntu host:

```sh
sudo apt-get update
sudo apt-get install --yes git build-essential cmake curl ca-certificates nano
```

Install Docker Engine using Docker's [Ubuntu instructions](https://docs.docker.com/engine/install/ubuntu/)
(or [Debian instructions](https://docs.docker.com/engine/install/debian/) on Debian).
A VS Code Docker extension alone does not install the Docker daemon.
After installation, verify that the daemon works:

```sh
sudo systemctl enable --now docker
sudo docker run --rm hello-world
```

Using `sudo docker` is completely fine. If you prefer to run Docker without
`sudo`, add your login user to the Docker group:

```sh
sudo groupadd --force docker
sudo usermod -aG docker "$USER"
```

Fully log out of SSH and reconnect, then run `id` and `docker version`.
`id` must include `docker`; `docker version` must show both Client and Server.
Existing processes keep their old groups. With VS Code Remote SSH, reconnect
and restart any agent sessions; if needed, run **Remote-SSH: Kill VS Code
Server on Host...** from the command palette before reconnecting.
The Docker group grants root-equivalent access; see Docker's
[post-install instructions](https://docs.docker.com/engine/install/linux-postinstall/).
The commands below retain `sudo`, including when reading the root-owned
secret environment file.

Create a permanent home for VexBot. The source checkout, secret configuration,
Whisper files, and recordings are deliberately separate:

```sh
sudo mkdir -p /srv/vexbot/recordings
sudo mkdir -p /srv/vexbot/whisper/models
sudo chown "$USER":"$USER" /srv/vexbot
sudo chown 10001:10001 /srv/vexbot/recordings
```

The first ownership command lets your normal Linux user clone and update the
source checkout in step 4. The second gives only the recordings directory
to the non-root user inside the container. Do not change it back to your SSH
user's ownership: the container needs numeric UID/GID `10001:10001`.

### 2. Create and invite the Discord bot

In the [Discord Developer Portal](https://discord.com/developers/applications), create an application, add a Bot user, and
copy its token. Treat the token like a password: do not paste it into chat,
commit it to Git, or put it in the Docker image.

Invite the bot to the server where it will run with the `bot` and
`applications.commands` scopes. In the text channel where you use `/join`, and
the voice channels VexBot will record, grant the bot these permissions:

- View Channel
- Send Messages
- Attach Files
- Connect

Administrator is not required. Copy the server's numeric ID as well: enable
Discord Developer Mode, right-click the server icon, then choose **Copy Server
ID**.

### 3. Build Whisper.cpp and download a local model

These commands compile Whisper.cpp on the Linux host and download the English
`base.en` model. Whisper.cpp's own [quick-start guide](https://github.com/ggml-org/whisper.cpp/blob/master/README.md)
describes the build and model-download workflow. The options below prepare a
CPU-only executable for the mount layout used here.

```sh
git clone https://github.com/ggml-org/whisper.cpp.git /tmp/whisper.cpp
cd /tmp/whisper.cpp
sh ./models/download-ggml-model.sh base.en
cmake -B build -DBUILD_SHARED_LIBS=OFF -DWHISPER_CURL=OFF
cmake --build build --parallel 2 --config Release --target whisper-cli

sudo install -m 755 ./build/bin/whisper-cli /srv/vexbot/whisper/whisper-cli
sudo install -m 644 ./models/ggml-base.en.bin /srv/vexbot/whisper/models/ggml-base.en.bin
```

If `/tmp/whisper.cpp` already contains your checkout, skip the clone command
and start with `cd /tmp/whisper.cpp`. Stop if a command fails and resolve its
error before continuing to the install commands.

The executable is compiled for this Linux machine, then mounted read-only into
the container. `BUILD_SHARED_LIBS=OFF` includes Whisper and GGML in the
executable so copying just `whisper-cli` works without their separate shared
libraries. Two build jobs limit memory use; increase this if the host has
capacity. The model stays on the host. To choose another model later,
download that model with Whisper.cpp, copy it into `/srv/vexbot/whisper/models/`,
and update `WHISPER_MODEL_PATH` below.

### 4. Get VexBot and create its secret configuration file

Clone the VexBot repository as your normal login user, without `sudo`. For a
private repository, your GitHub account must have access. With HTTPS, enter
your GitHub username and use a personal access token with repository read
access when Git asks for a password; a GitHub account password will not work.
Do not put the token in the clone URL. See
[GitHub authentication](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github).
An SSH key configured for this same Linux user is another option.

```sh
git clone https://github.com/vextryl/vexbot.git /srv/vexbot/source
cd /srv/vexbot/source
```

Keep the clone command on one line: `/srv/vexbot/source` is its destination.
If this checkout already exists, use `cd /srv/vexbot/source` and `git status`
instead of cloning again.

Create the environment file outside the repository so it cannot be accidentally
committed. Replace every placeholder before continuing:

```sh
sudo nano /srv/vexbot/vexbot.env
```

Paste this into the editor, then replace the two Discord values:

```dotenv
DISCORD_TOKEN=paste-your-bot-token-here
DISCORD_GUILD_ID=paste-your-server-id-here

RECORDING_RETENTION_COUNT=5
DISCORD_TRANSCRIPT_UPLOAD_LIMIT_BYTES=16777216

WHISPER_CLI_PATH=/opt/whisper/whisper-cli
WHISPER_MODEL_PATH=/opt/whisper/models/ggml-base.en.bin
FFMPEG_PATH=ffmpeg
WHISPER_LANGUAGE=en
```

Save in Nano with **Ctrl+O**, press **Enter**, then exit with **Ctrl+X**. Lock
down the file because it contains the Discord token:

```sh
sudo chmod 600 /srv/vexbot/vexbot.env
```

The `/opt/whisper/...` values are paths **inside the container**, mapped from
`/srv/vexbot/whisper` on the host. Docker's `--env-file` reads this file when
the container is created; do not use `export` or quote the values here.

### 5. Build and start VexBot

From the VexBot source directory, build the image:

```sh
cd /srv/vexbot/source
sudo docker build --tag vexbot:local .
```

Before starting the bot, verify the mounted Whisper executable can run inside
the image:

```sh
sudo docker run --rm --network none \
  --volume /srv/vexbot/whisper:/opt/whisper:ro \
  --entrypoint /opt/whisper/whisper-cli vexbot:local --help
```

This should print Whisper's options. If it reports missing `libwhisper` or
`libggml` libraries, repeat step 3 with `BUILD_SHARED_LIBS=OFF` and copy the
rebuilt executable again.

Check recording-directory permissions and model readability using the actual
container user:

```sh
sudo docker run --rm --network none \
  --mount type=bind,src=/srv/vexbot/recordings,dst=/app/recordings \
  --mount type=bind,src=/srv/vexbot/whisper,dst=/opt/whisper,readonly \
  --entrypoint sh vexbot:local -ec \
  'id; probe=$(mktemp /app/recordings/.vexbot-check.XXXXXX); rm "$probe"; test -r /opt/whisper/models/ggml-base.en.bin; echo "Storage checks passed"'
```

If writing fails, repeat `sudo chown 10001:10001 /srv/vexbot/recordings`.
If the model check fails, complete the model installation in step 3. These
checks do not connect to Discord.

Start one named container. The first mount preserves recordings and transcripts;
the second makes the host's local Whisper executable and model available without
copying them into the image:

```sh
sudo docker run --detach --name vexbot --restart unless-stopped \
  --env-file /srv/vexbot/vexbot.env \
  --volume /srv/vexbot/recordings:/app/recordings \
  --volume /srv/vexbot/whisper:/opt/whisper:ro \
  vexbot:local
```

### 6. Verify it works

Check that the container is running and watch its startup log:

```sh
sudo docker ps --all --filter name=vexbot
sudo docker logs --follow vexbot
```

You should see VexBot connect to Discord and register its commands. In Discord,
run `/ping`, then join a voice channel and run `/join` in a text channel. Use
`/stop` from the same Discord account after saying a few words; only the
recording's starter can stop it. The transcript and WAV files should appear
under `/srv/vexbot/recordings/`. Transcription progress and the completed
attachment appear in the text channel where `/join` was used.
Press **Ctrl+C** to leave the log viewer; this does not stop the detached bot.
If the container exits or restarts, inspect its logs for missing configuration
or dependency errors before testing commands in Discord.

Useful everyday commands:

```sh
sudo docker logs --follow vexbot  # watch VexBot logs
sudo docker restart vexbot        # restart with the existing configuration
sudo docker stop vexbot           # stop cleanly; VexBot finalizes recordings
sudo docker start vexbot          # start it again
```

Changes to `vexbot.env` require removing and recreating the container with the
same `docker run` command from step 5; `docker restart` does not reread that
file. For a configuration-only change, no image rebuild is needed.

Before an update or restart, use `/stop` and wait for transcript delivery if
you want the current recording transcribed. Application shutdown finalizes
recordings but does not start transcription, and in-progress transcription
is not guaranteed to finish before process exit.

To update VexBot later, pull and build first so a failed build leaves the
existing container available. Then replace the container. These commands
preserve recordings, Whisper files, and the environment file:

```sh
cd /srv/vexbot/source
git pull --ff-only
sudo docker build --tag vexbot:local .
```

Only after the build succeeds:

```sh
sudo docker stop vexbot
sudo docker rm vexbot

sudo docker run --detach --name vexbot --restart unless-stopped \
  --env-file /srv/vexbot/vexbot.env \
  --volume /srv/vexbot/recordings:/app/recordings \
  --volume /srv/vexbot/whisper:/opt/whisper:ro \
  vexbot:local
```

Do not remove `/srv/vexbot/recordings` or `/srv/vexbot/whisper`; those hold the
persistent artifacts and local transcription dependency.

`/app/recordings` is the image's persistent-volume location. It maps directly
to VexBot's relative `recordings/` directory because the container runs from
`/app`. The existing local-development workflow remains unchanged: use
`go run .` with your local `.env` file.

## Local transcription setup

Set these values in `.env`:

| Variable | Required | Purpose |
| --- | --- | --- |
| `DISCORD_TOKEN` | Yes | VexBot's Discord bot token. |
| `DISCORD_GUILD_ID` | Yes | Guild where VexBot registers its commands. |
| `RECORDING_RETENTION_COUNT` | No | Completed recording sessions to keep; defaults to `5`. |
| `DISCORD_TRANSCRIPT_UPLOAD_LIMIT_BYTES` | No | Explicit transcript attachment ceiling in bytes; defaults to 16 MiB. Set it to a limit supported by the bot's Discord server/account. |
| `WHISPER_CLI_PATH` | For transcription | Path to the local Whisper.cpp CLI executable. |
| `WHISPER_MODEL_PATH` | For transcription | Path to the local Whisper model file. |
| `FFMPEG_PATH` | No | Local `ffmpeg` executable; defaults to `ffmpeg`. |
| `WHISPER_LANGUAGE` | No | Transcription language; defaults to `en`. |
| `WHISPER_DTW_PRESET` | No | Whisper timestamp preset when it cannot be inferred from the model filename. |

After `/stop`, VexBot uses local `ffmpeg` to prepare each speaker turn as
16 kHz mono PCM WAV audio and invokes the configured Whisper.cpp executable.
Whisper runs locally, either directly on the host or inside its Docker
container. Audio is not sent to a remote transcription provider. The completed
transcript is uploaded to Discord; the Whisper executable and model remain local.

Temporary per-turn audio and Whisper output are removed as each turn finishes.

When delivery is enabled, VexBot checks `transcript.txt` against its configured
attachment ceiling before attempting a Discord upload. The 16 MiB default is a
conservative VexBot safety limit, not a claim about Discord's current limits;
set `DISCORD_TRANSCRIPT_UPLOAD_LIMIT_BYTES` explicitly for the server/account
where the bot runs. Oversized transcripts remain in local recording storage and
VexBot reports the delivery failure without exposing local paths in Discord.

## Recording output

Each session is written under `recordings/` in a timestamped directory:

```text
recordings/
  20260901T231500Z-<guild-id>/
    <safe-display-name>.wav
    <another-safe-display-name>.wav
    timeline.json
    transcript.txt
```

Each participant has one compact WAV file containing their recorded audio.
`timeline.json` maps that compact audio back to shared session time. VexBot uses
the timeline to identify speaker turns, transcribe them, and write
`transcript.txt` in conversation order with display names and timestamps.
`transcript.txt` is created when local transcription is configured.

`recordings/` is ignored by Git and is intended to be persistent storage. When
VexBot is deployed in Docker, it should be mounted as a persistent host volume
so recordings and transcripts survive container replacement.

Before a new recording starts, VexBot removes the oldest completed session
directories as needed to retain the configured number of sessions. Active local
transcription directories are protected from this cleanup.

## Project layout

- `internal/app` configures runtime dependencies, logging, and shutdown.
- `internal/audio` receives decoded Discord audio and creates speaker segments.
- `internal/dave` provides local Discord DAVE session support and decrypt-failure logging.
- `internal/discord` handles Discord commands, interactions, voice connections,
  and display-name snapshots.
- `internal/recording` manages output recording filenames and retention cleanup.
- `internal/session` owns active recording sessions and transcription progress.
- `internal/speaker` normalizes and resolves readable speaker labels.
- `internal/transcript` writes the chronological combined transcript.
- `internal/turn` groups timeline data into speaker turns and transcribes them.
- `internal/wav` writes per-speaker WAV files and the shared timeline.
- `internal/whisper` invokes the local Whisper.cpp CLI and parses its output.

## Roadmap

- [x] Join a Discord voice channel.
- [x] Record speaker-separated audio.
- [x] Transcribe locally with Whisper.cpp.
- [x] Generate timestamped transcripts.
- [x] Upload completed transcripts to Discord with progress reporting.
- [x] Package the bot for Docker deployment.
- [ ] Add automated CI build and test checks.

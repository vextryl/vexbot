# VexBot

VexBot is a fully self-hosted Discord voice-recording and transcription bot.
It records speaker-separated audio in a voice channel, transcribes completed
sessions locally, and writes a readable chronological conversation log.

## Running locally

Create `.env` from `.env.example`, set the required values, then run:

```sh
go run .
```

VexBot registers three guild commands:

- `/ping` checks that the bot is responding.
- `/join` joins the caller's current voice channel and begins recording.
- `/stop` stops the recording, leaves the channel, and begins local
  transcription when Whisper is configured.

## Docker deployment: step-by-step Linux setup

This guide is for a Linux homelab host. It keeps everything local: VexBot,
Whisper.cpp, its model, recordings, and transcripts stay on the host or inside
its local Docker container. Discord is only used for the bot connection and
for the transcript message it posts back to your server.

The production image contains VexBot, `ffmpeg`, and the Linux runtime libraries
needed by typical CPU-only Whisper.cpp builds. It runs as the non-root `vexbot`
user (UID/GID `10001`). Discord credentials, Whisper models, recordings, and
the Whisper executable are never copied into the image.

### 1. Prepare the host

Install Git, Docker Engine, and the small set of packages used to build a local
Whisper.cpp executable. On a Debian or Ubuntu host:

```sh
sudo apt-get update
sudo apt-get install --yes git build-essential cmake
```

Install Docker Engine using Docker's [official Linux installation guide](https://docs.docker.com/engine/install/).
After installation, verify that the daemon works:

```sh
sudo docker run --rm hello-world
```

Using `sudo docker` is completely fine. If you prefer to run Docker without
`sudo`, follow Docker's [post-install instructions](https://docs.docker.com/engine/install/linux-postinstall/).

Create a permanent home for VexBot. The source checkout, secret configuration,
Whisper files, and recordings are deliberately separate:

```sh
sudo mkdir -p /srv/vexbot/recordings
sudo mkdir -p /srv/vexbot/whisper/models
sudo chown "$USER":"$USER" /srv/vexbot
sudo chown 10001:10001 /srv/vexbot/recordings
```

The first ownership command lets your normal Linux user clone and update the
source checkout in the next step. The second gives only the recordings directory
to the non-root user inside the container.

### 2. Create and invite the Discord bot

In the Discord Developer Portal, create an application, add a Bot user, and
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
uses the same build and model-download commands.

```sh
git clone https://github.com/ggml-org/whisper.cpp.git /tmp/whisper.cpp
cd /tmp/whisper.cpp
sh ./models/download-ggml-model.sh base.en
cmake -B build
cmake --build build -j --config Release

sudo install -m 755 ./build/bin/whisper-cli /srv/vexbot/whisper/whisper-cli
sudo install -m 644 ./models/ggml-base.en.bin /srv/vexbot/whisper/models/ggml-base.en.bin
```

The executable is compiled for this Linux machine, then mounted read-only into
the container. The model stays on the host. To choose another model later,
download that model with Whisper.cpp, copy it into `/srv/vexbot/whisper/models/`,
and update `WHISPER_MODEL_PATH` below.

### 4. Get VexBot and create its secret configuration file

Clone the VexBot repository somewhere you normally keep application source. If
the repository is private, use the GitHub authentication method you normally
use for private repositories.

```sh
git clone https://github.com/vextryl/vexbot.git /srv/vexbot/source
cd /srv/vexbot/source
```

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

### 5. Build and start VexBot

From the VexBot source directory, build the image:

```sh
cd /srv/vexbot/source
sudo docker build --tag vexbot:local .
```

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
sudo docker ps
sudo docker logs --follow vexbot
```

You should see VexBot connect to Discord and register its commands. In Discord,
run `/ping`, then join a voice channel and run `/join`. Use `/stop` after saying
a few words. The transcript and WAV files should appear on the host under
`/srv/vexbot/recordings/`.

Useful everyday commands:

```sh
sudo docker logs --follow vexbot  # watch VexBot logs
sudo docker restart vexbot        # restart after a configuration change
sudo docker stop vexbot           # stop cleanly; VexBot finalizes recordings
sudo docker start vexbot          # start it again
```

To update VexBot later, run these commands. They remove only the container—not
your recordings, Whisper executable, model, or secret environment file:

```sh
sudo docker stop vexbot
sudo docker rm vexbot

cd /srv/vexbot/source
git pull
sudo docker build --tag vexbot:local .

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
Whisper runs on the host machine: VexBot does not upload audio, transcripts, or
metadata to a cloud service, external AI service, or any other remote
transcription provider. The Whisper executable and model both remain local.

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
- `internal/recording` manages output recording filenames.
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
- [ ] Upload or otherwise share a completed transcript from the session.

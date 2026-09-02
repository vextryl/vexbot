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

## Local transcription setup

Set these values in `.env`:

| Variable | Required | Purpose |
| --- | --- | --- |
| `DISCORD_TOKEN` | Yes | VexBot's Discord bot token. |
| `DISCORD_GUILD_ID` | Yes | Guild where VexBot registers its commands. |
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

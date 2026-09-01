# vexbot
A self-hosted Discord transcription bot.

## Local transcription setup

VexBot records one WAV file per speaker. To transcribe those recordings after
`/stop`, configure `WHISPER_CLI_PATH` and `WHISPER_MODEL_PATH` in `.env` (see
`.env.example`). VexBot uses `ffmpeg` to prepare 16 kHz mono PCM WAV input, then
runs Whisper.cpp locally. Each speaker's transcript is written beside their WAV
file as `<discord-user-id>.txt`; Whisper's WAV-relative timestamped segments are
also saved as `<discord-user-id>.json`.

## Roadmap

- Join discord channel
- Record audio (utilizing discord marking audio owners)
- Transcribe locally
- Generate transcripts with timestamp
- Upload transcript from session

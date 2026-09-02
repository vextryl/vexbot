package vexbot

import "github.com/vextryl/vexbot/internal/wav"

type RecordingFile = wav.File
type sessionTimeline = wav.Timeline
type timelineSpan = wav.Span

const (
	timelineFileName = wav.TimelineFileName
	wavHeaderSize    = wav.HeaderSize
	wavSampleBytes   = wav.SampleBytes
)

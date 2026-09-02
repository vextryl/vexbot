package wav

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/audio"
)

const (
	RecordingsDirectory = "recordings"
	TimelineFileName    = "timeline.json"
	HeaderSize          = 44
	SampleBytes         = 2
)

type Recorder struct {
	mu       sync.Mutex
	dir      string
	writers  map[snowflake.ID]*wavWriter
	spans    []recordingSpan
	lastSpan map[snowflake.ID]int
	closed   bool
	err      error
}

type wavWriter struct {
	file       *os.File
	writer     *bufio.Writer
	path       string
	sampleRate int
	channels   int
	dataBytes  uint32
}

type File struct {
	UserID snowflake.ID
	Path   string
}

type recordingSpan struct {
	UserID       snowflake.ID
	SessionStart time.Duration
	SessionEnd   time.Duration
	WAVStart     time.Duration
	WAVEnd       time.Duration
}

type Timeline struct {
	Version int    `json:"version"`
	Spans   []Span `json:"spans"`
}

type Span struct {
	UserID         string `json:"user_id"`
	SessionStartMS int64  `json:"session_start_ms"`
	SessionEndMS   int64  `json:"session_end_ms"`
	WAVStartMS     int64  `json:"wav_start_ms"`
	WAVEndMS       int64  `json:"wav_end_ms"`
}

func NewRecorder(guildID snowflake.ID, startedAt time.Time) (*Recorder, error) {
	sessionID := fmt.Sprintf("%s-%s", startedAt.UTC().Format("20060102T150405Z"), guildID)
	dir := filepath.Join(RecordingsDirectory, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create recording directory: %w", err)
	}

	return &Recorder{
		dir:      dir,
		writers:  make(map[snowflake.ID]*wavWriter),
		lastSpan: make(map[snowflake.ID]int),
	}, nil
}

func (r *Recorder) Directory() string {
	return r.dir
}

func (r *Recorder) Files() []File {
	r.mu.Lock()
	defer r.mu.Unlock()

	files := make([]File, 0, len(r.writers))
	for userID, writer := range r.writers {
		files = append(files, File{
			UserID: userID,
			Path:   writer.path,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files
}

func (r *Recorder) ConsumeChunk(chunk audio.Chunk) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed || r.err != nil {
		return
	}

	writer, ok := r.writers[chunk.UserID]
	if !ok {
		var err error
		writer, err = r.newWriter(chunk.UserID, chunk.SampleRate, chunk.Channels)
		if err != nil {
			r.setError(err)
			return
		}
		r.writers[chunk.UserID] = writer
	}

	if writer.sampleRate != chunk.SampleRate || writer.channels != chunk.Channels {
		r.setError(fmt.Errorf("audio format changed for user %v", chunk.UserID))
		return
	}

	wavStart := writer.duration()
	duration := chunkDuration(chunk)
	if err := writer.writeSamples(chunk.Samples); err != nil {
		r.setError(fmt.Errorf("write audio for user %v: %w", chunk.UserID, err))
		return
	}

	r.recordSpan(recordingSpan{
		UserID:       chunk.UserID,
		SessionStart: chunk.Timestamp,
		SessionEnd:   chunk.Timestamp + duration,
		WAVStart:     wavStart,
		WAVEnd:       wavStart + duration,
	})
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return r.err
	}
	r.closed = true

	var closeErr error
	for userID, writer := range r.writers {
		if err := writer.close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("finalize audio for user %v: %w", userID, err))
		}
	}

	r.setError(closeErr)
	if r.err != nil {
		return r.err
	}

	if err := r.writeTimeline(); err != nil {
		r.setError(fmt.Errorf("write session timeline: %w", err))
	}

	return r.err
}

func (r *Recorder) newWriter(userID snowflake.ID, sampleRate, channels int) (*wavWriter, error) {
	path := filepath.Join(r.dir, userID.String()+".wav")
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	writer := &wavWriter{
		file:       file,
		writer:     bufio.NewWriter(file),
		path:       path,
		sampleRate: sampleRate,
		channels:   channels,
	}
	if err := writer.writeHeader(); err != nil {
		_ = file.Close()
		return nil, err
	}

	return writer, nil
}

func (r *Recorder) setError(err error) {
	if err != nil && r.err == nil {
		r.err = err
	}
}

func (r *Recorder) recordSpan(span recordingSpan) {
	if lastIndex, ok := r.lastSpan[span.UserID]; ok {
		last := &r.spans[lastIndex]
		if recordingTimestampsMatch(last.SessionEnd, span.SessionStart) &&
			recordingTimestampsMatch(last.WAVEnd, span.WAVStart) {
			last.SessionEnd = span.SessionEnd
			last.WAVEnd = span.WAVEnd
			return
		}
	}

	r.lastSpan[span.UserID] = len(r.spans)
	r.spans = append(r.spans, span)
}

func recordingTimestampsMatch(first, second time.Duration) bool {
	difference := first - second
	if difference < 0 {
		difference = -difference
	}
	return difference <= time.Millisecond
}

func (r *Recorder) writeTimeline() error {
	spans := append([]recordingSpan(nil), r.spans...)
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].SessionStart == spans[j].SessionStart {
			return spans[i].UserID < spans[j].UserID
		}
		return spans[i].SessionStart < spans[j].SessionStart
	})

	timeline := Timeline{
		Version: 1,
		Spans:   make([]Span, 0, len(spans)),
	}
	for _, span := range spans {
		timeline.Spans = append(timeline.Spans, Span{
			UserID:         span.UserID.String(),
			SessionStartMS: span.SessionStart.Milliseconds(),
			SessionEndMS:   span.SessionEnd.Milliseconds(),
			WAVStartMS:     span.WAVStart.Milliseconds(),
			WAVEndMS:       span.WAVEnd.Milliseconds(),
		})
	}

	contents, err := json.MarshalIndent(timeline, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')

	temporaryFile, err := os.CreateTemp(r.dir, TimelineFileName+"-*")
	if err != nil {
		return err
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporaryFile.Write(contents); err != nil {
		_ = temporaryFile.Close()
		return err
	}
	if err := temporaryFile.Close(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, filepath.Join(r.dir, TimelineFileName))
}

func chunkDuration(chunk audio.Chunk) time.Duration {
	frames := len(chunk.Samples) / chunk.Channels
	return time.Duration(frames) * time.Second / time.Duration(chunk.SampleRate)
}

func (w *wavWriter) duration() time.Duration {
	frames := int64(w.dataBytes) / int64(w.channels*SampleBytes)
	return time.Duration(frames) * time.Second / time.Duration(w.sampleRate)
}

func (w *wavWriter) writeSamples(samples []int16) error {
	bytes := make([]byte, len(samples)*SampleBytes)
	if len(bytes) > int(^uint32(0)-w.dataBytes) {
		return fmt.Errorf("WAV file exceeds 4 GiB")
	}

	for i, sample := range samples {
		binary.LittleEndian.PutUint16(bytes[i*SampleBytes:], uint16(sample))
	}

	if _, err := w.writer.Write(bytes); err != nil {
		return err
	}
	w.dataBytes += uint32(len(bytes))
	return nil
}

func (w *wavWriter) writeHeader() error {
	header := make([]byte, HeaderSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(w.channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(w.sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(w.sampleRate*w.channels*SampleBytes))
	binary.LittleEndian.PutUint16(header[32:34], uint16(w.channels*SampleBytes))
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")

	_, err := w.writer.Write(header)
	return err
}

func (w *wavWriter) close() error {
	if err := w.writer.Flush(); err != nil {
		_ = w.file.Close()
		return err
	}
	if _, err := w.file.Seek(4, 0); err != nil {
		_ = w.file.Close()
		return err
	}

	var sizes [8]byte
	binary.LittleEndian.PutUint32(sizes[0:4], 36+w.dataBytes)
	binary.LittleEndian.PutUint32(sizes[4:8], w.dataBytes)
	if _, err := w.file.Write(sizes[0:4]); err != nil {
		_ = w.file.Close()
		return err
	}
	if _, err := w.file.Seek(40, 0); err != nil {
		_ = w.file.Close()
		return err
	}
	if _, err := w.file.Write(sizes[4:8]); err != nil {
		_ = w.file.Close()
		return err
	}

	return w.file.Close()
}

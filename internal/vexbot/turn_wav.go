package vexbot

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/vextryl/vexbot/internal/wav"
)

type pcmWAVFormat struct {
	SampleRate uint32
	Channels   uint16
	DataBytes  uint32
}

// extractTurnWAV writes a temporary PCM WAV containing exactly one
// transcription turn. temporaryDir is owned and cleaned up by the caller.
func extractTurnWAV(temporaryDir, sourcePath string, turn transcriptionTurn) (string, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open source WAV: %w", err)
	}
	defer source.Close()

	format, err := readPCMWAVFormat(source)
	if err != nil {
		return "", fmt.Errorf("read source WAV: %w", err)
	}

	startFrame, err := wavDurationToFrames(turn.WAVStart, format.SampleRate)
	if err != nil {
		return "", fmt.Errorf("turn start: %w", err)
	}
	endFrame, err := wavDurationToFrames(turn.WAVEnd, format.SampleRate)
	if err != nil {
		return "", fmt.Errorf("turn end: %w", err)
	}
	if endFrame <= startFrame {
		return "", fmt.Errorf("turn WAV range is empty")
	}

	bytesPerFrame := int64(format.Channels) * wav.SampleBytes
	startByte := startFrame * bytesPerFrame
	endByte := endFrame * bytesPerFrame
	if startByte < 0 || endByte > int64(format.DataBytes) {
		return "", fmt.Errorf("turn WAV range is outside source audio")
	}

	temporaryFile, err := os.CreateTemp(temporaryDir, turn.UserID+"-*.wav")
	if err != nil {
		return "", fmt.Errorf("create temporary turn WAV: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer func() {
		if temporaryFile != nil {
			_ = temporaryFile.Close()
			_ = os.Remove(temporaryPath)
		}
	}()

	dataBytes := uint32(endByte - startByte)
	if err := writePCM16WAVHeader(temporaryFile, format.SampleRate, format.Channels, dataBytes); err != nil {
		return "", fmt.Errorf("write temporary WAV header: %w", err)
	}
	if _, err := source.Seek(wav.HeaderSize+startByte, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek source WAV: %w", err)
	}
	if _, err := io.CopyN(temporaryFile, source, int64(dataBytes)); err != nil {
		return "", fmt.Errorf("copy turn audio: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return "", fmt.Errorf("close temporary turn WAV: %w", err)
	}
	temporaryFile = nil

	return temporaryPath, nil
}

func readPCMWAVFormat(file *os.File) (pcmWAVFormat, error) {
	var header [wav.HeaderSize]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return pcmWAVFormat{}, err
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" || string(header[12:16]) != "fmt " || string(header[36:40]) != "data" {
		return pcmWAVFormat{}, fmt.Errorf("unsupported WAV header")
	}
	if format := binary.LittleEndian.Uint16(header[20:22]); format != 1 {
		return pcmWAVFormat{}, fmt.Errorf("WAV audio format %d is not PCM", format)
	}
	if bitsPerSample := binary.LittleEndian.Uint16(header[34:36]); bitsPerSample != 16 {
		return pcmWAVFormat{}, fmt.Errorf("WAV bit depth %d is not 16-bit", bitsPerSample)
	}

	format := pcmWAVFormat{
		SampleRate: binary.LittleEndian.Uint32(header[24:28]),
		Channels:   binary.LittleEndian.Uint16(header[22:24]),
		DataBytes:  binary.LittleEndian.Uint32(header[40:44]),
	}
	if format.SampleRate == 0 || format.Channels == 0 || int64(format.DataBytes)%(int64(format.Channels)*wav.SampleBytes) != 0 {
		return pcmWAVFormat{}, fmt.Errorf("invalid PCM WAV format")
	}

	return format, nil
}

func wavDurationToFrames(duration time.Duration, sampleRate uint32) (int64, error) {
	if duration < 0 {
		return 0, fmt.Errorf("duration must not be negative")
	}

	frames := duration.Nanoseconds() * int64(sampleRate)
	if frames%int64(time.Second) != 0 {
		return 0, fmt.Errorf("duration %s is not aligned to the WAV sample rate", duration)
	}
	return frames / int64(time.Second), nil
}

func writePCM16WAVHeader(file *os.File, sampleRate uint32, channels uint16, dataBytes uint32) error {
	header := make([]byte, wav.HeaderSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataBytes)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], channels)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	binary.LittleEndian.PutUint32(header[28:32], sampleRate*uint32(channels)*wav.SampleBytes)
	binary.LittleEndian.PutUint16(header[32:34], channels*wav.SampleBytes)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataBytes)

	_, err := file.Write(header)
	return err
}

func temporaryTurnWAVDirectory(sessionDirectory string) string {
	return filepath.Join(sessionDirectory, ".transcription-tmp")
}

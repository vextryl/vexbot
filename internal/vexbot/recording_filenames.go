package vexbot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vextryl/vexbot/internal/speaker"
	"github.com/vextryl/vexbot/internal/wav"
)

const maxRecordingFileStemBytes = 180

var windowsReservedFileNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

type recordingRename struct {
	original    string
	temporary   string
	destination string
}

func renameRecordingFiles(files []wav.File, displayNames map[string]string) ([]wav.File, error) {
	planned := make([]recordingRename, len(files))
	usedStems := make(map[string]struct{}, len(files))
	sourcePaths := make(map[string]struct{}, len(files))
	for _, file := range files {
		sourcePaths[filepath.Clean(file.Path)] = struct{}{}
	}
	for index, file := range files {
		stem := uniqueRecordingFileStem(speaker.Resolve(file.UserID.String(), displayNames), file.UserID.String(), usedStems)
		destination := filepath.Join(filepath.Dir(file.Path), stem+filepath.Ext(file.Path))
		if _, source := sourcePaths[filepath.Clean(destination)]; !source {
			if _, err := os.Stat(destination); err == nil {
				return nil, fmt.Errorf("recording destination already exists: %s", destination)
			} else if !os.IsNotExist(err) {
				return nil, fmt.Errorf("check recording destination: %w", err)
			}
		}
		planned[index] = recordingRename{
			original:    file.Path,
			temporary:   filepath.Join(filepath.Dir(file.Path), fmt.Sprintf(".vexbot-rename-%d%s", index, filepath.Ext(file.Path))),
			destination: destination,
		}
	}

	for index := range planned {
		if err := os.Rename(planned[index].original, planned[index].temporary); err != nil {
			return nil, errors.Join(
				fmt.Errorf("move recording to temporary path: %w", err),
				rollbackRecordingRenames(planned[:index]),
			)
		}
	}
	for index := range planned {
		if err := os.Rename(planned[index].temporary, planned[index].destination); err != nil {
			return nil, errors.Join(
				fmt.Errorf("rename recording: %w", err),
				rollbackRecordingRenames(planned),
			)
		}
	}

	renamed := append([]wav.File(nil), files...)
	for index := range renamed {
		renamed[index].Path = planned[index].destination
	}
	return renamed, nil
}

func rollbackRecordingRenames(planned []recordingRename) error {
	var rollbackErr error
	for _, rename := range planned {
		if _, err := os.Stat(rename.temporary); err == nil {
			rollbackErr = errors.Join(rollbackErr, os.Rename(rename.temporary, rename.original))
			continue
		}
		if _, err := os.Stat(rename.destination); err == nil {
			rollbackErr = errors.Join(rollbackErr, os.Rename(rename.destination, rename.original))
		}
	}
	return rollbackErr
}

func uniqueRecordingFileStem(displayName, userID string, used map[string]struct{}) string {
	stem := recordingFileStem(displayName, userID)
	if _, exists := used[strings.ToUpper(stem)]; !exists {
		used[strings.ToUpper(stem)] = struct{}{}
		return stem
	}

	suffix := " (" + userID + ")"
	stem = truncateUTF8(stem, maxRecordingFileStemBytes-len(suffix)) + suffix
	used[strings.ToUpper(stem)] = struct{}{}
	return stem
}

func recordingFileStem(displayName, userID string) string {
	name := strings.Map(func(character rune) rune {
		switch {
		case unicode.IsControl(character):
			return ' '
		case strings.ContainsRune(`<>:"/\\|?*`, character):
			return '-'
		default:
			return character
		}
	}, displayName)
	name = strings.Trim(strings.Join(strings.Fields(name), " "), ". ")
	if name == "" {
		return "user-" + userID
	}
	if _, reserved := windowsReservedFileNames[strings.ToUpper(name)]; reserved {
		name += "-user"
	}
	return truncateUTF8(name, maxRecordingFileStemBytes)
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.RuneStart(value[maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

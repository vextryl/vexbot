package main

import (
	"testing"
	"time"
)

func TestBuildTranscriptionTurnsGroupsNearbySpans(t *testing.T) {
	timeline := sessionTimeline{
		Version: 1,
		Spans: []timelineSpan{
			{UserID: "alex", SessionStartMS: 10000, SessionEndMS: 11000, WAVStartMS: 1000, WAVEndMS: 2000},
			{UserID: "sam", SessionStartMS: 1500, SessionEndMS: 2500, WAVStartMS: 0, WAVEndMS: 1000},
			{UserID: "alex", SessionStartMS: 5000, SessionEndMS: 6000, WAVStartMS: 0, WAVEndMS: 1000},
			{UserID: "alex", SessionStartMS: 12100, SessionEndMS: 13100, WAVStartMS: 2000, WAVEndMS: 3000},
			{UserID: "alex", SessionStartMS: 15100, SessionEndMS: 16100, WAVStartMS: 3000, WAVEndMS: 4000},
		},
	}

	got := buildTranscriptionTurns(timeline)
	want := []transcriptionTurn{
		{UserID: "sam", SessionStart: 1500 * time.Millisecond, SessionEnd: 2500 * time.Millisecond, WAVStart: 0, WAVEnd: time.Second},
		{UserID: "alex", SessionStart: 5 * time.Second, SessionEnd: 6 * time.Second, WAVStart: 0, WAVEnd: time.Second},
		{UserID: "alex", SessionStart: 10 * time.Second, SessionEnd: 13100 * time.Millisecond, WAVStart: time.Second, WAVEnd: 3 * time.Second},
		{UserID: "alex", SessionStart: 15100 * time.Millisecond, SessionEnd: 16100 * time.Millisecond, WAVStart: 3 * time.Second, WAVEnd: 4 * time.Second},
	}

	if len(got) != len(want) {
		t.Fatalf("turn count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("turn %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}

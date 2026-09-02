package session

import (
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestManagerReservePreventsDuplicateReservations(t *testing.T) {
	manager := NewManager(nil, nil)
	guildID := snowflake.ID(42)

	if !manager.Reserve(guildID) {
		t.Fatal("first Reserve() = false, want true")
	}
	if manager.Reserve(guildID) {
		t.Fatal("second Reserve() = true, want false")
	}

	manager.CancelReservation(guildID)
	if !manager.Reserve(guildID) {
		t.Fatal("Reserve() after CancelReservation() = false, want true")
	}
}

func TestManagerStartConsumesReservationAndKeepsSessionActive(t *testing.T) {
	manager := NewManager(nil, nil)
	guildID := snowflake.ID(42)

	if !manager.Reserve(guildID) {
		t.Fatal("Reserve() = false, want true")
	}
	manager.Start(&Session{guildID: guildID})

	if _, ok := manager.starting[guildID]; ok {
		t.Fatal("Start() left guild reservation in place")
	}
	if _, ok := manager.sessions[guildID]; !ok {
		t.Fatal("Start() did not register active session")
	}
	if manager.Reserve(guildID) {
		t.Fatal("Reserve() with active session = true, want false")
	}

	manager.CancelReservation(guildID)
	if manager.Reserve(guildID) {
		t.Fatal("CancelReservation() removed active session")
	}
}

package golibdave

import (
	"log/slog"
	"testing"

	"github.com/disgoorg/godave"
)

func TestDecryptPassthroughPreservesFrame(t *testing.T) {
	input := []byte{1, 2, 3, 4}
	output := make([]byte, len(input))
	session := NewSession(slog.Default(), "bot", testCallbacks{})

	n, err := session.Decrypt("speaker", input, output)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if n != len(input) {
		t.Fatalf("Decrypt() bytes = %d, want %d", n, len(input))
	}
	if string(output) != string(input) {
		t.Fatalf("Decrypt() output = %v, want %v", output, input)
	}
}

type testCallbacks struct{}

func (testCallbacks) SendMLSKeyPackage([]byte) error        { return nil }
func (testCallbacks) SendMLSCommitWelcome([]byte) error     { return nil }
func (testCallbacks) SendReadyForTransition(uint16) error   { return nil }
func (testCallbacks) SendInvalidCommitWelcome(uint16) error { return nil }

var _ godave.Callbacks = testCallbacks{}

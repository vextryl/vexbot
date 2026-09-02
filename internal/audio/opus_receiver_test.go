package audio

import (
	"testing"

	"github.com/disgoorg/disgo/voice"
)

func TestAudioReceiverDropsUnknownSpeakerPackets(t *testing.T) {
	receiver := NewOpusReceiver(nil, nil)

	if err := receiver.ReceiveOpusFrame(0, &voice.Packet{}); err != nil {
		t.Fatalf("ReceiveOpusFrame() error = %v", err)
	}
	if len(receiver.decoders) != 0 {
		t.Fatalf("created %d decoders for an unknown speaker, want 0", len(receiver.decoders))
	}
}

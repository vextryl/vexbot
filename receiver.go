package main

import (
	"fmt"
	"sync"

	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
)

const (
	opusSampleRate = 48000
	opusChannels   = 2
	maxOpusSamples = 5760
)

type decoderState struct {
	mu      sync.Mutex
	decoder *opus.Decoder
	pcm     []int16
}

type audioReceiver struct {
	mu       sync.Mutex
	decoders map[snowflake.ID]*decoderState
	sink     AudioSink
	session  *VoiceSession
}

func newAudioReceiver(sink AudioSink, session *VoiceSession) *audioReceiver {
	return &audioReceiver{
		decoders: make(map[snowflake.ID]*decoderState),
		sink:     sink,
		session:  session,
	}
}

func (r *audioReceiver) ReceiveOpusFrame(
	userID snowflake.ID,
	packet *voice.Packet,
) error {
	state, err := r.decoderState(userID)
	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	samples, err := state.decoder.Decode(
		packet.Opus,
		state.pcm,
	)
	if err != nil {
		return fmt.Errorf(
			"decode opus frame for user %v: %w",
			userID,
			err,
		)
	}

	frame := AudioFrame{
		UserID:     userID,
		Samples:    append([]int16(nil), state.pcm[:samples*opusChannels]...),
		SampleRate: opusSampleRate,
		Channels:   opusChannels,
		Timestamp:  r.session.Timestamp(),
	}

	r.sink.ConsumeAudioFrame(frame)

	return nil
}

func (r *audioReceiver) decoderState(userID snowflake.ID) (*decoderState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.decoders[userID]
	if ok {
		return state, nil
	}

	decoder, err := opus.NewDecoder(
		opusSampleRate,
		opusChannels,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create opus decoder for user %v: %w",
			userID,
			err,
		)
	}

	state = &decoderState{
		decoder: decoder,
		pcm:     make([]int16, maxOpusSamples*opusChannels),
	}

	r.decoders[userID] = state

	fmt.Printf("AUDIO: created decoder for user=%v\n", userID)

	return state, nil
}

func (r *audioReceiver) CleanupUser(userID snowflake.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.decoders, userID)

	fmt.Printf("AUDIO: cleaned up decoder for user=%v\n", userID)
}

func (r *audioReceiver) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.decoders = make(map[snowflake.ID]*decoderState)

	fmt.Println("AUDIO: receiver closed")
}

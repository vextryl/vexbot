package main

import (
	"fmt"
	"sync"

	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
)

type audioReceiver struct {
	mu       sync.Mutex
	decoders map[snowflake.ID]*opus.Decoder
}

const (
	opusSampleRate = 48000
	opusChannels   = 2
)

func newAudioReceiver() *audioReceiver {
	return &audioReceiver{
		decoders: make(map[snowflake.ID]*opus.Decoder),
	}
}

func (r *audioReceiver) ReceiveOpusFrame(
	userID snowflake.ID,
	packet *voice.Packet,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	decoder, ok := r.decoders[userID]
	if !ok {
		var err error

		decoder, err = opus.NewDecoder(
			opusSampleRate,
			opusChannels,
		)
		if err != nil {
			return fmt.Errorf(
				"create opus decoder for user %v: %w",
				userID,
				err,
			)
		}

		r.decoders[userID] = decoder

		fmt.Printf("AUDIO: created decoder for user=%v\n", userID)
	}

	pcm := make([]int16, 960*opusChannels)

	_, err := decoder.Decode(
		packet.Opus,
		pcm,
	)
	if err != nil {
		return fmt.Errorf(
			"decode opus frame for user %v: %w",
			userID,
			err,
		)
	}

	return nil
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

	r.decoders = make(map[snowflake.ID]*opus.Decoder)

	fmt.Println("AUDIO: receiver closed")
}

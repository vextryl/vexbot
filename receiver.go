package main

import (
	"fmt"

	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
)

type audioReceiver struct{}

func (r *audioReceiver) ReceiveOpusFrame(
	userID snowflake.ID,
	packet *voice.Packet,
) error {
	fmt.Printf(
		"AUDIO: user=%v frame=%d bytes\n",
		userID,
		len(packet.Opus),
	)

	return nil
}

func (r *audioReceiver) CleanupUser(userID snowflake.ID) {
	fmt.Printf("AUDIO: cleanup user=%v\n", userID)
}

func (r *audioReceiver) Close() {
	fmt.Println("AUDIO: receiver closed")
}

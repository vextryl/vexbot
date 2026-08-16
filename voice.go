package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave/golibdave"
	"github.com/disgoorg/snowflake/v2"
)

func botUserIDFromToken(token string) (snowflake.ID, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid Discord bot token")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to decode bot token: %w", err)
	}

	var id uint64
	_, err = fmt.Sscanf(string(decoded), "%d", &id)
	if err != nil {
		return 0, fmt.Errorf("failed to parse bot user ID: %w", err)
	}

	return snowflake.ID(id), nil
}

func newVoiceManager(
	updateVoiceState func(context.Context, snowflake.ID, *snowflake.ID, bool, bool) error, userID snowflake.ID,
) voice.Manager {
	return voice.NewManager(
		func(
			ctx context.Context,
			guildID snowflake.ID,
			channelID *snowflake.ID,
			selfMute bool,
			selfDeaf bool,
		) error {
			return updateVoiceState(
				ctx,
				guildID,
				channelID,
				selfMute,
				selfDeaf,
			)
		},
		userID,
		voice.WithDaveSessionCreateFunc(golibdave.NewSession),
	)
}

func joinUserVoiceChannel(
	ctx context.Context,
	manager voice.Manager,
	caches cache.Caches,
	guildID snowflake.ID,
	userID snowflake.ID,
) error {
	voiceState, ok := caches.VoiceState(guildID, userID)
	if !ok {
		return fmt.Errorf("user is not in a voice channel")
	}

	if voiceState.ChannelID == nil {
		return fmt.Errorf("user is not in a voice channel")
	}

	conn := manager.CreateConn(guildID)

	err := conn.Open(
		ctx,
		*voiceState.ChannelID,
		true,  // selfMute
		false, // selfDeaf
	)
	if err != nil {
		return err
	}

	sink := newDiscardAudioSink()
	receiver := newAudioReceiver(sink)
	conn.SetOpusFrameReceiver(receiver)

	return nil
}

package discord

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/audio"
	"github.com/vextryl/vexbot/internal/dave"
	"github.com/vextryl/vexbot/internal/session"
)

// BotUserIDFromToken extracts the Discord application ID embedded in a bot
// token for voice-manager setup.
func BotUserIDFromToken(token string) (snowflake.ID, error) {
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

// NewVoiceManager creates the Discord voice manager used by VexBot.
func NewVoiceManager(
	updateVoiceState func(context.Context, snowflake.ID, *snowflake.ID, bool, bool) error, userID snowflake.ID, logger *slog.Logger,
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
		voice.WithLogger(dave.NewRateLimitedLogger(logger)),
		voice.WithDaveSessionCreateFunc(dave.NewSession),
	)
}

// UserVoiceChannelID returns the command user's current voice channel.
func UserVoiceChannelID(caches cache.Caches, guildID, userID snowflake.ID) (snowflake.ID, error) {
	voiceState, ok := caches.VoiceState(guildID, userID)
	if !ok || voiceState.ChannelID == nil {
		return 0, fmt.Errorf("user is not in a voice channel")
	}
	return *voiceState.ChannelID, nil
}

// JoinUserVoiceChannel creates a recording session in a known voice channel.
func JoinUserVoiceChannel(
	ctx context.Context,
	manager voice.Manager,
	guildID snowflake.ID,
	userID snowflake.ID,
	voiceChannelID snowflake.ID,
	transcriptChannelID snowflake.ID,
	logger *slog.Logger,
) (*session.Session, snowflake.ID, error) {
	conn := manager.CreateConn(guildID)
	voiceSession, err := session.New(guildID, userID, voiceChannelID, transcriptChannelID, conn, logger)
	if err != nil {
		return nil, 0, err
	}

	err = conn.Open(
		ctx,
		voiceChannelID,
		true,  // selfMute
		false, // selfDeaf
	)
	if err != nil {
		_ = voiceSession.Abort()
		manager.RemoveConn(guildID)
		return nil, 0, err
	}

	receiver := audio.NewOpusReceiver(voiceSession.AudioBuffer(), voiceSession, logger)
	conn.SetOpusFrameReceiver(receiver)

	return voiceSession, voiceChannelID, nil
}

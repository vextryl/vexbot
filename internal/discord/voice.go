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
		voice.WithLogger(dave.NewRateLimitedLogger(slog.Default())),
		voice.WithDaveSessionCreateFunc(dave.NewSession),
	)
}

// JoinUserVoiceChannel creates a recording session in the command user's
// current voice channel.
func JoinUserVoiceChannel(
	ctx context.Context,
	manager voice.Manager,
	caches cache.Caches,
	guildID snowflake.ID,
	userID snowflake.ID,
) (*session.Session, error) {
	voiceState, ok := caches.VoiceState(guildID, userID)
	if !ok {
		return nil, fmt.Errorf("user is not in a voice channel")
	}

	if voiceState.ChannelID == nil {
		return nil, fmt.Errorf("user is not in a voice channel")
	}

	conn := manager.CreateConn(guildID)
	voiceSession, err := session.New(guildID, userID, conn)
	if err != nil {
		return nil, err
	}

	err = conn.Open(
		ctx,
		*voiceState.ChannelID,
		true,  // selfMute
		false, // selfDeaf
	)
	if err != nil {
		_ = voiceSession.Abort()
		manager.RemoveConn(guildID)
		return nil, err
	}

	receiver := audio.NewOpusReceiver(voiceSession.AudioBuffer(), voiceSession)
	conn.SetOpusFrameReceiver(receiver)

	return voiceSession, nil
}

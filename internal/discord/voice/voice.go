// Package voice manages VexBot's Discord voice connections and recording setup.
package voice

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/audio"
	"github.com/vextryl/vexbot/internal/dave"
	"github.com/vextryl/vexbot/internal/session"
)

// NewVoiceManager creates the Discord voice manager used by VexBot.
func NewVoiceManager(
	updateVoiceState func(context.Context, snowflake.ID, *snowflake.ID, bool, bool) error,
	userID snowflake.ID,
	logger *slog.Logger,
	failures *dave.DecryptFailureCounter,
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
		voice.WithLogger(logger),
		voice.WithConnCreateFunc(recordingConnCreateFunc(logger, failures)),
		voice.WithDaveSessionCreateFunc(dave.NewSession),
	)
}

func recordingConnCreateFunc(logger *slog.Logger, failures *dave.DecryptFailureCounter) voice.ConnCreateFunc {
	return func(
		guildID snowflake.ID,
		userID snowflake.ID,
		voiceStateUpdateFunc voice.StateUpdateFunc,
		removeConnFunc func(),
		opts ...voice.ConnConfigOpt,
	) voice.Conn {
		failures.Reset(guildID)
		opts = append(opts, voice.WithConnLogger(dave.NewRecordingLogger(logger, failures, guildID)))
		return voice.NewConn(guildID, userID, voiceStateUpdateFunc, removeConnFunc, opts...)
	}
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
	failures *dave.DecryptFailureCounter,
	logger *slog.Logger,
) (*session.Session, snowflake.ID, error) {
	if failures != nil {
		failures.Reset(guildID)
	}
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

package vexbot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
	"github.com/joho/godotenv"
)

func Run() {
	// load discord token from .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
	}

	// parse token
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		fmt.Println("Error: DISCORD_TOKEN not found in .env file")
		return
	}

	// parse guild ID
	guildID := os.Getenv("DISCORD_GUILD_ID")
	if guildID == "" {
		fmt.Println("Error: DISCORD_GUILD_ID is not set")
		return
	}

	transcriber, err := newWhisperTranscriberFromEnv()
	if err != nil {
		fmt.Println("Error configuring local transcription:", err)
		return
	}
	if transcriber == nil {
		fmt.Println("local transcription is not configured")
	}

	// disgo requires botUserID for the voice manager
	botUserID, err := botUserIDFromToken(token)
	if err != nil {
		fmt.Println("Error getting bot user ID:", err)
		return
	}

	// The voice manager needs to update the bot's voice state,
	// but the client needs the voice manager during construction.
	// A closure lets us resolve that dependency after the client exists.
	var client *bot.Client

	voiceManager := newVoiceManager(
		func(
			ctx context.Context,
			guildID snowflake.ID,
			channelID *snowflake.ID,
			selfMute bool,
			selfDeaf bool,
		) error {
			return client.UpdateVoiceState(
				ctx,
				guildID,
				channelID,
				selfMute,
				selfDeaf,
			)
		},
		botUserID,
	)
	sessions := NewSessionManager(transcriber)

	client, err = disgo.New(
		token,
		bot.WithDefaultGateway(),
		bot.WithVoiceManager(voiceManager),
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildVoiceStates,
			),
		),
		bot.WithCacheConfigOpts(
			cache.WithCaches(
				cache.FlagGuilds,
				cache.FlagMembers,
				cache.FlagChannels,
				cache.FlagVoiceStates,
			),
		),
	)
	if err != nil {
		fmt.Println("Error creating Discord client:", err)
		return
	}

	// defer the closing of the Discord session until the program exits
	defer client.Close(context.Background())
	defer func() {
		if err := sessions.Close(context.Background()); err != nil {
			fmt.Println("Error finalizing recordings:", err)
		}
	}()

	// add event listeners
	client.AddEventListeners(
		&events.ListenerAdapter{
			OnApplicationCommandInteraction: func(event *events.ApplicationCommandInteractionCreate) {
				onApplicationCommandInteraction(event, client, voiceManager, sessions)
			},
			OnGuildVoiceStateUpdate: func(event *events.GuildVoiceStateUpdate) {
				channelID := "<nil>"
				if event.VoiceState.ChannelID != nil {
					channelID = event.VoiceState.ChannelID.String()
				}

				fmt.Printf(
					"VOICE STATE: guild=%v user=%v channel=%s\n",
					event.VoiceState.GuildID,
					event.VoiceState.UserID,
					channelID,
				)
			},
			OnVoiceServerUpdate: func(event *events.VoiceServerUpdate) {
				fmt.Printf(
					"VOICE SERVER: guild=%v endpoint=%v\n",
					event.GuildID,
					event.Endpoint,
				)
			},
		},
	)

	// open the Discord gateway to start receiving events
	err = client.OpenGateway(context.Background())
	if err != nil {
		fmt.Println("Error opening Discord gateway:", err)
		return
	}

	// status
	fmt.Println("vexbot connected to discord")

	err = registerCommands(client, guildID)
	if err != nil {
		fmt.Println("Error registering commands:", err)
		return
	}

	// status
	fmt.Println("registered commands")

	// Wait for a termination signal to gracefully shut down the bot
	// otherwise program will exit immediately
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

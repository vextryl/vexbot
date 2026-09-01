package main

import (
	"context"
	"fmt"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/voice"
)

func onApplicationCommandInteraction(
	event *events.ApplicationCommandInteractionCreate,
	client *bot.Client,
	voiceManager voice.Manager,
	sessions *SessionManager,
) {
	switch event.SlashCommandInteractionData().CommandName() {
	case "ping":
		handlePing(event)

	case "join":
		handleJoin(event, client, voiceManager, sessions)

	case "stop":
		handleStop(event, sessions)
	}
}

func handlePing(event *events.ApplicationCommandInteractionCreate) {
	err := event.CreateMessage(discord.MessageCreate{
		Content: "Pong!",
	})
	if err != nil {
		fmt.Println("Error responding to /ping:", err)
	}
}

func handleJoin(
	event *events.ApplicationCommandInteractionCreate,
	client *bot.Client,
	voiceManager voice.Manager,
	sessions *SessionManager,
) {
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "This command can only be used in a server.",
		})
		return
	}

	user := event.User()
	if !sessions.Reserve(*guildID) {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "I am already recording or connecting in this server.",
		})
		return
	}

	err := event.CreateMessage(discord.MessageCreate{
		Content: "Attempting to join your voice channel...",
	})
	if err != nil {
		sessions.CancelReservation(*guildID)
		fmt.Println("Error responding to /join:", err)
		return
	}

	go func() {
		session, err := joinUserVoiceChannel(
			context.Background(),
			voiceManager,
			client.Caches,
			*guildID,
			user.ID,
		)
		if err != nil {
			sessions.CancelReservation(*guildID)
			fmt.Println("Error joining voice channel:", err)
			return
		}

		sessions.Start(session)
		fmt.Printf("Successfully joined voice channel; recording to %s\n", session.RecorderDirectory())
	}()
}

func handleStop(event *events.ApplicationCommandInteractionCreate, sessions *SessionManager) {
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "This command can only be used in a server.",
		})
		return
	}

	session, err := sessions.Stop(context.Background(), *guildID, event.User().ID)
	if err != nil {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "Unable to stop recording: " + err.Error(),
		})
		return
	}

	fmt.Printf("Recording stopped; files saved to %s\n", session.Directory)
	message := "Recording stopped and I left the voice channel."
	if sessions.StartTranscription(session) {
		message += " Local transcription has started."
	} else {
		message += " Local transcription is not configured."
	}
	_ = event.CreateMessage(discord.MessageCreate{
		Content: message,
	})
}

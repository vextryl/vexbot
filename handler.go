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
) {
	switch event.SlashCommandInteractionData().CommandName() {
	case "ping":
		handlePing(event)

	case "join":
		handleJoin(event, client, voiceManager)
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
) {
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "This command can only be used in a server.",
		})
		return
	}

	user := event.User()

	err := event.CreateMessage(discord.MessageCreate{
		Content: "Attempting to join your voice channel...",
	})
	if err != nil {
		fmt.Println("Error responding to /join:", err)
		return
	}

	go func() {
		err := joinUserVoiceChannel(
			context.Background(),
			voiceManager,
			client.Caches,
			*guildID,
			user.ID,
		)
		if err != nil {
			fmt.Println("Error joining voice channel:", err)
			return
		}

		fmt.Println("Successfully joined voice channel")
	}()
}

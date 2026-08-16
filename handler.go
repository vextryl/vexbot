package main

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

func onApplicationCommandInteraction(event *events.ApplicationCommandInteractionCreate) {
	switch event.Data.CommandName() {
	case "ping":
		handlePing(event)
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

package main

import (
	"github.com/bwmarrin/discordgo"
)

// pingCommand is a simple command that responds with "Pong!" when invoked.
var pingCommand = &discordgo.ApplicationCommand{
	Name:        "ping",
	Description: "Check if vexbot is alive",
}

func registerCommands(discord *discordgo.Session, guildID string) error {
	commands := []*discordgo.ApplicationCommand{
		pingCommand,
	}

	for _, command := range commands {
		_, err := discord.ApplicationCommandCreate(
			discord.State.User.ID,
			guildID,
			command,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

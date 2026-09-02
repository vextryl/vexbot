package discord

import (
	"fmt"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

var pingCommand = discord.SlashCommandCreate{
	Name:        "ping",
	Description: "Check if vexbot is alive",
}

var joinCommand = discord.SlashCommandCreate{
	Name:        "join",
	Description: "Join your current voice channel",
}

var stopCommand = discord.SlashCommandCreate{
	Name:        "stop",
	Description: "Stop recording and leave the voice channel",
}

// RegisterCommands installs VexBot's slash commands for a development guild.
func RegisterCommands(client *bot.Client, guildID string) error {
	guild, err := snowflake.Parse(guildID)
	if err != nil {
		return err
	}

	commands := []discord.SlashCommandCreate{
		pingCommand,
		joinCommand,
		stopCommand,
	}

	for _, command := range commands {
		_, err = client.Rest.CreateGuildCommand(
			client.ID(),
			guild,
			command,
		)
		if err != nil {
			return err
		}
		fmt.Printf("registered command: %s\n", command.Name)
	}

	return nil
}

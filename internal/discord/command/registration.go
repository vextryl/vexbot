// Package command registers and handles VexBot's Discord slash commands.
package command

import (
	"log/slog"

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

var statusCommand = discord.SlashCommandCreate{
	Name:        "status",
	Description: "Show the current recording status",
}

// RegisterCommands installs VexBot's slash commands for a development guild.
func RegisterCommands(client *bot.Client, guildID string, logger *slog.Logger) error {
	guild, err := snowflake.Parse(guildID)
	if err != nil {
		return err
	}

	commands := []discord.SlashCommandCreate{
		pingCommand,
		joinCommand,
		stopCommand,
		statusCommand,
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
		if logger != nil {
			logger.Info("registered command", slog.String("command", command.Name))
		}
	}

	return nil
}

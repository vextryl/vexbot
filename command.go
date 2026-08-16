package main

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

var pingCommand = discord.SlashCommandCreate{
	Name:        "ping",
	Description: "Check if vexbot is alive",
}

func registerCommands(client *bot.Client, guildID string) error {
	guild, err := snowflake.Parse(guildID)
	if err != nil {
		return err
	}

	_, err = client.Rest.CreateGuildCommand(
		client.ID(),
		guild,
		pingCommand,
	)

	return err
}

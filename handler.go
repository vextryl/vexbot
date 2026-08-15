package main

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// onReady is called when the bot has successfully connected to Discord and is ready to start receiving events.
func onReady(discord *discordgo.Session, event *discordgo.Ready) {
	fmt.Printf("vexbot connect as %s#%s\n", event.User.Username, event.User.Discriminator)
}

func onInteractionCreate(discord *discordgo.Session, interaction *discordgo.InteractionCreate) {
	// log the interaction type for debugging purposes
	fmt.Printf("received interaction: %s\n", interaction.Type)

	// check if the interaction is an application command
	if interaction.Type != discordgo.InteractionApplicationCommand {
		return
	}

	switch interaction.ApplicationCommandData().Name {
	case "ping":
		err := discord.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Pong!",
			},
		})
		if err != nil {
			fmt.Println("Error responding to /ping:", err)
		}
	}
}

package main

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// onReady is called when the bot has successfully connected to Discord and is ready to start receiving events.
func onReady(discord *discordgo.Session, event *discordgo.Ready) {
	fmt.Printf("vexbot connect as %s#%s\n", event.User.Username, event.User.Discriminator)
}

package main

import (
	"github.com/bwmarrin/discordgo"
)

// pingCommand is a simple command that responds with "Pong!" when invoked.
var pingCommand = &discordgo.ApplicationCommand{
	Name:        "ping",
	Description: "Check if vexbot is alive",
}

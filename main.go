package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/joho/godotenv"
)

func main() {
	// load discord token from .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
	}

	// parse token
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		fmt.Println("Error: DISCORD_TOKEN not found in .env file")
		return
	}

	// parse guild ID
	guildID := os.Getenv("DISCORD_GUILD_ID")
	if guildID == "" {
		fmt.Println("Error: DISCORD_GUILD_ID is not set")
		return
	}

	client, err := disgo.New(
		token,
		bot.WithDefaultGateway(),
	)
	if err != nil {
		fmt.Println("Error creating Discord client:", err)
		return
	}

	// defer the closing of the Discord session until the program exits
	defer client.Close(context.Background())

	// add event listeners
	client.AddEventListeners(
		&events.ListenerAdapter{
			OnApplicationCommandInteraction: onApplicationCommandInteraction,
		},
	)

	// open the Discord gateway to start receiving events
	err = client.OpenGateway(context.Background())
	if err != nil {
		fmt.Println("Error opening Discord gateway:", err)
		return
	}

	// status
	fmt.Println("vexbot connected to discord")

	err = registerCommands(client, guildID)
	if err != nil {
		fmt.Println("Error registering commands:", err)
		return
	}

	// status
	fmt.Println("registered commands")

	// Wait for a termination signal to gracefully shut down the bot - otherwise program will exit immediately
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

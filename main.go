package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
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

	// make a new Discord session using the provided bot token
	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating Discord session:", err)
		return
	}

	// open a websocket connection to Discord and begin listening
	err = discord.Open()
	if err != nil {
		fmt.Println("Error opening Discord connection:", err)
		return
	}

	// status
	fmt.Println("vexbot connected to discord")

	// defer the closing of the Discord session until the program exits
	defer discord.Close()

	// Wait for a termination signal to gracefully shut down the bot - otherwise program will exit immediately
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()

	if err != nil {
		fmt.Println("Error loading .env file")
	}

	token := os.Getenv("DISCORD_TOKEN")

	if token == "" {
		fmt.Println("Error: DISCORD_TOKEN not found in .env file")
		return
	}

	fmt.Println("Discord token loaded successfully.")
}

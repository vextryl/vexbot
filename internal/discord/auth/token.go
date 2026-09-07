// Package auth validates Discord authentication values needed during startup.
package auth

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/disgoorg/snowflake/v2"
)

// BotUserIDFromToken extracts the Discord application ID embedded in a bot
// token for startup validation and voice-manager setup.
func BotUserIDFromToken(token string) (snowflake.ID, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid Discord bot token")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to decode bot token: %w", err)
	}

	var id uint64
	_, err = fmt.Sscanf(string(decoded), "%d", &id)
	if err != nil {
		return 0, fmt.Errorf("failed to parse bot user ID: %w", err)
	}

	return snowflake.ID(id), nil
}

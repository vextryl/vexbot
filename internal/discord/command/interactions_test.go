package command

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/session"
)

func TestJoinAcknowledgmentFailureStopsBeforeLocalWork(t *testing.T) {
	event := &events.ApplicationCommandInteractionCreate{
		Respond: func(kind discord.InteractionResponseType, _ discord.InteractionResponseData, _ ...rest.RequestOpt) error {
			if kind != discord.InteractionResponseTypeDeferredCreateMessage {
				t.Fatalf("response type = %v", kind)
			}
			return errors.New("interaction expired")
		},
	}
	// Nil dependencies ensure no cache, storage, or voice work follows a failed acknowledgment.
	handleJoin(context.Background(), event, nil, nil, nil, 5, nil, nil)
}

func TestJoinDefersBeforeStorageAndEditsFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	var interaction discord.ApplicationCommandInteraction
	if err := json.Unmarshal([]byte(`{"id":"1","application_id":"2","type":2,"token":"test","guild_id":"3","channel":{"id":"4","type":0},"member":{"user":{"id":"5","username":"test"}},"data":{"id":"6","name":"join","type":1}}`), &interaction); err != nil {
		t.Fatal(err)
	}
	acknowledged := false
	event := &events.ApplicationCommandInteractionCreate{
		ApplicationCommandInteraction: interaction,
		Respond: func(kind discord.InteractionResponseType, _ discord.InteractionResponseData, _ ...rest.RequestOpt) error {
			if acknowledged {
				t.Fatal("attempted a second initial response")
			}
			if kind != discord.InteractionResponseTypeDeferredCreateMessage {
				t.Fatalf("response type = %v", kind)
			}
			acknowledged = true
			// Make storage preparation fail only after the acknowledgment. If pruning
			// moves before this response, this failure path will no longer be taken.
			return os.WriteFile("recordings", []byte("not a directory"), 0600)
		},
	}
	sender := &joinTestREST{}
	caches := &joinTestCaches{t: t, acknowledged: &acknowledged}
	manager := session.NewManager(nil, nil)
	handleJoin(context.Background(), event, &bot.Client{Rest: sender, Caches: caches}, nil, manager, 5, nil, nil)
	if !acknowledged {
		t.Fatal("interaction was not acknowledged")
	}
	if len(sender.messages) != 1 || !strings.Contains(sender.messages[0], "Unable to prepare recording storage") {
		t.Fatalf("response edits = %v", sender.messages)
	}
	if _, reserved := manager.VoiceChannelID(3); reserved {
		t.Fatal("failed preparation left a reservation behind")
	}
}

type joinTestCaches struct {
	cache.Caches
	t            *testing.T
	acknowledged *bool
}

func (c *joinTestCaches) VoiceState(guildID, userID snowflake.ID) (discord.VoiceState, bool) {
	if !*c.acknowledged {
		c.t.Fatal("cache lookup preceded acknowledgment")
	}
	channel := snowflake.ID(7)
	return discord.VoiceState{GuildID: guildID, UserID: userID, ChannelID: &channel}, true
}

type joinTestREST struct {
	rest.Rest
	messages []string
}

func (r *joinTestREST) UpdateInteractionResponse(_ snowflake.ID, _ string, update discord.MessageUpdate, _ ...rest.RequestOpt) (*discord.Message, error) {
	r.messages = append(r.messages, *update.Content)
	return &discord.Message{}, nil
}

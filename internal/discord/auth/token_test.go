package auth

import "testing"

func TestBotUserIDFromToken(t *testing.T) {
	userID, err := BotUserIDFromToken("MTIzNDU2.NA.signature")
	if err != nil {
		t.Fatalf("BotUserIDFromToken() error = %v", err)
	}
	if want := "123456"; userID.String() != want {
		t.Fatalf("BotUserIDFromToken() = %s, want %s", userID, want)
	}
}

func TestBotUserIDFromTokenRejectsInvalidPrefix(t *testing.T) {
	for _, token := range []string{"", "not-base64.token", "bm90LWEtbnVtYmVy.token"} {
		if _, err := BotUserIDFromToken(token); err == nil {
			t.Fatalf("BotUserIDFromToken(%q) error = nil, want error", token)
		}
	}
}

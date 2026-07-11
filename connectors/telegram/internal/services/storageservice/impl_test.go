package storageservice

import (
	"testing"

	connectordomain "github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/domain"
)

func TestStoragePersistsBotsAndBindings(t *testing.T) {
	stateDir := t.TempDir()
	service := newService(stateDir, "im.telegram")
	bot := &connectordomain.Bot{
		BotID:        "123",
		BotToken:     "bot-token",
		Username:     "xagent_bot",
		DisplayName:  "xAgent Bot",
		CreatedAt:    100,
		UpdateOffset: 42,
	}
	binding := &connectordomain.ConnectionBinding{
		ConnectorChannelID: "cch_a",
		BotID:              "123",
		ChatID:             "987",
		ChatType:           "private",
		ChatTitle:          "Coffee",
		CreatedAt:          200,
	}
	if err := service.SaveBot(bot); err != nil {
		t.Fatalf("SaveBot failed: %v", err)
	}
	if err := service.SaveConnectionBinding(binding); err != nil {
		t.Fatalf("SaveConnectionBinding failed: %v", err)
	}

	reloaded := newService(stateDir, "im.telegram")
	bots, err := reloaded.LoadBots()
	if err != nil {
		t.Fatalf("LoadBots failed: %v", err)
	}
	if bots["123"] == nil || bots["123"].BotToken != "bot-token" || bots["123"].UpdateOffset != 42 {
		t.Fatalf("bot state mismatch: %+v", bots["123"])
	}
	bindings, err := reloaded.LoadConnectionBindings()
	if err != nil {
		t.Fatalf("LoadConnectionBindings failed: %v", err)
	}
	if bindings["cch_a"] == nil || bindings["cch_a"].ChatID != "987" || bindings["cch_a"].BotID != "123" {
		t.Fatalf("binding state mismatch: %+v", bindings["cch_a"])
	}
}

func TestStoragePersistsMediaReferences(t *testing.T) {
	stateDir := t.TempDir()
	service := newService(stateDir, "im.telegram")
	reference := &connectordomain.MediaReference{
		Ref:                "tgmedia_a",
		Direction:          "inbound",
		ConnectorChannelID: "cch_a",
		BotID:              "123",
		BotToken:           "bot-token",
		ChatID:             "987",
		FileID:             "file-id",
		MediaType:          "file",
		Filename:           "a.pdf",
		ContentType:        "application/pdf",
		ByteSize:           1024,
		CreatedAt:          100,
		ExpiresAt:          10000,
	}
	if err := service.SaveMediaReference(reference); err != nil {
		t.Fatalf("SaveMediaReference failed: %v", err)
	}
	reloaded := newService(stateDir, "im.telegram")
	loaded, err := reloaded.GetMediaReference("tgmedia_a", 200)
	if err != nil {
		t.Fatalf("GetMediaReference failed: %v", err)
	}
	if loaded == nil || loaded.FileID != "file-id" || loaded.MediaType != "file" {
		t.Fatalf("media reference mismatch: %+v", loaded)
	}
	expired, err := reloaded.GetMediaReference("tgmedia_a", 10000)
	if err != nil {
		t.Fatalf("GetMediaReference expired failed: %v", err)
	}
	if expired != nil {
		t.Fatalf("expected expired media reference to be nil: %+v", expired)
	}
}

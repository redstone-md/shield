package main

import (
	"time"

	"github.com/umputun/tg-spam/app/events"
)

func loggableTelegramListenerConfig(listener *events.TelegramListener) telegramListenerLogConfig {
	return telegramListenerLogConfig{
		Bot:                 listener.BotUsername,
		Group:               listener.Group,
		Idle:                listener.IdleDuration,
		SuperUsers:          listener.SuperUsers,
		Admin:               listener.AdminGroup,
		TestingIDs:          listener.TestingIDs,
		NoSpamReply:         listener.NoSpamReply,
		SuppressJoinMessage: listener.SuppressJoinMessage,
		Dry:                 listener.Dry,
		TrainingMode:        listener.TrainingMode,
	}
}

type telegramListenerLogConfig struct {
	Bot                 string
	Group               string
	Idle                time.Duration
	SuperUsers          events.SuperUsers
	Admin               string
	TestingIDs          []int64
	NoSpamReply         bool
	SuppressJoinMessage bool
	Dry                 bool
	TrainingMode        bool
}

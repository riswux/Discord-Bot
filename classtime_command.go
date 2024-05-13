package main

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
)

// ==================================CLASS TIME, DELETE TIME===========================================
func setClassTime(s *discordgo.Session, m *discordgo.MessageCreate, classTime string) {
	// Load the GMT+7 timezone
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Failed to load timezone data.")
		return
	}

	parsedTime, err := time.ParseInLocation("15:04", classTime, location)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Invalid time format. Please use format HH:MM.")
		return
	}

	utcTime := parsedTime.UTC().Add(-17 * time.Minute)

	classTimes[m.GuildID] = utcTime

	showClassTime(s, m)

	// s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Class time set to %s GMT+7.", utcTime.Format("15:04")))
}

func showClassTime(s *discordgo.Session, m *discordgo.MessageCreate) {
	if utcClassTime, ok := classTimes[m.GuildID]; ok {
		// Convert UTC time to GMT+7 for display
		location, _ := time.LoadLocation("Asia/Bangkok") // GMT+7
		localTime := utcClassTime.In(location).Add(17 * time.Minute)

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Current class time is %s.", localTime.Format("15:04")))
	} else {
		s.ChannelMessageSend(m.ChannelID, "No class time is set.")
	}
}

func deleteClassTime(s *discordgo.Session, m *discordgo.MessageCreate, classTime string) {
	_, err := time.Parse("15:04", classTime)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Invalid time format. Please use format HH:MM.")
		return
	}

	if _, ok := classTimes[m.GuildID]; !ok {
		s.ChannelMessageSend(m.ChannelID, "Class time is not set.")
		return
	}

	delete(classTimes, m.GuildID)
	s.ChannelMessageSend(m.ChannelID, "Class time deleted.")
}

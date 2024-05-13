package main

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
)

// =========================================Reaction Role========================================================
func findRoleByNameReac(s *discordgo.Session, guildID, roleName string) (string, error) {
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		return "", err
	}

	for _, role := range roles {
		if role.Name == roleName {
			return role.ID, nil
		}
	}
	return "", fmt.Errorf("role %s not found", roleName)
}

func handleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd, messageID string, roleID string) {
	if r.MessageID != messageID || r.Emoji.Name != "✅" {
		return
	}

	err := s.GuildMemberRoleAdd(r.GuildID, r.UserID, roleID)
	if err != nil {
		s.ChannelMessageSend(r.ChannelID, fmt.Sprintf("Failed to assign role: %v", err))
		return
	}
}

func handleReactionRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove, messageID string, roleID string) {
	if r.MessageID != messageID || r.Emoji.Name != "✅" {
		return
	}

	err := s.GuildMemberRoleRemove(r.GuildID, r.UserID, roleID)
	if err != nil {
		s.ChannelMessageSend(r.ChannelID, fmt.Sprintf("Failed to remove role: %v", err))
		return
	}
}

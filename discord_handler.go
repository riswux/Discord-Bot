package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
)

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate, db *sql.DB) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	startTime := time.Now()

	switch {
	case m.Content == "!help":
		HelpCommand(s, m)
		return

	case strings.Contains(m.Content, "!ping"):
		sentMsg, _ := s.ChannelMessageSend(m.ChannelID, "Pong!")
		endTime := time.Now()
		responseTime := endTime.Sub(startTime).Milliseconds()
		responseMsg := fmt.Sprintf("Pong! (%d ms)", responseTime)
		s.ChannelMessageEdit(m.ChannelID, sentMsg.ID, responseMsg)

		//===========================================MARK LIST ATTENDANCE==============================================================
	case strings.HasPrefix(m.Content, "!marklistnow"), strings.HasPrefix(m.Content, "!mn"):
		args := strings.Fields(m.Content)
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "Usage: `!marklistnow [voice channel name] [time]` or !mn `[voice channel name] [time]`")
			return
		}

		voiceChannelName := args[1]
		timeStr := args[2]

		handleMarkListNow(s, m, voiceChannelName, timeStr)

		//===========================================MARKLIST G-SHEET==============================================================
	case strings.HasPrefix(m.Content, "!marksheet "), strings.HasPrefix(m.Content, "!ms "):
		args := strings.Fields(m.Content)
		if len(args) == 2 && (args[1] == "stop") {
			updateDuration[m.GuildID] = false
			classEndTimes[m.GuildID] = time.Now().UTC().Add(-5 * time.Minute) // Set the end time to now
			s.ChannelMessageSend(m.ChannelID, "Attendance updates have been stopped.")
		} else if len(args) >= 2 && args[1] != "now" {
			updateDuration[m.GuildID] = true // Ensure we set true when starting a new session
			sheetName := strings.Join(args[1:], " ")
			manageAttendanceSheet(s, m, sheetName)
		} else if len(args) >= 3 && args[1] == "now" {
			updateDuration[m.GuildID] = true
			sheetName := strings.Join(args[2:], " ")
			classTimes[m.GuildID] = time.Now().UTC().Truncate(time.Minute)
			manageAttendanceSheet(s, m, sheetName)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Class time for '%s' updated to current time: %s", sheetName, classTimes[m.GuildID].Format("15:04 UTC")))
		} else {
			s.ChannelMessageSend(m.ChannelID, "Usage: !marksheet [Sheet Name] or !ms [Sheet Name] or include 'now' for current time.\nMore detail use `!help`")
		}

	//===========================================SET STUDENT LIST==============================================================
	case strings.HasPrefix(m.Content, "!setstudent"):
		args := strings.Fields(m.Content)
		if len(args) < 2 {
			s.ChannelMessageSend(m.ChannelID, "Usage: !setstudent [role name]")
			return
		}

		roleName := strings.Join(args[1:], " ")
		guildMembers, err := s.GuildMembers(m.GuildID, "", 1000)
		if err != nil {
			fmt.Println("Error fetching guild members:", err)
			s.ChannelMessageSend(m.ChannelID, "Failed to fetch members.")
			return
		}

		role, err := findRoleByName(s, m.GuildID, roleName)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Role not found.")
			return
		}

		var userIds []string
		for _, member := range guildMembers {
			for _, roleID := range member.Roles {
				if roleID == role.ID {
					userIds = append(userIds, member.User.ID)
					break
				}
			}
		}

		for _, userID := range userIds {
			member, _ := s.GuildMember(m.GuildID, userID)
			if member != nil {
				// Check if the student already exists in the database to prevent duplicates
				var exists int
				err := db.QueryRow("SELECT COUNT(*) FROM students WHERE guild_id = ? AND user_id = ?", m.GuildID, userID).Scan(&exists)
				if err != nil {
					fmt.Println("Error checking if user exists in database:", err)
					continue // Skip to the next user if there's an error
				}

				// If the student does not exist, insert them into the database
				if exists == 0 {
					_, err = db.Exec("INSERT INTO students (guild_id, user_id, username) VALUES (?, ?, ?)", m.GuildID, userID, member.User.Username)
					if err != nil {
						fmt.Println("Error inserting user into database:", err)
					}
				}
			}
		}

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Added %d students with role '%s' to the database.", len(userIds), roleName))

	//=========================================== Set Class Time
	case strings.HasPrefix(m.Content, "!setclasstime"):
		args := strings.Fields(m.Content)
		if len(args) < 2 {
			s.ChannelMessageSend(m.ChannelID, "Usage: !setclasstime {time}")
			return
		}
		classTime := args[1]
		setClassTime(s, m, classTime)

	case strings.HasPrefix(m.Content, "!classtime"): //==================SHOW class TIME===========
		showClassTime(s, m)

	//=========================================== Delete Class Time
	case strings.HasPrefix(m.Content, "!delclasstime"):
		args := strings.Fields(m.Content)
		if len(args) < 2 {
			s.ChannelMessageSend(m.ChannelID, "Usage: !delclasstime {time}")
			return
		}
		classTime := args[1]
		deleteClassTime(s, m, classTime)

	//=========================================== Reaction Role
	case strings.HasPrefix(m.Content, "!reacrole"):
		args := strings.SplitN(m.Content, " ", 3)
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "Usage: !reacrole [role name] [message]")
			return
		}

		roleID, err := findRoleByNameReac(s, m.GuildID, args[1])
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Role '%s' not found: %v", args[1], err))
			return
		}

		message := strings.TrimSpace(m.Content[len(args[0])+len(args[1])+2:]) // Get everything after the command and role name as the message

		msg, err := s.ChannelMessageSend(m.ChannelID, message)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Failed to send message: "+err.Error())
			return
		}

		err = s.MessageReactionAdd(msg.ChannelID, msg.ID, "✅")
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Failed to add reaction: "+err.Error())
			return
		}

		s.AddHandler(func(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
			handleReactionAdd(s, r, msg.ID, roleID)
		})
		s.AddHandler(func(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
			handleReactionRemove(s, r, msg.ID, roleID)
		})
	}
}

func voiceStateUpdate(s *discordgo.Session, vs *discordgo.VoiceStateUpdate, voiceStates map[string]map[string]time.Time, db *sql.DB) {
	guildID := vs.GuildID
	userID := vs.UserID
	joinTime := time.Now().UTC() // Ensure join time is recorded in UTC

	if vs.ChannelID != "" {
		if voiceStates[guildID] == nil {
			voiceStates[guildID] = make(map[string]time.Time)
		}
		voiceStates[guildID][userID] = joinTime

		voiceChannel, err := s.Channel(vs.ChannelID)
		if err != nil {
			fmt.Println("Error getting voice channel information:", err)
			return
		}
		voiceChannelName := voiceChannel.Name

		fmt.Println("INSERT INTO attendance: User joined")

		_, err = db.Exec("INSERT INTO attendance (guild_id, user_id, join_time, voice_channel) VALUES (?, ?, ?, ?)", guildID, userID, joinTime, voiceChannelName)
		if err != nil {
			fmt.Println("Error inserting join record:", err)
			return
		}
	} else {
		// Retrieve the stored join time from the map
		if storedJoinTime, ok := voiceStates[guildID][userID]; ok {
			leaveTime := time.Now().UTC() // Ensure leave time is recorded in UTC
			fmt.Println("INSERT INTO attendance: User left")

			duration := leaveTime.Sub(storedJoinTime).Minutes()
			fmt.Printf("User %s spent %.2f minutes in the channel\n", userID, duration)

			_, err := db.Exec("UPDATE attendance SET leave_time = ? WHERE guild_id = ? AND user_id = ? AND join_time = ?", leaveTime, guildID, userID, storedJoinTime)
			if err != nil {
				fmt.Println("Error inserting leave record:", err)
				return
			}
			delete(voiceStates[guildID], userID)
		}
	}
}

/*
// ====================================SET STUDENT, TIME, and DELETE TIME=========================================
// func trackRoleChange(s *discordgo.Session, guildID, roleID string) {
// 	s.AddHandler(func(s *discordgo.Session, u *discordgo.GuildMemberUpdate) {
// 		if u.GuildID != guildID {
// 			return // Ignore updates from other guilds.
// 		}

// 		// Check if the updated roles contain the specific role ID.
// 		for _, rID := range u.Roles {
// 			if rID == roleID {
// 				// Check if user already exists in the database to prevent duplicates.
// 				var exists int
// 				err := db.QueryRow("SELECT COUNT(*) FROM students WHERE guild_id = ? AND user_id = ?", guildID, u.User.ID).Scan(&exists)
// 				if err != nil {
// 					fmt.Println("Error checking user existence:", err)
// 					return
// 				}

// 				if exists == 0 {
// 					_, err := db.Exec("INSERT INTO students (guild_id, user_id, username) VALUES (?, ?, ?)", guildID, u.User.ID, u.User.Username)
// 					if err != nil {
// 						fmt.Println("Error inserting user into database:", err)
// 						return
// 					}
// 					fmt.Printf("Added new student: %s\n", u.User.Username)
// 				}
// 				break // No need to check further roles.
// 			}
// 		}
// 	})
// }
*/

func findRoleByName(s *discordgo.Session, guildID, roleName string) (*discordgo.Role, error) {
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		return nil, err
	}

	for _, role := range roles {
		if role.Name == roleName {
			return role, nil
		}
	}
	return nil, fmt.Errorf("role not found")
}

func HelpCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	pingMessage := "Check bot's response time.\n"
	marklistnowMessage := "Create list of users in a voice channel at a specific time.\nExample: `!marklistnow backend 08:45`.\n"
	marksheetMessage := "Manage attendance in a Google Sheet.\n" +
		"Create or Update Attendance in a Google Sheet. If the sheet is not available, a new one will be created. Can handle multi-word sheet names.\nExample: `!marksheet [Sheet Name]` or `!ms [Sheet Name]`. Add `now` before [Sheet Name] to use current Time.\n"
	setstudentMessage := "Add students with a specific role to the database.\n" +
		"Set the student in database by the role.\nExample: `!setstudent student` for users with the @student role.\n"
	setclasstimeMessage := "Set the class time.\n" +
		"Set the class Time format HH:MM.\nExample: `!setclasstime 08:45`.\n"
	classtimeMessage := "Show the set class time.\n"
	delclasstimeMessage := "Delete a set class time.\nExample: `!delclasstime 08:45`."
	reacroleMessage := "Add a reaction role.\n" +
		"Reaction role command. Create a message that will be sent and reacted to by users. All users who react will be assigned the specified role.\nExample: `!reacrole 4year For 4th year students! Leave a reaction below!`."

	embed := &discordgo.MessageEmbed{
		Title:       "Available Commands",
		Description: "Use the following commands to interact with the bot:",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "- `!ping`",
				Value:  pingMessage,
				Inline: false,
			},
			{
				Name:   "- `!marklistnow [voice channel name] [time]`",
				Value:  marklistnowMessage,
				Inline: false,
			},
			{
				Name:   "- `!marksheet [Sheet Name]` or `!ms [Sheet Name]`",
				Value:  marksheetMessage,
				Inline: false,
			},
			{
				Name:   "- `!setstudent [role name]`",
				Value:  setstudentMessage,
				Inline: false,
			},
			{
				Name:   "- `!setclasstime [time]`",
				Value:  setclasstimeMessage,
				Inline: false,
			},
			{
				Name:   "- `!classtime`",
				Value:  classtimeMessage,
				Inline: false,
			},
			{
				Name:   "- `!delclasstime [time]`",
				Value:  delclasstimeMessage,
				Inline: false,
			},
			{
				Name:   "- `!reacrole [role name] [message]`",
				Value:  reacroleMessage,
				Inline: false,
			},
		},
		Color: 0x00ff00, // Green color
	}

	// Command-specific help or general help
	if strings.HasPrefix(m.Content, "!help") {
		args := strings.Fields(m.Content)
		if len(args) == 1 {
			// Display a general help embed if only "!help" is entered without additional arguments.
			s.ChannelMessageSendEmbed(m.ChannelID, embed)
		} else {
			// Detailed command help
			specificCommandHelp(s, m, args[1:], embed)
		}
	} else {
		s.ChannelMessageSend(m.ChannelID, "More details needed. Use `!help` to see available commands.")
	}
}

// Handle specific command help
func specificCommandHelp(s *discordgo.Session, m *discordgo.MessageCreate, args []string, embed *discordgo.MessageEmbed) {
	if len(args) == 0 {
		s.ChannelMessageSend(m.ChannelID, "More details needed. Use `!help` to see available commands.")
		return
	}

	command := args[0]
	for _, field := range embed.Fields {
		if strings.Contains(field.Name, command) {
			s.ChannelMessageSend(m.ChannelID, field.Value)
			return
		}
	}

	s.ChannelMessageSend(m.ChannelID, "Invalid command. Use `!help` to see available commands.")
}

package main

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/api/sheets/v4"
)

/*Content:
-Mark list Now
	-handleMarkListNow

-Mark list Google Sheet
	-manageAttendanceSheet
	-createNewSheet
	-updateAttendanceSheet
	-fetchStudents
	-determineAttendance
*/

type Student struct {
	UserID   string
	Username string
}

// ===================================Mark list Now===========================================
func handleMarkListNow(s *discordgo.Session, m *discordgo.MessageCreate, voiceChannelName string, timeStr string) {
	currentDate := time.Now().UTC().Format("2006-01-02") // Ensures date is in UTC
	dateTimeStr := currentDate + " " + timeStr

	dateTimeParsed, err := time.Parse("2006-01-02 15:04", dateTimeStr) // Assuming timeStr is in UTC
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Invalid time format. Please use format HH:MM")
		return
	}

	startTime := dateTimeParsed.Add(-10 * time.Minute) // Starts checking 10 minutes before the given time
	endTime := dateTimeParsed.Add(90 * time.Minute)    // Ends checking 90 minutes after the given time

	query := `SELECT user_id FROM attendance WHERE join_time >= ? AND join_time <= ? AND voice_channel = ?`
	rows, err := db.Query(query, startTime, endTime, voiceChannelName)
	if err != nil {
		log.Println("Error querying database:", err)
		s.ChannelMessageSend(m.ChannelID, "An error occurred. Please try again later.")
		return
	}
	defer rows.Close()

	var userList []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			log.Println("Error scanning row:", err)
			continue
		}
		userList = append(userList, userID)
	}

	if len(userList) == 0 {
		s.ChannelMessageSend(m.ChannelID, "No users found in the specified time range.")
		return
	}

	var userNames []string
	for _, userID := range userList {
		user, err := s.User(userID)
		if err != nil {
			log.Println("Error getting user info:", err)
			continue
		}
		userNames = append(userNames, user.Username)
	}

	embed := &discordgo.MessageEmbed{
		Title: "Attendance List",
		Color: 0x00ff00, // Green color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Users Present",
				Value:  strings.Join(userNames, "\n"),
				Inline: false,
			},
		},
	}
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}

// ===================================Mark list Google Sheet===========================================
func manageAttendanceSheet(s *discordgo.Session, m *discordgo.MessageCreate, sheetName string) {
	if sheetName == "" {
		s.ChannelMessageSend(m.ChannelID, "Sheet name cannot be empty.")
		log.Println("Attempted to access a sheet with an empty name.")
		return
	}

	srv, err := initSheetsService()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to initialize Google Sheets service: %v", err))
		log.Printf("Failed to initialize Google Sheets service: %v\n", err)
		return
	}

	spreadsheet, err := srv.Spreadsheets.Get(spreadsheetID).Do()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to access the spreadsheet: %v", err))
		log.Printf("Failed to access the spreadsheet: %v\n", err)
		return
	}

	found := false
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			found = true
			break
		}
	}

	if !found {
		log.Printf("Sheet not found, creating a new one: %s\n", sheetName)
		createNewSheet(s, m, srv, sheetName)
	} else {
		log.Printf("Successfully accessed sheet: %s\n", sheetName)
		updateAttendanceSheet(s, m, srv, sheetName, m.GuildID)
	}
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully accessed sheet: %s\n", sheetName))
	s.ChannelMessageSend(m.ChannelID, "Sheet updated successfully with new attendance marks.")

	// endTime := classTimes[m.GuildID].Add(classDuration)
	// remainingTime := time.Until(endTime)

	if updateDuration[m.GuildID] {
		endTime := classTimes[m.GuildID].Add(classDuration)
		remainingTime := time.Until(endTime)

		if remainingTime > 0 {
			ticker := time.NewTicker(1 * time.Minute)
			endTimer := time.NewTimer(remainingTime)
			go func() {
				for {
					select {
					case <-ticker.C:
						if !updateDuration[m.GuildID] {
							ticker.Stop()
							updateAttendanceSheet(s, m, srv, sheetName, m.GuildID)
							log.Println("Update halted as per command.")
							return
						}
						updateAttendanceSheet(s, m, srv, sheetName, m.GuildID)
					case <-endTimer.C:
						ticker.Stop()
						log.Println("Class ended, stopping attendance updates.")
						return
					}
				}
			}()
		} else {
			log.Println("Class time has already passed, no attendance updates needed.")
		}
	}

	log.Printf("Attendance monitoring started for %s", sheetName)
	updateAttendanceSheet(s, m, srv, sheetName, m.GuildID)
}

func createNewSheet(s *discordgo.Session, m *discordgo.MessageCreate, srv *sheets.Service, sheetName string) {
	// Fetch student data
	students, err := fetchStudents(db, m.GuildID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Failed to fetch student data: "+err.Error())
		return
	}

	// Define the request to add a new sheet to the existing spreadsheet
	addSheetRequest := &sheets.AddSheetRequest{
		Properties: &sheets.SheetProperties{
			Title: sheetName,
		},
	}
	batchUpdateRequest := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: addSheetRequest,
			},
		},
	}

	// Execute the batch update to add the new sheet
	resp, err := srv.Spreadsheets.BatchUpdate(spreadsheetID, batchUpdateRequest).Do()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to create new sheet: %v", err))
		log.Printf("Failed to create new sheet: %v", err)
		return
	}

	// Extract the sheet ID of the newly created sheet
	newSheetID := resp.Replies[0].AddSheet.Properties.SheetId

	// Construct URL to the newly created sheet
	sheetURL := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit#gid=%d", spreadsheetID, newSheetID)

	// Append header to the new sheet
	headerValues := []interface{}{"Number", "Username", "Mark " + time.Now().Format("02/01/2006")}
	vr := &sheets.ValueRange{
		Values: [][]interface{}{headerValues},
	}
	_, err = srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1", vr).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Unable to append header to new sheet: %v", err))
		log.Printf("Unable to append header to new sheet: %v", err)
		return
	}

	// Append initial student data to the new sheet
	valueRange := sheetName + "!A2:B" + strconv.Itoa(len(students)+1)
	var data [][]interface{}
	for i, student := range students {
		row := []interface{}{i + 1, student.Username}
		data = append(data, row)
	}
	vr.Values = data
	_, err = srv.Spreadsheets.Values.Append(spreadsheetID, valueRange, vr).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to append student data to new sheet: %v", err))
		log.Printf("Failed to append student data to new sheet: %v", err)
		return
	}

	// Notify the user about the successful creation and provide the link to the new sheet
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("New sheet '%s' created and initialized successfully. You can access it here: %s", sheetName, sheetURL))
}

func updateAttendanceSheet(s *discordgo.Session, m *discordgo.MessageCreate, srv *sheets.Service, sheetName, guildID string) {
	students, err := fetchStudents(db, guildID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Failed to fetch student data: "+err.Error())
		return
	}

	currentDate := time.Now().UTC().Format("2006-01-02") // Ensuring the date is in UTC
	dateColumn := "Mark " + currentDate
	classTime, exists := classTimes[guildID]
	if !exists {
		s.ChannelMessageSend(m.ChannelID, "Class time not found.")
		return
	}

	// Calculate the start and potentially adjusted end times
	newClassTime := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), time.Now().UTC().Day(),
		classTime.Hour(), classTime.Minute(), classTime.Second(), classTime.Nanosecond(), time.UTC)
	startTime := newClassTime.Add(-10 * time.Minute) // Start time is 10 minutes before the recorded start time

	var endTime time.Time
	var classDurationSet time.Duration
	if classEndTimes, adjusted := classEndTimes[guildID]; adjusted {
		endTime = classEndTimes
		classDurationSet = endTime.Sub(startTime) // Use the time from the start to the adjusted end as the class duration
	} else {
		endTime = newClassTime.Add(classDuration) // Use the default duration if not adjusted
		classDurationSet = classDuration
	}

	startTimeStr := startTime.Format(time.RFC3339Nano)
	endTimeStr := endTime.Format(time.RFC3339Nano)
	log.Printf("Class Start time: %v, Adjusted End time: %v", startTimeStr, endTimeStr)

	// Continue processing to check header row existence and manage the sheet columns
	headerResp, err := srv.Spreadsheets.Values.Get(spreadsheetID, sheetName+"!1:1").Do()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to retrieve header row: %v", err))
		return
	}

	headerExists := false
	columnIndex := len(headerResp.Values[0])
	for index, headerValue := range headerResp.Values[0] {
		if headerValue.(string) == dateColumn {
			headerExists = true
			columnIndex = index
			break
		}
	}

	if !headerExists {
		newColumnRange := sheetName + fmt.Sprintf("!R1C%d", columnIndex+1)
		vr := &sheets.ValueRange{Values: [][]interface{}{{dateColumn}}}
		_, err = srv.Spreadsheets.Values.Update(spreadsheetID, newColumnRange, vr).ValueInputOption("USER_ENTERED").Do()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to add new date column: %v", err))
			return
		}
	}

	// Update the sheet with the attendance statuses
	values := make([][]interface{}, len(students))
	for i, student := range students {
		status := determineAttendance(db, student.UserID, guildID, startTime, endTime, classDurationSet)
		values[i] = []interface{}{status}
	}

	if len(values) > 0 {
		dataRange := fmt.Sprintf("%s!R2C%d:R%dC%d", sheetName, columnIndex+1, len(students)+1, columnIndex+1)
		vr := &sheets.ValueRange{Values: values}
		_, err = srv.Spreadsheets.Values.Update(spreadsheetID, dataRange, vr).ValueInputOption("USER_ENTERED").Do()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to update sheet: %v", err))
			return
		}
	}

	log.Printf("Sheet updated successfully with new attendance marks.")
}

/*
	func updateAttendanceSheet(s *discordgo.Session, m *discordgo.MessageCreate, srv *sheets.Service, sheetName, guildID string) {
		students, err := fetchStudents(db, guildID)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Failed to fetch student data: "+err.Error())
			return
		}

		currentDate := time.Now().UTC().Format("2006-01-02") // Ensuring the date is in UTC
		dateColumn := "Mark " + currentDate
		classTime, exists := classTimes[guildID]
		if !exists {
			s.ChannelMessageSend(m.ChannelID, "Class time not found.")
			return
		}

		newClassTime := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), time.Now().UTC().Day(),
			classTime.Hour(), classTime.Minute(), classTime.Second(), classTime.Nanosecond(), time.UTC)

		// Prepare start and end times, formatted in ISO 8601
		startTime := newClassTime.Add(-10 * time.Minute)
		endTime := newClassTime.Add(classDuration)
		startTimeStr := startTime.Format(time.RFC3339Nano)
		endTimeStr := endTime.Format(time.RFC3339Nano)

		log.Printf("Class Start time: %v, End time: %v", startTimeStr, endTimeStr)
		readHeaderRange := sheetName + "!1:1"
		headerResp, err := srv.Spreadsheets.Values.Get(spreadsheetID, readHeaderRange).Do()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to retrieve header row: %v", err))
			return
		}

		headerExists := false
		columnIndex := len(headerResp.Values[0]) // Assume the new column index is next in the row
		for index, headerValue := range headerResp.Values[0] {
			if headerValue.(string) == dateColumn {
				headerExists = true
				columnIndex = index // Update column index if the date column already exists
				break
			}
		}

		if !headerExists {
			newColumnRange := sheetName + fmt.Sprintf("!R1C%d", columnIndex+1) // Using R1C1 notation for clarity
			vr := &sheets.ValueRange{
				Values: [][]interface{}{{dateColumn}},
			}
			_, err = srv.Spreadsheets.Values.Update(spreadsheetID, newColumnRange, vr).ValueInputOption("USER_ENTERED").Do()
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to add new date column: %v", err))
				return
			}
		}

		// Prepare to update the new column with attendance status
		values := make([][]interface{}, len(students))
		for i, student := range students {
			status := determineAttendance(db, student.UserID, guildID, startTime, endTime, classDurationSet)
			values[i] = []interface{}{status} // Fill the new or existing column with status
		}

		// Update only the specific column for the current date
		if len(values) > 0 { // Ensure there are values to update
			dataRange := fmt.Sprintf("%s!R2C%d:R%dC%d", sheetName, columnIndex+1, len(students)+1, columnIndex+1) // Correct range format
			vr := &sheets.ValueRange{
				Values: values,
			}
			_, err = srv.Spreadsheets.Values.Update(spreadsheetID, dataRange, vr).ValueInputOption("USER_ENTERED").Do()
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to update sheet: %v", err))
				return
			}
		}

		log.Printf("Sheet updated successfully with new attendance marks.")
	}
*/
func fetchStudents(db *sql.DB, guildID string) ([]Student, error) {
	query := `SELECT user_id, username FROM students WHERE guild_id = ?`
	rows, err := db.Query(query, guildID)
	if err != nil {
		return nil, fmt.Errorf("error fetching students from database: %v", err)
	}
	defer rows.Close()

	var students []Student
	for rows.Next() {
		var student Student
		if err := rows.Scan(&student.UserID, &student.Username); err != nil {
			return nil, fmt.Errorf("error reading student data: %v", err)
		}
		students = append(students, student)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating through students: %v", err)
	}

	return students, nil
}

func determineAttendance(db *sql.DB, userID, guildID string, classStartTimeUTC, classEndTimeUTC time.Time, classDurationSet time.Duration) string {
	var rows *sql.Rows
	var err error
	query := `
        SELECT join_time, leave_time
        FROM attendance
        WHERE user_id = ? AND guild_id = ? AND join_time < ? AND (leave_time IS NULL OR leave_time > ?)
        ORDER BY join_time ASC
    `
	rows, err = db.Query(query, userID, guildID, classEndTimeUTC, classStartTimeUTC)
	if err != nil {
		log.Printf("Error querying attendance data: %v", err)
		return "" // Error state
	}
	defer rows.Close()

	totalAttendedDuration := time.Duration(0)
	var earliestJoinTime time.Time

	const layout = time.RFC3339Nano // Time layout for parsing
	isFirst := true

	for rows.Next() {
		var joinTimeStr, leaveTimeStr sql.NullString
		err = rows.Scan(&joinTimeStr, &leaveTimeStr)
		if err != nil {
			log.Printf("Error scanning attendance data: %v", err)
			return "" // Error state
		}

		var joinTime, leaveTime time.Time
		if joinTimeStr.Valid {
			joinTime, err = time.Parse(layout, joinTimeStr.String)
			if err != nil {
				log.Printf("Error parsing join time: %v", err)
				return "" // Invalid join time format
			}
			if isFirst || joinTime.Before(earliestJoinTime) {
				earliestJoinTime = joinTime
				isFirst = false
			}
		}

		if leaveTimeStr.Valid {
			leaveTime, err = time.Parse(layout, leaveTimeStr.String)
			if err != nil {
				log.Printf("Error parsing leave time: %v", err)
				return "" // Invalid leave time format
			}
		} else {
			leaveTimeNull := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), time.Now().UTC().Day(),
				time.Now().UTC().Hour(), time.Now().UTC().Minute(), time.Now().UTC().Second(), time.Now().UTC().Nanosecond(), time.UTC)
			leaveTimeNullStr := leaveTimeNull.Format(layout)
			leaveTime, err = time.Parse(layout, leaveTimeNullStr)
			if err != nil {
				log.Printf("Error parsing leave time: %v", err)
				return "" // Invalid leave time format
			}
		}

		duration := leaveTime.Sub(joinTime)
		if duration > 0 {
			totalAttendedDuration += duration
		}
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error processing rows: %v", err)
		return "" // Error state
	}

	if totalAttendedDuration <= 0 {
		return "A 0%" // Absent
	}

	// Calculations for attendance duration and percentage
	attendedMinutes := totalAttendedDuration.Minutes()
	totalMinutes := classDurationSet.Minutes()
	attendancePercentage := int((attendedMinutes / totalMinutes) * 100)
	if attendancePercentage > 100 {
		attendancePercentage = 100 // Cap at 100% if there's overlapping time
	}

	var status string
	adjClassStart := classStartTimeUTC.Add(20 * time.Minute) // 10 minutes grace period *note classtartime is already -10m
	if earliestJoinTime.After(adjClassStart) {
		lateDuration := earliestJoinTime.Sub(adjClassStart.Add(-10 * time.Minute)) // Return 10 minutes back to class start time
		minutes := int(lateDuration.Minutes())
		seconds := int(lateDuration.Seconds()) % 60
		status = fmt.Sprintf("L %dm%ds %d%%", minutes, seconds, attendancePercentage)
	} else {
		status = fmt.Sprintf("X %d%%", attendancePercentage)
	}

	return status
}

// // If the join time is after the class has ended, return absent
// if joinTime.After(classEndTimeUTC) {
// 	return "A 0%"
// }

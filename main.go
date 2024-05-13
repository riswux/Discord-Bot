package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	var err error
	db, err = sql.Open("sqlite3", "./classroom.db")
	if err != nil {
		fmt.Println("Error opening database:", err)
		return
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS attendance (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			guild_id TEXT,
			user_id TEXT,
			join_time DATETIME,
			leave_time DATETIME,
			voice_channel TEXT
		)
	`)
	if err != nil {
		fmt.Println("Error creating table:", err)
		return
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS students (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            guild_id TEXT NOT NULL,
            user_id TEXT NOT NULL,
			username VARCHAR(32),
            UNIQUE(guild_id, user_id) ON CONFLICT REPLACE
        )
    `)
	if err != nil {
		fmt.Println("Error creating students table:", err)
		return
	}

	classTimes = make(map[string]time.Time)

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating Discord session:", err)
		return
	}

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		messageCreate(s, m, db)
	})
	dg.AddHandler(func(s *discordgo.Session, vs *discordgo.VoiceStateUpdate) {
		voiceStateUpdate(s, vs, voiceStates, db)
	})

	// dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
	// 	handleSetStudent(s, m)
	// })

	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session:", err)
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic: %v\n", r)
		}
	}()

	fmt.Println("Bot is now running. Press Ctrl+C to exit.")

	<-make(chan struct{})

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	dg.Close()
}

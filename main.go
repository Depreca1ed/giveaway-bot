// main.go
package main

import (
	"embed"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Cylis-Dragneel/giveaway-bot/internal/bot"
	"github.com/Cylis-Dragneel/giveaway-bot/internal/db"
	"github.com/Cylis-Dragneel/giveaway-bot/internal/models"
	"github.com/bwmarrin/discordgo"
)

//go:embed schema.sql
var schema embed.FS

func main() {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_BOT_TOKEN environment variable not set")
	}

	// Verify schema.sql is embedded
	schemaContent, err := schema.ReadFile("schema.sql")
	if err != nil {
		log.Fatal("Error reading embedded schema.sql: ", err)
	}
	log.Println("Successfully embedded schema.sql, size:", len(schemaContent), "bytes")

	dbPath := "giveaway.db"
	if _, err := os.ReadFile(dbPath); err != nil {
		log.Printf("File doesn't exist, creating...")
		if err = os.WriteFile(dbPath, nil, 0644); err != nil {
			log.Fatalf("Failed to create DB file: %v", err)
		}
	}
	err = db.InitDB(dbPath, schema)
	if err != nil {
		log.Fatal(err)
	}
	defer db.CloseDB()

	// Load active giveaways
	db.LoadGiveaways()

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal("Error creating Discord session: ", err)
	}

	bot.SetSession(dg) // Set global session for endGiveaway access

	// Load active giveaways and set timers
	giveaways, err := db.LoadGiveaways()
	if err != nil {
		log.Fatal("Error loading giveaways: ", err)
	}
	for _, ga := range giveaways {
		if time.Now().After(ga.EndTime) {
			db.DeleteGiveaway(ga.ID)
			continue
		}
		duration := time.Until(ga.EndTime)
		ga.Timer = time.AfterFunc(duration, func() {
			models.EndGiveaway(dg, ga)
			db.DeleteGiveaway(ga.ID)
		})
		models.Giveaways[ga.ID] = ga
	}

	dg.AddHandler(bot.Ready)
	dg.AddHandler(bot.InteractionCreate)

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildMembers

	err = dg.Open()
	if err != nil {
		log.Fatal("Error opening connection: ", err)
	}

	// Register slash commands globally
	commands := bot.GetCommands()
	for _, cmd := range commands {
		_, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("Cannot create '%v' command: %v", cmd.Name, err)
		}
	}

	log.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()

}

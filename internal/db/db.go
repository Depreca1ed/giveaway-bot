// internal/db/db.go
package db

import (
	"database/sql"
	"embed"
	"log"
	"time"

	"github.com/Cylis-Dragneel/giveaway-bot/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB(path string, schema embed.FS) error {
	var err error
	DB, err = sql.Open("sqlite3", path)
	if err != nil {
		return err
	}

	// Read embedded schema.sql
	schemaBytes, err := schema.ReadFile("schema.sql")
	if err != nil {
		return err
	}

	// Execute schema
	_, err = DB.Exec(string(schemaBytes))
	if err != nil {
		return err
	}

	log.Println("Database initialized successfully")
	return nil
}

func CloseDB() {
	if DB != nil {
		if err := DB.Close(); err != nil {
			log.Println("Error closing database:", err)
		}
	}
}

func SaveGiveaway(ga *models.Giveaway) {
	_, err := DB.Exec(`INSERT INTO giveaways (id, title, end_time, role_id, channel_id, message_id, winners) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ga.ID, ga.Title, ga.EndTime.Unix(), ga.RoleID, ga.ChannelID, ga.MessageID, ga.Winners)
	if err != nil {
		log.Println("Error saving giveaway:", err)
	}
}

func SaveParticipants(giveawayID string, participants []string) {
	tx, err := DB.Begin()
	if err != nil {
		log.Println("Error starting transaction:", err)
		return
	}
	_, err = tx.Exec(`DELETE FROM participants WHERE giveaway_id = ?`, giveawayID)
	if err != nil {
		tx.Rollback()
		log.Println("Error deleting participants:", err)
		return
	}
	for _, p := range participants {
		_, err = tx.Exec(`INSERT INTO participants (giveaway_id, user_id) VALUES (?, ?)`, giveawayID, p)
		if err != nil {
			tx.Rollback()
			log.Println("Error saving participant:", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		log.Println("Error committing transaction:", err)
	}
}

func LoadGiveaways() ([]*models.Giveaway, error) {
	rows, err := DB.Query(`SELECT id, title, end_time, role_id, channel_id, message_id, winners FROM giveaways`)
	if err != nil {
		log.Println("Error querying giveaways:", err)
		return nil, err
	}
	defer rows.Close()

	var giveaways []*models.Giveaway
	for rows.Next() {
		var id, title, roleID, channelID, messageID string
		var endUnix int64
		var winners int
		err = rows.Scan(&id, &title, &endUnix, &roleID, &channelID, &messageID, &winners)
		if err != nil {
			log.Println("Error scanning giveaway:", err)
			continue
		}
		ga := &models.Giveaway{
			ID:           id,
			Title:        title,
			EndTime:      time.Unix(endUnix, 0),
			RoleID:       roleID,
			ChannelID:    channelID,
			MessageID:    messageID,
			Winners:      winners,
			Participants: LoadParticipants(id),
		}
		giveaways = append(giveaways, ga)
	}
	return giveaways, nil
}

func LoadParticipants(giveawayID string) []string {
	rows, err := DB.Query(`SELECT user_id FROM participants WHERE giveaway_id = ?`, giveawayID)
	if err != nil {
		log.Println("Error querying participants:", err)
		return nil
	}
	defer rows.Close()

	var participants []string
	for rows.Next() {
		var userID string
		err = rows.Scan(&userID)
		if err != nil {
			log.Println("Error scanning participant:", err)
			continue
		}
		participants = append(participants, userID)
	}
	return participants
}

func DeleteGiveaway(id string) {
	_, err := DB.Exec(`DELETE FROM giveaways WHERE id = ?`, id)
	if err != nil {
		log.Println("Error deleting giveaway:", err)
	}
	_, err = DB.Exec(`DELETE FROM participants WHERE giveaway_id = ?`, id)
	if err != nil {
		log.Println("Error deleting participants:", err)
	}
}

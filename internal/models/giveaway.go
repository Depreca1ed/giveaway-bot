// internal/models/giveaway.go
package models

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Giveaway struct {
	ID           string
	Title        string
	EndTime      time.Time
	RoleID       string
	Participants []string
	ChannelID    string
	MessageID    string
	Timer        *time.Timer
	Winners      int
}

var (
	Giveaways      = make(map[string]*Giveaway)
	GiveawaysMutex sync.RWMutex
)

func ParseEndTime(endStr string) (time.Time, error) {
	loc, err := time.LoadLocation("Etc/UTC")
	if err != nil {
		return time.Time{}, err
	}
	dur, err := time.ParseDuration(endStr)
	if err == nil {
		return time.Now().In(loc).Add(dur), nil
	}
	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		t, err := time.ParseInLocation(f, endStr, loc)
		if err == nil {
			if f == "2006-01-02" {
				t = t.Add(23*time.Hour + 59*time.Minute)
			}
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid format")
}

func CreateGiveawayEmbed(title string, endTime time.Time, roleID string, participants int, winners int) *discordgo.MessageEmbed {
	loc, _ := time.LoadLocation("Etc/UTC")
	roleMention := "None"
	if roleID != "" {
		roleMention = "<@&" + roleID + ">"
	}
	timestamp := fmt.Sprintf("<t:%d:R>", endTime.Unix())
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: fmt.Sprintf("Participants: %d\nWinners: %d\nTime Left: %s\nRole Required: %s", participants, winners, timestamp, roleMention),
		Color:       0x00ff00,
		Timestamp:   endTime.In(loc).Format(time.RFC3339),
	}
}

func UpdateGiveawayEmbed(s *discordgo.Session, ga *Giveaway) {
	embed := CreateGiveawayEmbed(ga.Title, ga.EndTime, ga.RoleID, len(ga.Participants), ga.Winners)
	_, err := s.ChannelMessageEditEmbed(ga.ChannelID, ga.MessageID, embed)
	if err != nil {
		log.Println("Error updating embed:", err)
	}
}

func EndGiveaway(s *discordgo.Session, ga *Giveaway) {
	// Check if message exists
	_, err := s.ChannelMessage(ga.ChannelID, ga.MessageID)
	if err != nil {
		log.Printf("Error fetching message %s in channel %s: %v", ga.MessageID, ga.ChannelID, err)
		_, sendErr := s.ChannelMessageSend(ga.ChannelID, "Giveaway ended, but the original message could not be found.")
		if sendErr != nil {
			log.Println("Error sending fallback message:", sendErr)
		}
		GiveawaysMutex.Lock()
		delete(Giveaways, ga.ID)
		GiveawaysMutex.Unlock()
		return
	}

	if len(ga.Participants) == 0 {
		_, err := s.ChannelMessageSend(ga.ChannelID, "Giveaway ended with no participants.")
		if err != nil {
			log.Println("Error sending message:", err)
		}
	} else {
		winnersCount := ga.Winners
		if winnersCount < 1 {
			winnersCount = 1
		}
		if winnersCount > len(ga.Participants) {
			winnersCount = len(ga.Participants)
		}
		rand.Shuffle(len(ga.Participants), func(i, j int) {
			ga.Participants[i], ga.Participants[j] = ga.Participants[j], ga.Participants[i]
		})
		winners := ga.Participants[:winnersCount]
		// winnerMentions := make([]string, len(winners))
		var winnerMentions []string
		for _, uid := range winners {
			winnerMentions = append(winnerMentions, fmt.Sprintf("<@%s>", uid))
		}
		pingText := strings.Join(winnerMentions, " ")
		var mentionList string
		if len(winnerMentions) == 1 {
			mentionList = winnerMentions[0]
		} else {
			mentionList = strings.Join(winnerMentions, ", ")
		}
		// winnerIdx := rand.Intn(len(ga.Participants))
		// winnerID := ga.Participants[winnerIdx]
		embed := &discordgo.MessageEmbed{
			Title:       "Giveaway Ended!",
			Description: fmt.Sprintf("**Winner(s):** %s", mentionList),
			Color:       0xff0000,
		}
		components := []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "Reroll",
						Style:    discordgo.PrimaryButton,
						CustomID: "reroll",
					},
				},
			},
		}
		_, err := s.ChannelMessageSendComplex(ga.ChannelID, &discordgo.MessageSend{
			Content:    pingText,
			Embed:      embed,
			Components: components,
			Reference: &discordgo.MessageReference{
				MessageID: ga.MessageID,
				ChannelID: ga.ChannelID,
			},
		})
		if err != nil {
			log.Println("Error sending winner message:", err)
		}
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji:    &discordgo.ComponentEmoji{Name: "🎉"},
					Style:    discordgo.PrimaryButton,
					CustomID: "enter_giveaway",
					Disabled: true,
				},
				discordgo.Button{
					Label:    "Participants",
					Style:    discordgo.SecondaryButton,
					CustomID: "list_participants_1",
				},
			},
		},
	}
	messageEdit := &discordgo.MessageEdit{
		ID:         ga.MessageID,
		Channel:    ga.ChannelID,
		Components: &components,
	}
	_, err = s.ChannelMessageEditComplex(messageEdit)
	if err != nil {
		log.Printf("Error updating message components for message %s in channel %s: %v", ga.MessageID, ga.ChannelID, err)
		_, sendErr := s.ChannelMessageSend(ga.ChannelID, "Giveaway ended, but could not update the original message.")
		if sendErr != nil {
			log.Println("Error sending fallback message:", sendErr)
		}
	}

	GiveawaysMutex.Lock()
	delete(Giveaways, ga.ID)
	GiveawaysMutex.Unlock()
}

// internal/bot/bot.go
package bot

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

var session *discordgo.Session

func SetSession(s *discordgo.Session) {
	session = s
}

func GetSession() *discordgo.Session {
	return session
}

func GetCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "create-giveaway",
			Description: "Create a new giveaway",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "title",
					Description: "Title of the giveaway",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "end",
					Description: "End time: duration (e.g., 1h30m) or date/time (YYYY-MM-DD [HH:MM])",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role required to join (optional)",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "winners",
					Description: "Number of winners (optional)",
					Required:    false,
				},
			},
		},
		{
			Name:        "list-giveaways",
			Description: "List all running giveaways in the server",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "See giveaways entered by this user (optional)",
					Required:    false,
				},
			},
		},
		{
			Name:        "my-giveaways",
			Description: "List giveaways you have entered",
		},
		{
			Name:        "leave-giveaway",
			Description: "Leave a giveaway using its ID",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "id",
					Description: "Giveaway ID (shown in /list-giveaways)",
					Required:    true,
				},
			},
		},
		{
			Name:        "leave-all-giveaways",
			Description: "Leave all giveaways you have entered",
		},
		{
			Name:        "remove",
			Description: "Remove a user from a giveaway (Admin/Mod only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "User to remove",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "id",
					Description: "Giveaway ID (from /list-giveaways)",
					Required:    true,
				},
			},
		},
	}
}

func Ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready!")
}

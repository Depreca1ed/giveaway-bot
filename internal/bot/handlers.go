// internal/bot/handlers.go
package bot

import (
	"fmt"
	"log"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Cylis-Dragneel/giveaway-bot/internal/db"
	"github.com/Cylis-Dragneel/giveaway-bot/internal/models"
	"github.com/bwmarrin/discordgo"
)

func InteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		handleSlashCommand(s, i)
	case discordgo.InteractionMessageComponent:
		handleButtonClick(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	}
}

func handleSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	switch data.Name {
	case "create-giveaway":
		createGiveaway(s, i)
	case "list-giveaways":
		userID := ""
		if len(data.Options) > 0 && data.Options[0].Name == "user" {
			userID = data.Options[0].UserValue(nil).ID
		}
		listGiveaways(s, i, userID)
	case "my-giveaways":
		userID := i.Member.User.ID
		if i.Member == nil {
			// DM context
			userID = i.User.ID
		}
		listGiveaways(s, i, userID)
	case "leave-giveaway":
		giveawayID := data.Options[0].StringValue()
		userID := i.Member.User.ID

		models.GiveawaysMutex.Lock()
		ga, exists := models.Giveaways[giveawayID]
		if !exists {
			models.GiveawaysMutex.Unlock()
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Giveaway not found or already ended.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		// Remove user from participants
		removed := false
		for idx, p := range ga.Participants {
			if p == userID {
				ga.Participants = append(ga.Participants[:idx], ga.Participants[idx+1:]...)
				removed = true
				break
			}
		}

		if !removed {
			models.GiveawaysMutex.Unlock()
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "You are not in this giveaway.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		// Update embed + DB
		models.UpdateGiveawayEmbed(s, ga)
		db.SaveParticipants(ga.ID, ga.Participants)
		models.GiveawaysMutex.Unlock()

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("You have left the giveaway **%s**", escapeMarkdown(ga.Title)),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	case "leave-all-giveaways":
		userID := i.Member.User.ID
		models.GiveawaysMutex.Lock()
		defer models.GiveawaysMutex.Unlock()

		leftCount := 0
		var leftTitles []string
		var fields []*discordgo.MessageEmbedField
		for _, ga := range models.Giveaways {
			if time.Now().After(ga.EndTime) {
				continue
			}

			removed := false
			for idx, p := range ga.Participants {
				if p == userID {
					ga.Participants = append(ga.Participants[:idx], ga.Participants[idx+1:]...)
					removed = true
					break
				}
			}
			if removed {
				leftCount++
				leftTitles = append(leftTitles, escapeMarkdown(ga.Title))

				models.UpdateGiveawayEmbed(s, ga)
				db.SaveParticipants(ga.ID, ga.Participants)
			}
			guildID := i.GuildID
			if guildID == "" {
				guildID = "@me" // DMs (shouldn't happen)
			}
			link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guildID, ga.ChannelID, ga.MessageID)
			titleLink := fmt.Sprintf("%s %s", escapeMarkdown(ga.Title), link)

			loc, _ := time.LoadLocation("Etc/UTC")
			timeLeft := fmt.Sprintf("<t:%d:R>", ga.EndTime.In(loc).Unix())

			field := &discordgo.MessageEmbedField{
				Name:   fmt.Sprintf("%s (ID: `%s`)", titleLink, ga.MessageID),
				Value:  fmt.Sprintf("Time: %s", timeLeft),
				Inline: false,
			}
			fields = append(fields, field)
		}
		if leftCount == 0 {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "You are not in any active giveaways.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
		var msg string
		if leftCount == 1 {
			msg = "You have left **1** giveaway:"
		} else {
			msg = fmt.Sprintf("You have left **%d** giveaways:", leftCount)
		}
		embed := &discordgo.MessageEmbed{
			Title:  msg,
			Color:  0x00ff00,
			Fields: fields,
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
				Flags:  discordgo.MessageFlagsEphemeral,
			},
		})
	case "remove":
		if !hasPermission(s, i) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "You do not have permission to use this command.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		targetUser := data.Options[0].UserValue(nil)
		giveawayID := data.Options[1].StringValue()
		actorID := i.Member.User.ID

		models.GiveawaysMutex.Lock()
		ga, exists := models.Giveaways[giveawayID]
		if !exists {
			models.GiveawaysMutex.Unlock()
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Giveaway not found or already ended.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		removed := false
		for idx, p := range ga.Participants {
			if p == targetUser.ID {
				ga.Participants = append(ga.Participants[:idx], ga.Participants[idx+1:]...)
				removed = true
				break
			}
		}

		if !removed {
			models.GiveawaysMutex.Unlock()
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("<@%s> is not in this giveaway.", targetUser.ID),
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		// Update embed + DB
		models.UpdateGiveawayEmbed(s, ga)
		db.SaveParticipants(ga.ID, ga.Participants)
		models.GiveawaysMutex.Unlock()

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf(
					"<@%s> has been removed from giveaway **%s** by <@%s>.",
					targetUser.ID, escapeMarkdown(ga.Title), actorID,
				),
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})

	}
}

func hasPermission(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	member := i.Member
	if member == nil {
		return false
	}

	if member.Permissions&discordgo.PermissionAdministrator != 0 {
		return true
	}

	allowedRoles := []string{"1348095555594879026", "1172677966912815174", "1436059069424336958"}
	for _, role := range member.Roles {
		if slices.Contains(allowedRoles, role) {
			return true
		}
	}
	return false
}

func handleButtonClick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	userID := i.Member.User.ID
	messageID := i.Message.ID

	if customID == "enter_giveaway" {
		handleEnterGiveaway(s, i, userID, messageID)
	} else if strings.HasPrefix(customID, "list_participants_") {
		pageStr := strings.TrimPrefix(customID, "list_participants_")
		page, _ := strconv.Atoi(pageStr)
		showParticipants(s, i, page, messageID)
	} else if strings.HasPrefix(customID, "next_page_") || strings.HasPrefix(customID, "prev_page_") {
		parts := strings.Split(customID, "_")
		page, _ := strconv.Atoi(parts[2])
		if parts[0] == "prev" {
			page--
		} else {
			page++
		}
		showParticipants(s, i, page, messageID)
	} else if customID == "reroll" {
		handleReroll(s, i)
	}
}

func getOption(m map[string]*discordgo.ApplicationCommandInteractionDataOption, name string) *discordgo.ApplicationCommandInteractionDataOption {
	if opt, ok := m[name]; ok {
		return opt
	}
	return nil
}

func createGiveaway(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}
	var roleID string
	winners := 1 // default
	title := getOption(optionMap, "title").StringValue()
	endStr := getOption(optionMap, "end").StringValue()
	if roleOpt := getOption(optionMap, "role"); roleOpt != nil {
		roleID = roleOpt.RoleValue(nil, "").ID
	}

	if winnerOpt := getOption(optionMap, "winners"); winnerOpt != nil {
		w := int(winnerOpt.IntValue())
		if w > 0 {
			winners = w
		}
	}

	endTime, err := models.ParseEndTime(endStr)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Invalid end time format: " + err.Error(),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Creating giveaway...",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	embed := models.CreateGiveawayEmbed(title, endTime, roleID, 0, winners)
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji:    &discordgo.ComponentEmoji{Name: "🎉"},
					Style:    discordgo.PrimaryButton,
					CustomID: "enter_giveaway",
				},
				discordgo.Button{
					Label:    "Participants",
					Style:    discordgo.SecondaryButton,
					CustomID: "list_participants_0",
				},
			},
		},
	}

	msg, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: components,
	})
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: ptr("Error sending message: " + err.Error()),
		})
		return
	}

	ga := &models.Giveaway{
		ID:           msg.ID,
		Title:        title,
		EndTime:      endTime,
		RoleID:       roleID,
		Participants: []string{},
		ChannelID:    i.ChannelID,
		MessageID:    msg.ID,
		Winners:      winners,
	}

	duration := time.Until(endTime)
	ga.Timer = time.AfterFunc(duration, func() {
		models.EndGiveaway(GetSession(), ga)
	})

	models.Giveaways[msg.ID] = ga
	db.SaveGiveaway(ga)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: ptr("Giveaway created!"),
	})
}

func handleEnterGiveaway(s *discordgo.Session, i *discordgo.InteractionCreate, userID, messageID string) {
	ga, ok := models.Giveaways[messageID]
	if !ok || time.Now().After(ga.EndTime) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Giveaway not found or has ended.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if ga.RoleID != "" {
		hasRole := false
		for _, role := range i.Member.Roles {
			if role == ga.RoleID {
				hasRole = true
				break
			}
		}
		if !hasRole {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "You don't have the required role to join.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
	}

	isParticipant := false
	for _, p := range ga.Participants {
		if p == userID {
			isParticipant = true
			break
		}
	}

	if isParticipant {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "leave_giveaway_modal_" + messageID,
				Title:    "Confirm Leave Giveaway",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "leave_confirmation",
								Label:       "Type LEAVE to confirm",
								Style:       discordgo.TextInputShort,
								Placeholder: "LEAVE",
								Required:    true,
							},
						},
					},
				},
			},
		})
	} else {
		ga.Participants = append(ga.Participants, userID)
		models.UpdateGiveawayEmbed(s, ga)
		db.SaveParticipants(ga.ID, ga.Participants)

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You have entered the giveaway!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	log.Printf("Modal CustomID: %s, Components: %+v", data.CustomID, data.Components)
	if strings.HasPrefix(data.CustomID, "leave_giveaway_modal_") {
		messageID := strings.TrimPrefix(data.CustomID, "leave_giveaway_modal_")
		userID := i.Member.User.ID

		// Extract text input from modal
		var input string
		for _, component := range data.Components {
			if actionRow, ok := component.(*discordgo.ActionsRow); ok {
				for _, comp := range actionRow.Components {
					if textInput, ok := comp.(*discordgo.TextInput); ok && textInput.CustomID == "leave_confirmation" {
						input = textInput.Value
						break
					}
				}
			}
		}

		if input != "LEAVE" {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Invalid input. You must type 'LEAVE' exactly.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Println("Error responding to modal submission:", err)
			}
			return
		}

		ga, ok := models.Giveaways[messageID]
		if !ok {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Giveaway not found.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Println("Error responding to modal submission:", err)
			}
			return
		}

		for idx, p := range ga.Participants {
			if p == userID {
				ga.Participants = append(ga.Participants[:idx], ga.Participants[idx+1:]...)
				break
			}
		}
		models.UpdateGiveawayEmbed(s, ga)
		db.SaveParticipants(ga.ID, ga.Participants)

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You have left the giveaway.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			log.Println("Error responding to modal submission:", err)
		}
	}
}

func handleReroll(s *discordgo.Session, i *discordgo.InteractionCreate) {
	originalMsgID := i.Message.ID
	ga, ok := models.Giveaways[originalMsgID]
	var participants []string
	if ok {
		participants = ga.Participants
	} else {
		participants = db.LoadParticipants(originalMsgID)
	}

	if len(participants) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No participants to reroll.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	winnerIdx := rand.Intn(len(participants))
	winnerID := participants[winnerIdx]

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("<@%s>", winnerID),
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "Giveaway Rerolled!",
					Description: fmt.Sprintf("New Winner: <@%s>", winnerID),
					Color:       0xff0000,
				},
			},
		},
	})
}

func showParticipants(s *discordgo.Session, i *discordgo.InteractionCreate, page int, messageID string) {
	ga, ok := models.Giveaways[messageID]
	if !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Giveaway not found.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	const perPage = 10
	total := len(ga.Participants)
	maxPage := (total + perPage - 1) / perPage
	if page < 0 {
		page = 0
	}
	if page >= maxPage {
		page = maxPage - 1
	}

	start := page * perPage
	end := start + perPage
	if end > total {
		end = total
	}

	var entries []string
	for _, uid := range ga.Participants[start:end] {
		user, err := s.User(uid)
		name := uid
		if err == nil {
			name = user.Username
		}
		entries = append(entries, fmt.Sprintf("<@%s> (%s)", uid, name))
	}

	description := strings.Join(entries, "\n")
	if len(entries) == 0 {
		description = "*No participants on this page.*"
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Participants (%d total)", total),
		Description: description,
		Color:       0x00ff00,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Page %d of %d", page+1, maxPage),
		},
	}

	components := []discordgo.MessageComponent{}
	if maxPage > 1 {
		row := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{},
		}
		if page > 0 {
			row.Components = append(row.Components, discordgo.Button{
				Label:    "Previous",
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("prev_page_%d_%s", page, messageID),
			})
		}
		if page < maxPage-1 {
			row.Components = append(row.Components, discordgo.Button{
				Label:    "Next",
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("next_page_%d_%s", page, messageID),
			})
		}
		components = append(components, row)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}

// Prevent markdown injection in titles
func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "~", "\\~")
	s = strings.ReplaceAll(s, "`", "\\`")
	return s
}

func listGiveaways(s *discordgo.Session, i *discordgo.InteractionCreate, userID string) {
	models.GiveawaysMutex.RLock()
	defer models.GiveawaysMutex.RUnlock()

	var fields []*discordgo.MessageEmbedField
	now := time.Now()

	for _, ga := range models.Giveaways {
		if now.After(ga.EndTime) {
			continue
		}

		if userID != "" && !slices.Contains(ga.Participants, userID) {
			continue
		}

		// Link (https://discord.com/channels/guildID/channelID/messageID)
		guildID := i.GuildID
		if guildID == "" {
			guildID = "@me" // DMs (shouldn't happen)
		}
		link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guildID, ga.ChannelID, ga.MessageID)
		titleLink := fmt.Sprintf("%s %s", escapeMarkdown(ga.Title), link)

		loc, _ := time.LoadLocation("Etc/UTC")
		timeLeft := fmt.Sprintf("<t:%d:R>", ga.EndTime.In(loc).Unix())

		field := &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s (ID: `%s`)", titleLink, ga.MessageID),
			Value:  fmt.Sprintf("Time: %s", timeLeft),
			Inline: false,
		}
		fields = append(fields, field)
	}

	if len(fields) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No active giveaways found.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:  "Active Giveaways",
		Color:  0x00ff00,
		Fields: fields,
	}

	if userID != "" {
		targetUser, _ := s.User(userID)
		username := "user"
		if targetUser != nil {
			username = targetUser.Username
		}
		embed.Description = fmt.Sprintf("Giveaways entered by **%s**:", username)
	} else {
		embed.Description = "All running giveaways:"
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func ptr(s string) *string {
	return &s
}

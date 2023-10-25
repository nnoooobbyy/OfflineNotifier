package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
)

// ----- VARS
var (
	botVersion        = "V7.3 | GOLANG"
	actionQueue       []Request
	startTime         = time.Now().Unix()
	startedCoroutines = false
	inviteLink        = ""
	ownerID           = ""
	defaultColor      = 0x7289da
	successColor      = 0x76e06e
	failColor         = 0xe06c6c
	onlineColor       = 0x43b581
	offlineColor      = 0x727c8a
)

// ----- STRUCTS
type Request struct {
	action string
	data   [4]string
}

type Bot struct {
	ID          string   `json:"id"`
	Guilds      []string `json:"guilds"`
	Subscribers []string `json:"subscribers"`
	Status      string   `json:"status"`
	Timestamp   int64    `json:"timestamp"`
}

type Guild struct {
	ID   string   `json:"id"`
	CID  string   `json:"cid"`
	Bots []string `json:"bots"`
}

type Subscriber struct {
	ID   string   `json:"id"`
	Bots []string `json:"bots"`
}

// -----  MAIN
func main() {
	// LOGGING
	f, err := os.OpenFile("./logs/"+time.Now().Format("2006-01-02 15:04:05 MST")+".log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.SetOutput(f)

	// LOADING ENV
	err = godotenv.Load("./OfflineNotifier.env")
	if err != nil {
		log.Fatal("[GODOTENV] error loading .env file |", err)
		os.Exit(1)
	}
	TOKEN := os.Getenv("DISCORD_TOKEN")
	ownerID = os.Getenv("OWNER_ID")
	inviteLink = os.Getenv("INVITE_LINK")

	// CREATING BOT INSTANCE
	discord, err := discordgo.New("Bot " + TOKEN)
	if err != nil {
		log.Fatal("[DISCORDGO] error creating discord session |", err)
		os.Exit(1)
	}

	// REGISTER CALLBACKS
	discord.AddHandler(ready)
	discord.AddHandler(connect)
	discord.AddHandler(disconnect)
	discord.AddHandler(resumed)
	discord.AddHandler(commandHandler)
	discord.AddHandler(checkOffline)
	discord.AddHandler(reaction)

	var (
		dmPermission            = false
		channelPermission int64 = discordgo.PermissionManageChannels

		commands = []*discordgo.ApplicationCommand{
			{
				Name:        "invite",
				Description: "Sends an invite link for the bot",
				Type:        discordgo.ChatApplicationCommand,
			},
			{
				Name:        "list",
				Description: "List bots being watched",
				Type:        discordgo.ChatApplicationCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        "server",
						Description: "List bots in the current server",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
					},
					{
						Name:        "subscriptions",
						Description: "List bots you're subscribed to",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
					},
				},
			},
			{
				Name:         "notify",
				Description:  "Subscribe to a bot to be notified when it goes online/offline",
				DMPermission: &dmPermission,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        "subscribe",
						Description: "[MUST ALLOW DMs] Subscribes to a bot",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Options: []*discordgo.ApplicationCommandOption{
							{
								Name:        "bot",
								Description: "The bot to subscribe to",
								Type:        discordgo.ApplicationCommandOptionUser,
								Required:    true,
							},
						},
					},
					{
						Name:        "unsubscribe",
						Description: "Unsubscribes from a bot",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Options: []*discordgo.ApplicationCommandOption{
							{
								Name:        "bot",
								Description: "The bot to unsubscribe from",
								Type:        discordgo.ApplicationCommandOptionUser,
								Required:    true,
							},
						},
					},
				},
			},
			{
				Name:        "privacy",
				Description: "Sends OfflineNotifier's privacy policy",
				Type:        discordgo.ChatApplicationCommand,
			},
			{
				Name:        "shutdown",
				Description: "Shuts down OfflineNotifier",
				Type:        discordgo.ChatApplicationCommand,
			},
			{
				Name:        "stats",
				Description: "Shows stats about OfflineNotifier",
				Type:        discordgo.ChatApplicationCommand,
			},
			{
				Name:        "support",
				Description: "Need help with OfflineNotifier? Join this server!",
				Type:        discordgo.ChatApplicationCommand,
			},
			{
				Name:                     "watch",
				DefaultMemberPermissions: &channelPermission,
				DMPermission:             &dmPermission,
				Description:              "Starts/stops watching bots in a server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        "set",
						Description: "Sets the channel OfflineNotifier will send messages in & starts watching a server",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
					},
					{
						Name:        "stop",
						Description: "Stops watching a server",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
					},
				},
			},
		}
	)
	// INTENTS
	discord.Identify.Intents = discordgo.IntentsAllWithoutPrivileged | discordgo.IntentGuildMembers | discordgo.IntentGuildPresences

	// open the websocket and begin listening
	err = discord.Open()
	if err != nil {
		log.Fatal("[DISCORDGO] error opening discord session |", err)
		os.Exit(1)
	}

	// write commands
	createdCommands, err := discord.ApplicationCommandBulkOverwrite(discord.State.User.ID, "", commands)
	if err != nil {
		log.Println("[COMMAND WRITE] error writing commands |", err)
		os.Exit(1)
	}

	for _, command := range createdCommands {
		if command.Name == "shutdown" {
			appID := command.ApplicationID
			cmdID := command.ID
			GID := command.GuildID
			permissions := []*discordgo.ApplicationCommandPermissions{{ID: ownerID, Type: 2, Permission: true}}
			permissionsList := discordgo.ApplicationCommandPermissionsList{Permissions: permissions}
			discord.ApplicationCommandPermissionsEdit(appID, GID, cmdID, &permissionsList)
		}
	}

	// wait here until CTRL-C or other term signal is received
	log.Println("[RUNNING]")
	fmt.Println("[RUNNING] Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// cleanly close down the discord session
	discord.Close()
}

// ----- BOT EVENTS

// called when OfflineNotifier connects to discord
func connect(s *discordgo.Session, event *discordgo.Connect) {
	log.Println("[CONNECTED]")
}

// called when OfflineNotifier gets disconnected from discord
func disconnect(s *discordgo.Session, event *discordgo.Disconnect) {
	log.Println("[DISCONNECTED]")
}

// called when discord responds with the ready event
func ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("[READY]")
	if !startedCoroutines {
		log.Println("[GOLANG] starting coroutines...")
		go requestBots(s)
		go queueHandler(s)
		startedCoroutines = true
	}
}

// called when connection resumes
func resumed(s *discordgo.Session, event *discordgo.Resumed) {
	log.Println("[RESUMED]")
}

// called when OfflineNotifier receives GuildMembersChunk
func checkOffline(s *discordgo.Session, event *discordgo.GuildMembersChunk) {
	botMap, err := getBotMap(s)
	if err != nil {
		logMessage(s, "[CHECK OFFLINE] error getting bot map |", err)
		return
	}

	guild, err := getGuild(s, event.GuildID)
	if err != nil {
		logMessage(s, "[CHECK OFFLINE] error getting json guild |", err)
		return
	}

	for _, BID := range guild.Bots {
		bot, err := s.GuildMember(guild.ID, BID)
		if err != nil {
			logMessage(s, "[CHECK OFFLINE] error getting discord bot |", err)
			continue
		}
		currentStatus := findStatus(event.Presences, BID)
		if botMap[BID].Status != currentStatus {
			if botMap[BID].Status == "offline" || currentStatus == "offline" {
				addToQueue("ss", [4]string{BID, currentStatus, "true"})
				deltaTime := calculateDeltaTime(botMap[BID].Timestamp)
				var embed *discordgo.MessageEmbed
				switch botMap[BID].Status {
				case "unknown":
					continue
				case "offline":
					// back online
					embed = &discordgo.MessageEmbed{
						Title:       (bot.User.Username + " is back online"),
						Description: "```TOTAL DOWNTIME\n" + deltaTime + "```",
						Color:       onlineColor,
						Timestamp:   time.Now().UTC().Format(time.RFC3339),
					}
				default:
					// now offline
					embed = &discordgo.MessageEmbed{
						Title:       (bot.User.Username + " is now offline"),
						Description: "```TOTAL UPTIME\n" + deltaTime + "```",
						Color:       offlineColor,
						Timestamp:   time.Now().UTC().Format(time.RFC3339),
					}
				}
				// notify servers
				for _, GID := range botMap[BID].Guilds {
					notifyGuild, err := getGuild(s, GID)
					if err != nil {
						log.Println("[CHECK OFFLINE] error getting notify guild |", err)
						continue
					}
					go sendEmbed(s, notifyGuild.CID, embed)
				}
				// notify subscribers
				for _, SID := range botMap[BID].Subscribers {
					userDM, err := s.UserChannelCreate(SID)
					if err != nil {
						log.Println("[CHECK OFFLINE] error creating DM channel |", err)
						continue
					}
					go sendEmbed(s, userDM.ID, embed)
				}
			} else {
				addToQueue("ss", [4]string{BID, currentStatus, "false"})
			}
		}
	}
}

func reaction(s *discordgo.Session, event *discordgo.MessageReactionAdd) {
	message, err := s.ChannelMessage(event.ChannelID, event.MessageID)
	if err != nil {
		logMessage(s, "[REACTION] error getting message |", err)
	}

	user, err := s.User(event.UserID)
	if err != nil {
		logMessage(s, "[REACTION] error getting discord user |", err)
	}

	if message.Embeds != nil && user.ID != s.State.User.ID {
		if strings.Contains(message.Embeds[0].Title, "'s subscriptions") || strings.Contains(message.Embeds[0].Title, "Bots being watched in ") {
			// get bot list
			var bots []string
			if strings.Contains(message.Embeds[0].Title, "'s subscriptions") {
				// subscriber
				userName := strings.Replace(message.Embeds[0].Title, "'s subscriptions", "", 1)
				if user.Username == userName {
					subscriber, err := getSubscriber(s, user.ID)
					if err != nil {
						logMessage(s, "[REACTION] error getting json bot |", err)
						return
					}
					bots = subscriber.Bots
				} else {
					return
				}
			} else {
				// guild
				guild, err := getGuild(s, event.GuildID)
				if err != nil {
					logMessage(s, "[REACTION] error getting json guild |", err)
					return
				}
				bots = guild.Bots
			}

			// get new page
			page, err := strconv.Atoi(strings.Split(message.Embeds[0].Footer.Text, "/")[0])
			if err != nil {
				logMessage(s, "[REACTION] error converting string to int |", err)
				return
			}
			maxPage := int(math.Ceil(float64(len(bots)) / 9))
			if event.MessageReaction.Emoji.Name == "➡️" {
				if page >= maxPage {
					page = 1
				} else {
					page++
				}
			} else if event.MessageReaction.Emoji.Name == "⬅" {
				if page == 1 {
					page = maxPage
				} else {
					page--
				}
			}

			// update embed
			embed := message.Embeds[0]
			embed.Fields = []*discordgo.MessageEmbedField{}
			embed.Timestamp = time.Now().UTC().Format(time.RFC3339)
			err = makeBotList(s, embed, bots, page)
			if err != nil {
				logMessage(s, "[LIST SUBSCRIBER] error making list |", err)
				return
			}
			s.ChannelMessageEditEmbed(message.ChannelID, message.ID, embed)
		}
	}
}

// ----- BOT COMMANDS
// invite
// list
// - server
// - subscriptions
// notify
// - subscribe [bot]
// - unsubscribe [bot]
// privacy
// shutdown
// stats
// support
// watch
// - set
// - stop

// receives slash command interactions and runs the respective command
func commandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	switch i.ApplicationCommandData().Name {
	case "invite":
		invite(s, i)
	case "list":
		switch i.ApplicationCommandData().Options[0].Name {
		case "server":
			listServer(s, i)
		case "subscriptions":
			listSubscriptions(s, i)
		}
	case "notify":
		switch i.ApplicationCommandData().Options[0].Name {
		case "subscribe":
			subscribe(s, i)
		case "unsubscribe":
			unsubscribe(s, i)
		}
	case "privacy":
		privacy(s, i)
	case "shutdown":
		shutdown(s, i)
	case "stats":
		stats(s, i)
	case "support":
		support(s, i)
	case "watch":
		switch i.ApplicationCommandData().Options[0].Name {
		case "set":
			set(s, i)
		case "stop":
			stop(s, i)
		}
	}
}

// sets the channel that OfflineNotifier will use
func set(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var embed []*discordgo.MessageEmbed
	addToQueue("ac", [4]string{i.GuildID, i.ChannelID})
	embed = []*discordgo.MessageEmbed{
		{
			Title: "Set channel request successful",
			Color: successColor,
		},
	}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
}

// sends an invite link for the bot
func invite(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := []*discordgo.MessageEmbed{
		{
			Title: "Want OfflineNotifier in your server? Use this link!",
			URL:   inviteLink,
			Color: defaultColor,
		},
	}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
}

// lists the bots being watched in this server
func listServer(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// check if in a server
	if i.GuildLocale == nil {
		embed := []*discordgo.MessageEmbed{
			{
				Title:       "List server failed",
				Description: "You're not in a server",
				Color:       failColor,
			},
		}
		responseData := &discordgo.InteractionResponseData{Embeds: embed}
		response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
		go s.InteractionRespond(i.Interaction, response)
		return
	}

	// get guild
	discordGuild, err := s.Guild(i.GuildID)
	if err != nil {
		logMessage(s, "[LIST SERVER] error getting discord guild |", err)
		return
	}
	guild, err := getGuild(s, discordGuild.ID)
	if err != nil {
		embeds := []*discordgo.MessageEmbed{
			{
				Title: "Bots aren't being watched in " + discordGuild.Name,
				Color: failColor,
			},
		}
		responseData := &discordgo.InteractionResponseData{Embeds: embeds}
		response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
		go s.InteractionRespond(i.Interaction, response)
		return
	}

	// make placeholder embed
	embeds := []*discordgo.MessageEmbed{
		{
			Title: "Loading bots...",
			Color: defaultColor,
		},
	}
	data := &discordgo.InteractionResponseData{Embeds: embeds}
	response := &discordgo.InteractionResponse{Type: 4, Data: data}
	go s.InteractionRespond(i.Interaction, response)

	// make embed
	embeds = []*discordgo.MessageEmbed{
		{
			Title:     "Bots being watched in " + discordGuild.Name,
			Color:     defaultColor,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}
	err = makeBotList(s, embeds[0], guild.Bots, 1)
	if err != nil {
		logMessage(s, "[LIST SERVER] error making list |", err)
		return
	}
	edit := &discordgo.WebhookEdit{Embeds: &embeds}
	message, err := s.InteractionResponseEdit(i.Interaction, edit)
	if err != nil {
		logMessage(s, "[LIST SERVER] error editing response |", err)
		return
	}
	s.MessageReactionAdd(message.ChannelID, message.ID, "⬅")
	s.MessageReactionAdd(message.ChannelID, message.ID, "➡️")
}

// lists the bots being watched by a subscriber
func listSubscriptions(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// get subscriber
	var user *discordgo.User
	if i.User != nil {
		user = i.User
	} else {
		user = i.Member.User
	}
	subscriber, err := getSubscriber(s, user.ID)
	if err != nil {
		embeds := []*discordgo.MessageEmbed{
			{
				Title: "You're not subscribed to any bots!",
				Color: failColor,
			},
		}
		responseData := &discordgo.InteractionResponseData{Embeds: embeds}
		response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
		go s.InteractionRespond(i.Interaction, response)
		return
	}

	// placeholder
	embeds := []*discordgo.MessageEmbed{
		{
			Title: "Loading subscriptions...",
			Color: defaultColor,
		},
	}
	data := &discordgo.InteractionResponseData{Embeds: embeds}
	response := &discordgo.InteractionResponse{Type: 4, Data: data}
	go s.InteractionRespond(i.Interaction, response)

	// make embed
	embeds = []*discordgo.MessageEmbed{
		{
			Title:     user.Username + "'s subscriptions",
			Color:     defaultColor,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}
	err = makeBotList(s, embeds[0], subscriber.Bots, 1)
	if err != nil {
		logMessage(s, "[LIST SUBSCRIBER] error making list |", err)
		return
	}
	edit := &discordgo.WebhookEdit{Embeds: &embeds}
	message, err := s.InteractionResponseEdit(i.Interaction, edit)
	if err != nil {
		logMessage(s, "[LIST SUBSCRIBER] error editing response |", err)
		return
	}
	s.MessageReactionAdd(message.ChannelID, message.ID, "⬅")
	s.MessageReactionAdd(message.ChannelID, message.ID, "➡️")
}

// sends OfflineNotifier's privacy policy
func privacy(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := []*discordgo.MessageEmbed{
		{
			Title:       "Privacy policy",
			Description: "```Data is never shared with anyone and is only ever used within the bot. OfflineNotifier stores the IDs of users, bots, servers, and channels along with bots' current online/offline status. We uses this data to track a bot's status and to send a notification in the right server and channel. We only store the user IDs of people subscribed to bots. To remove your user ID, unsubscribe from all bots. To remove your server's info, use /watch stop. Questions? Join the server at /support.```",
			Color:       defaultColor,
		},
	}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
}

func shutdown(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := []*discordgo.MessageEmbed{
		{
			Title: "Shutting down...",
			Color: defaultColor,
		},
	}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
}

// shows stats about OfflineNotifier
func stats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// calculate totals
	totalServers := int64(len(s.State.Guilds))
	guilds, err := getGuildMap(s)
	if err != nil {
		logMessage(s, "[STATS] error getting guild map |", err)
	}

	totalActive := int64(len(guilds))
	var totalBots int64
	for _, guild := range guilds {
		for range guild.Bots {
			totalBots += 1
		}
	}

	// calculate uptime
	uptime := calculateDeltaTime(startTime)

	// make embed
	embed := []*discordgo.MessageEmbed{{
		Title:     "OfflineNotifier stats",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Color:     defaultColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Total servers", Value: "```" + strconv.FormatInt(totalServers, 10) + "```", Inline: true},
			{Name: "Active servers", Value: "```" + strconv.FormatInt(totalActive, 10) + "```", Inline: true},
			{Name: "Bots watching", Value: "```" + strconv.FormatInt(totalBots, 10) + "```", Inline: true},
			{Name: "Bot version", Value: "```" + botVersion + "```", Inline: true},
			{Name: "Uptime", Value: "```" + uptime + "```", Inline: true},
		},
	}}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
}

// stops watching a server
func stop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var embed []*discordgo.MessageEmbed
	addToQueue("rg", [4]string{i.GuildID})
	embed = []*discordgo.MessageEmbed{
		{
			Title: "Stop request successful",
			Color: successColor,
		},
	}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
}

// subscribes to a bot
func subscribe(s *discordgo.Session, i *discordgo.InteractionCreate) {
	BID := i.ApplicationCommandData().Options[0].Options[0].Value.(string)
	UID := i.Member.User.ID

	var embed []*discordgo.MessageEmbed
	discordBot, err := s.GuildMember(i.GuildID, BID)
	if err != nil {
		logMessage(s, "[SUBSCRIBE] error getting discord bot |", err)
		embed = []*discordgo.MessageEmbed{{
			Title:       "Subscribe request failed",
			Description: "Error finding bot!",
			Color:       failColor,
		}}
	} else {
		if discordBot.User.Bot && BID != s.State.User.ID {
			addToQueue("as", [4]string{UID, BID})
			embed = []*discordgo.MessageEmbed{{
				Title:       "Subscribe request successful",
				Description: "You are now subscribed to " + discordBot.User.Username,
				Color:       successColor,
			}}
		} else {
			embed = []*discordgo.MessageEmbed{{
				Title:       "Subscribe request failed",
				Description: discordBot.User.Username + " is not a valid bot!",
				Color:       failColor,
			}}
		}
	}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
}

// support - sends a server invite for nooby's bot sanctuary
func support(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := []*discordgo.MessageEmbed{{
		Title: "Need help with OfflineNotifier? Join this server!",
		URL:   "https://discord.gg/YDRKdkh",
		Color: defaultColor,
	}}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
}

// unsubscribes from a bot
func unsubscribe(s *discordgo.Session, i *discordgo.InteractionCreate) {
	BID := i.ApplicationCommandData().Options[0].Options[0].Value.(string)
	UID := i.Member.User.ID

	addToQueue("rs", [4]string{UID, BID})
	embed := []*discordgo.MessageEmbed{{
		Title: "Unsubscribe request successful!",
		Color: successColor,
	}}
	responseData := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: responseData}
	go s.InteractionRespond(i.Interaction, response)
}

// ----- ARRAY FUNCTIONS

// looks through a discord presence array for a matching ID, returns the Status
func findStatus(presenceList []*discordgo.Presence, userID string) string {
	for _, p := range presenceList {
		if p.User.ID == userID {
			return string(p.Status)
		}
	}
	return "offline"
}

// looks through an ID array to find the matching ID, returns the index
func indexID(array []string, ID string) (i int, err error) {
	for i = range array {
		if array[i] == ID {
			return
		}
	}
	err = errors.New("not in list")
	return
}

// ----- MAP FUNCTIONS

// reads from a json file and returns a specific bot
func getBot(s *discordgo.Session, BID string) (bot Bot, err error) {
	jsonData, err := ioutil.ReadFile("data.json")
	if err != nil {
		return
	}

	var jsonMap map[string]map[string]map[string]interface{} // hehehehehehe....
	err = json.Unmarshal(jsonData, &jsonMap)
	if err != nil {
		return
	}

	botValue, exists := jsonMap["bots"][BID]
	if exists {
		bot = Bot{ID: BID}
		if botValue["guilds"] != nil {
			for _, guild := range botValue["guilds"].([]interface{}) {
				bot.Guilds = append(bot.Guilds, guild.(string))
			}
		}
		if botValue["subscribers"] != nil {
			for _, subscriber := range botValue["subscribers"].([]interface{}) {
				bot.Subscribers = append(bot.Subscribers, subscriber.(string))
			}
		}
		if botValue["status"] != nil {
			bot.Status = botValue["status"].(string)
		}
		if botValue["timestamp"] != nil {
			bot.Timestamp = int64(botValue["timestamp"].(float64))
		}
	} else {
		err = errors.New("bot not found")
	}
	return
}

// reads from a json file and returns a bot map
func getBotMap(s *discordgo.Session) (botMap map[string]Bot, err error) {
	jsonData, err := ioutil.ReadFile("data.json")
	if err != nil {
		return
	}

	var jsonMap map[string]map[string]map[string]interface{} // MWAHAHAHAHAHA
	err = json.Unmarshal(jsonData, &jsonMap)
	if err != nil {
		return
	}

	botsInterface := jsonMap["bots"]
	botMap = make(map[string]Bot)
	for botKey, botValue := range botsInterface {
		bot := Bot{ID: botKey}
		if botValue["guilds"] != nil {
			for _, guild := range botValue["guilds"].([]interface{}) {
				bot.Guilds = append(bot.Guilds, guild.(string))
			}
		}
		if botValue["subscribers"] != nil {
			for _, subscriber := range botValue["subscribers"].([]interface{}) {
				bot.Subscribers = append(bot.Subscribers, subscriber.(string))
			}
		}
		if botValue["status"] != nil {
			bot.Status = botValue["status"].(string)
		}
		if botValue["timestamp"] != nil {
			bot.Timestamp = int64(botValue["timestamp"].(float64))
		}
		botMap[botKey] = bot
	}
	return
}

// reads from a json file and returns a specific guild
func getGuild(s *discordgo.Session, GID string) (guild Guild, err error) {
	jsonData, err := ioutil.ReadFile("data.json")
	if err != nil {
		return
	}

	var jsonMap map[string]map[string]map[string]interface{} // it works just trust
	err = json.Unmarshal(jsonData, &jsonMap)
	if err != nil {
		return
	}

	guildValue, exists := jsonMap["guilds"][GID]
	if exists {
		guild = Guild{ID: GID}
		if guildValue["cid"] != nil {
			guild.CID = guildValue["cid"].(string)
		}
		if guildValue["bots"] != nil {
			for _, bot := range guildValue["bots"].([]interface{}) {
				guild.Bots = append(guild.Bots, bot.(string))
			}
		}
	} else {
		err = errors.New("guild not found")
	}
	return
}

// reads from a json file and returns a guild map
func getGuildMap(s *discordgo.Session) (guildMap map[string]Guild, err error) {
	jsonData, err := ioutil.ReadFile("data.json")
	if err != nil {
		return
	}

	var jsonMap map[string]map[string]map[string]interface{}
	err = json.Unmarshal(jsonData, &jsonMap)
	if err != nil {
		return
	}

	guildInterface := jsonMap["guilds"]
	guildMap = make(map[string]Guild)
	for guildKey, guildValue := range guildInterface {
		guild := Guild{ID: guildKey}
		if guildValue["cid"] != nil {
			guild.CID = guildValue["cid"].(string)
		}
		if guildValue["bots"] != nil {
			for _, bot := range guildValue["bots"].([]interface{}) {
				guild.Bots = append(guild.Bots, bot.(string))
			}
		}
		guildMap[guildKey] = guild
	}
	return
}

// reads from a json file and returns a subscriber
func getSubscriber(s *discordgo.Session, SID string) (subscriber Subscriber, err error) {
	jsonData, err := ioutil.ReadFile("data.json")
	if err != nil {
		return
	}

	var jsonMap map[string]map[string]map[string]interface{} // hehehehehehe....
	err = json.Unmarshal(jsonData, &jsonMap)
	if err != nil {
		return
	}

	subscriberValue, exists := jsonMap["subscribers"][SID]
	if exists {
		subscriber = Subscriber{ID: SID}
		if subscriberValue["bots"] != nil {
			for _, bot := range subscriberValue["bots"].([]interface{}) {
				subscriber.Bots = append(subscriber.Bots, bot.(string))
			}
		} else {
			err = errors.New("no bots found")
		}
	} else {
		err = errors.New("subscriber not found")
	}
	return
}

// reads from a json file and returns a subscriber map
func getSubscriberMap(s *discordgo.Session) (subscriberMap map[string]Subscriber, err error) {
	jsonData, err := ioutil.ReadFile("data.json")
	if err != nil {
		return
	}

	var jsonMap map[string]map[string]map[string]interface{}
	err = json.Unmarshal(jsonData, &jsonMap)
	if err != nil {
		return
	}

	subscribersInterface := jsonMap["subscribers"]
	subscriberMap = make(map[string]Subscriber)
	for subscriberKey, subscriberValue := range subscribersInterface {
		subscriber := Subscriber{ID: subscriberKey}
		if subscriberValue["bots"] != nil {
			for _, bot := range subscriberValue["bots"].([]interface{}) {
				subscriber.Bots = append(subscriber.Bots, bot.(string))
			}
		}
		subscriberMap[subscriberKey] = subscriber
	}
	return
}

// ----- FRAMEWORK FUNCTIONS

// adds actions into the action queue
func addToQueue(action string, data [4]string) {
	request := Request{action, data}
	actionQueue = append(actionQueue, request)
}

// calculate time delta
func calculateDeltaTime(unixTime int64) string {
	// calculate
	currentTime := time.Now().Unix()
	diffUnix := currentTime - unixTime
	diffTime := time.Unix(diffUnix, 0).UTC()

	// format
	day := strconv.FormatInt(int64((diffTime.YearDay()-1)*(diffTime.Year()-1969)), 10)
	hour := strconv.FormatInt(int64(diffTime.Hour()), 10)
	minute := strconv.FormatInt(int64(diffTime.Minute()), 10)
	second := strconv.FormatInt(int64(diffTime.Second()), 10)
	return day + "D " + hour + "H " + minute + "M " + second + "S"
}

// sends strings to the log and then DMs the bot owner
func logMessage(s *discordgo.Session, v ...interface{}) {
	log.Println(v...)
	dmChannel, err := s.UserChannelCreate(ownerID)
	if err != nil {
		log.Println("[LOG MESSAGE] error creating DM channel |", err)
		return
	}
	embed := discordgo.MessageEmbed{
		Title: fmt.Sprintln(v...),
		Color: failColor,
	}
	sendEmbed(s, dmChannel.ID, &embed)
}

// sends an embed
func sendEmbed(s *discordgo.Session, CID string, embed *discordgo.MessageEmbed) {
	_, err := s.ChannelMessageSendEmbed(CID, embed)
	if err != nil {
		logMessage(s, "[SEND EMBED] embed failed to send |", err)
	}
}

func getDiscordBot(s *discordgo.Session, bot Bot) (discordBot *discordgo.Member, err error) {
	for _, GID := range bot.Guilds {
		discordBot, err = s.GuildMember(GID, bot.ID)
		if err == nil {
			break
		}
	}
	return
}

// makes list for bot embed
func makeBotList(s *discordgo.Session, embed *discordgo.MessageEmbed, bots []string, page int) error {
	pageStart := ((page - 1) * 8)
	pageEnd := page*8 + 1 // len(bots) = 1 (0 - 1); pageEnd =
	if pageEnd > len(bots) {
		pageEnd = len(bots)
	}
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: fmt.Sprintf("%d/%d", page, int(math.Ceil(float64(len(bots))/9))),
	}

	for _, BID := range bots[pageStart:pageEnd] {
		bot, err := getBot(s, BID)
		if err != nil {
			logMessage(s, "[MAKE BOT LIST] error getting json bot |", err)
			return err
		}

		discordBot, err := getDiscordBot(s, bot)
		if err != nil {
			logMessage(s, "[MAKE BOT LIST] error getting discord bot |", err)
			return err
		}

		deltaTime := calculateDeltaTime(bot.Timestamp)
		deltaName := "UPTIME"
		if bot.Status == "offline" || bot.Status == "unknown" {
			deltaName = "DOWNTIME"
		}

		newField := &discordgo.MessageEmbedField{
			Name:   discordBot.User.Username,
			Value:  "```\nLAST STATUS\n" + bot.Status + "\n-----------\n" + deltaName + "\n" + deltaTime + "```",
			Inline: true,
		}
		embed.Fields = append(embed.Fields, newField)
	}
	return nil
}

// ----- LOOP FUNCTIONS

// loops forever; reads from action queue and does subsequent actions
func queueHandler(s *discordgo.Session) {
	for {
		time.Sleep(10 * time.Millisecond)
		if len(actionQueue) > 0 {
			// get guild map
			guildMap, err := getGuildMap(s)
			if err != nil {
				logMessage(s, "[QUEUE HANDLER] error getting guild map |", err)
				continue
			}

			// get bot map
			botMap, err := getBotMap(s)
			if err != nil {
				logMessage(s, "[QUEUE HANDLER] error getting bot map |", err)
				continue
			}

			// get subscriber map
			subscriberMap, err := getSubscriberMap(s)
			if err != nil {
				logMessage(s, "[QUEUE HANDLER] error getting subscriber map |", err)
				continue
			}

			// go through action queue
			for len(actionQueue) > 0 {
				request := actionQueue[0]
				switch request.action {
				// ASSIGN CHANNEL - [GID, CID]
				case "ac":
					GID := request.data[0]
					CID := request.data[1]

					guild, exists := guildMap[GID]
					if exists {
						guild.CID = CID
						guildMap[GID] = guild
					} else {
						guild = Guild{
							ID:   GID,
							CID:  CID,
							Bots: []string{},
						}
						guildMap[GID] = guild
					}
				// REMOVE GUILD - [GID]
				case "rg":
					GID := request.data[0]

					_, exists := guildMap[GID]
					if exists {
						// loop through guild's bot list
						for _, BID := range guildMap[GID].Bots {
							bot, exists := botMap[BID]
							if exists {
								i, err := indexID(bot.Guilds, GID)
								if err != nil {
									logMessage(s, "[REMOVE GUILD] error indexing GID |", err)
									continue
								}
								// remove GID from bot's guild list
								if len(bot.Guilds) == 1 {
									// loop through bot's subscriber list
									for _, SID := range bot.Subscribers {
										subscriber, exists := subscriberMap[SID]
										if exists {
											i, err := indexID(subscriber.Bots, BID)
											if err != nil {
												logMessage(s, "[REMOVE GUILD] error indexing BID |", err)
												continue
											}
											// remove BID from subscriber's bot list
											if len(subscriber.Bots) == 1 {
												delete(subscriberMap, SID)
											} else {
												subscriber.Bots[i] = subscriber.Bots[len(subscriber.Bots)-1]
												subscriber.Bots = subscriber.Bots[:len(subscriber.Bots)-1]
												subscriberMap[SID] = subscriber
											}
										} else {
											logMessage(s, "[REMOVE GUILD] error finding subscriber | not in subscriber map")
											continue
										}
									}
									delete(botMap, BID)
								} else {
									bot.Guilds[i] = bot.Guilds[len(bot.Guilds)-1]
									bot.Guilds = bot.Guilds[:len(bot.Guilds)-1]
									botMap[BID] = bot
								}
							} else {
								logMessage(s, "[REMOVE GUILD] error finding bot | not in bot map")
								actionQueue = actionQueue[1:]
								continue
							}
						}
						// delete guild
						delete(guildMap, GID)
					} else {
						logMessage(s, "[REMOVE GUILD] error finding guild | not in guild map")
						actionQueue = actionQueue[1:]
						continue
					}
				// SET STATUS - [BID, Status, changeTimestamp]
				case "ss":
					BID := request.data[0]
					Status := request.data[1]

					bot, exists := botMap[BID]
					if exists {
						bot.Status = Status
						if request.data[2] == "true" {
							bot.Timestamp = time.Now().Unix()
						}
						botMap[BID] = bot
					} else {
						logMessage(s, "[SET STATUS] error finding bot | not in bot map")
						actionQueue = actionQueue[1:]
						continue
					}
				// ADD BOT - [GID, BID]
				case "ab":
					GID := request.data[0]
					BID := request.data[1]

					// add BID to guild's bot list
					guild, exists := guildMap[GID]
					if exists {
						// check for duplicate
						_, err = indexID(guild.Bots, BID)
						if err != nil {
							guild.Bots = append(guild.Bots, BID)
							guildMap[GID] = guild
						}
					} else {
						logMessage(s, "[ADD BOT] error finding guild | not in guild map")
						actionQueue = actionQueue[1:]
						continue
					}

					// add GID to bot's guild list
					bot, exists := botMap[BID]
					if exists {
						// check for duplicate
						_, err = indexID(bot.Guilds, GID)
						if err != nil {
							bot.Guilds = append(bot.Guilds, GID)
							botMap[BID] = bot
						}
					} else {
						botMap[BID] = Bot{
							ID:          BID,
							Guilds:      []string{GID},
							Subscribers: []string{},
							Status:      "unknown",
							Timestamp:   time.Now().Unix(),
						}
					}
				// REMOVE BOT - [GID, BID]
				case "rb":
					GID := request.data[0]
					BID := request.data[1]

					guild, exists := guildMap[GID]
					if exists {
						i, err := indexID(guild.Bots, BID)
						if err != nil {
							logMessage(s, "[REMOVE BOT] error indexing BID |", err)
							actionQueue = actionQueue[1:]
							continue
						}
						// remove BID from guild's bot list
						if len(guild.Bots) == 1 {
							guild.Bots = []string{}
							guildMap[GID] = guild
						} else {
							guild.Bots[i] = guild.Bots[len(guild.Bots)-1]
							guild.Bots = guild.Bots[:len(guild.Bots)-1]
							guildMap[GID] = guild
						}
					} else {
						logMessage(s, "[REMOVE BOT] error finding guild | not in guild map")
						actionQueue = actionQueue[1:]
						continue
					}

					bot, exists := botMap[BID]
					if exists {
						i, err := indexID(bot.Guilds, GID)
						if err != nil {
							logMessage(s, "[REMOVE BOT] error indexing GID |", err)
							actionQueue = actionQueue[1:]
							continue
						}
						// remove GID from bot's guild list
						if len(bot.Guilds) == 1 {
							// loop through bot's subscriber list
							for _, SID := range bot.Subscribers {
								subscriber, exists := subscriberMap[SID]
								if exists {
									i, err := indexID(subscriber.Bots, BID)
									if err != nil {
										logMessage(s, "[REMOVE BOT] error indexing BID |", err)
										continue
									}
									// remove BID from subscriber's bot list
									if len(subscriber.Bots) == 1 {
										delete(subscriberMap, SID)
									} else {
										subscriber.Bots[i] = subscriber.Bots[len(subscriber.Bots)-1]
										subscriber.Bots = subscriber.Bots[:len(subscriber.Bots)-1]
										subscriberMap[SID] = subscriber
									}
								} else {
									logMessage(s, "[REMOVE BOT] error finding subscriber | not in subscriber map")
									continue
								}
							}
							delete(botMap, BID)
						} else {
							bot.Guilds[i] = bot.Guilds[len(bot.Guilds)-1]
							bot.Guilds = bot.Guilds[:len(bot.Guilds)-1]
							botMap[BID] = bot
						}
					} else {
						logMessage(s, "[ADD BOT] error finding bot | not in bot map")
						actionQueue = actionQueue[1:]
						continue
					}
				// ADD SUBSCRIBER [SID, BID]
				case "as":
					SID := request.data[0]
					BID := request.data[1]

					bot, exists := botMap[BID]
					if exists {
						// add SID to bot's subscriber list
						// check for duplicate
						_, err = indexID(bot.Subscribers, SID)
						if err != nil {
							bot.Subscribers = append(bot.Subscribers, SID)
							botMap[BID] = bot
						}
					} else {
						logMessage(s, "[ADD SUBSCRIBER] error finding bot | not in bot map")
						actionQueue = actionQueue[1:]
						continue
					}

					// add BID to subscriber's bot list
					subscriber, exists := subscriberMap[SID]
					if exists {
						// check for duplicate
						_, err = indexID(subscriber.Bots, BID)
						if err != nil {
							subscriber.Bots = append(subscriber.Bots, BID)
							subscriberMap[SID] = subscriber
						}
					} else {
						subscriberMap[SID] = Subscriber{
							ID:   SID,
							Bots: []string{BID},
						}
					}
				// REMOVE SUBSCRIBER [SID, BID]
				case "rs":
					SID := request.data[0]
					BID := request.data[1]

					bot, exists := botMap[BID]
					if exists {
						i, err := indexID(bot.Subscribers, SID)
						if err != nil {
							logMessage(s, "[REMOVE SUBSCRIBER] error indexing SID |", err)
							actionQueue = actionQueue[1:]
							continue
						}
						// remove SID from bot's subscriber list
						if len(bot.Subscribers) == 1 {
							bot.Subscribers = []string{}
							botMap[BID] = bot
						} else {
							bot.Subscribers[i] = bot.Subscribers[len(bot.Subscribers)-1]
							bot.Subscribers = bot.Subscribers[:len(bot.Subscribers)-1]
							botMap[BID] = bot
						}
					} else {
						logMessage(s, "[REMOVE SUBSCRIBER] error finding bot | not in bot map")
						actionQueue = actionQueue[1:]
						continue
					}

					subscriber, exists := subscriberMap[SID]
					if exists {
						i, err := indexID(subscriber.Bots, BID)
						if err != nil {
							logMessage(s, "[REMOVE SUBSCRIBER] error indexing BID |", err)
							actionQueue = actionQueue[1:]
							continue
						}
						// remove BID from subscriber's bot list
						if len(subscriber.Bots) == 1 {
							delete(subscriberMap, SID)
						} else {
							subscriber.Bots[i] = subscriber.Bots[len(subscriber.Bots)-1]
							subscriber.Bots = subscriber.Bots[:len(subscriber.Bots)-1]
							subscriberMap[SID] = subscriber
						}
					} else {
						logMessage(s, "[REMOVE SUBSCRIBER] error finding subscriber | not in subscriber map")
						actionQueue = actionQueue[1:]
						continue
					}
				}
				// POP ACTION FROM QUEUE
				actionQueue = actionQueue[1:]
			}
			jsonData, err := json.Marshal(map[string]interface{}{"guilds": guildMap, "bots": botMap, "subscribers": subscriberMap})
			if err != nil {
				logMessage(s, "[QUEUE HANDLER] error marshaling json |", err)
			}
			ioutil.WriteFile("data.json", jsonData, 0755)
			if err != nil {
				logMessage(s, "[QUEUE HANDLER] error writing json |", err)
			}
		}
	}
}

// loops forever; goes through active guild list and checks for new bots,
// requests presence list of bots, and culls removed bots.
func requestBots(s *discordgo.Session) {
	for {
		time.Sleep(1 * time.Second)

		// get guild map
		guildMap, err := getGuildMap(s)
		if err != nil {
			logMessage(s, "[REQUEST BOTS] error getting guild map |", err)
			continue
		}

		// update presence
		botMap, err := getBotMap(s)
		if err != nil {
			logMessage(s, "[REQUEST BOTS] error getting bot map |", err)
			continue
		}
		s.UpdateWatchStatus(0, fmt.Sprint(len(botMap), " bots"))

		// range through guilds
		for GID, guild := range guildMap {
			// check if OfflineNotifier is still in guild
			_, err := s.Guild(GID)
			if err != nil {
				logMessage(s, "[REQUEST BOTS] error getting discord guild |", err)
				continue
			}

			// check if OfflineNotifier is still in channel
			_, err = s.Channel(guild.CID)
			if err != nil {
				logMessage(s, "[REQUEST BOTS] error setting message channel |", err, "| removing guild...")
				addToQueue("rg", [4]string{guild.ID})
				continue
			}

			// get member list
			memberList, err := s.GuildMembers(GID, "", 1000)
			if err != nil {
				logMessage(s, "[REQUEST BOTS] error getting member list |", err)
				continue
			}

			var bots = guild.Bots
			// add bots to request list
			for _, member := range memberList {
				if member.User.Bot && member.User.ID != s.State.User.ID {
					i, err := indexID(bots, member.User.ID)
					if err != nil {
						// bot is not in data yet, add them
						addToQueue("ab", [4]string{guild.ID, member.User.ID})
						continue
					}
					// pop element from list
					if len(bots) != 0 {
						bots[len(bots)-1], bots[i] = bots[i], bots[len(bots)-1]
						bots = bots[:len(bots)-1]
					}
				}
			}

			// cull remaining bots
			for _, cullBot := range bots {
				addToQueue("rb", [4]string{guild.ID, cullBot})
			}

			// request bot list
			err = s.RequestGuildMembersList(GID, guild.Bots, 0, "", true)
			if err != nil && err != discordgo.ErrWSNotFound {
				logMessage(s, "[REQUEST BOTS] error requesting bots list |", err)
			}
		}
	}
}

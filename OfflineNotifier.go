package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
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
	botVersion       = "V6.4 | GOLANG"
	actionQueue      []Request
	startTime        = time.Now().Unix()
	startedFunctions = false
	connected        = false
	inviteLink       = ""
	ownerID          = ""
	defaultColor     = 0x7289da
	successColor     = 0x76e06e
	failColor        = 0xe06c6c
	onlineColor      = 0x43b581
	offlineColor     = 0x727c8a
)

// ----- STRUCTS
type Request struct {
	action string
	data   [4]string
}

type Bot struct {
	ID        string
	status    string
	timestamp int64
}

type Guild struct {
	ID   string
	bots []Bot
	CID  string
}

// -----  MAIN
func main() {
	// LOGGING
	f, err := os.OpenFile("./logs/"+time.Now().Format("2006-01-02 15:04:05 MST"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
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

	// REGISTER SLASH COMMANDS
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "assign",
			Type:        discordgo.ChatApplicationCommand,
			Description: "Sets the channel that OfflineNotifier will use (MUST HAVE MANAGE CHANNEL PERMS TO USE)",
		},
		{
			Name:        "stop",
			Type:        discordgo.ChatApplicationCommand,
			Description: "OfflineNotifier will stop watching bots in this server (MUST HAVE MANAGE CHANNEL PERMS TO USE)",
		},
		{
			Name:        "list",
			Type:        discordgo.ChatApplicationCommand,
			Description: "Lists the bots being watched in this server (MUST BE IN AN ACTIVE SERVER TO USE)",
		},
		{
			Name:        "invite",
			Type:        discordgo.ChatApplicationCommand,
			Description: "Sends an invite link for the bot",
		},
		{
			Name:        "stats",
			Type:        discordgo.ChatApplicationCommand,
			Description: "Shows stats about OfflineNotifier",
		},
		{
			Name:        "support",
			Type:        discordgo.ChatApplicationCommand,
			Description: "Need help with OfflineNotifier? Join this server!",
		},
	}

	// INTENTS
	discord.Identify.Intents = discordgo.IntentsAllWithoutPrivileged | discordgo.IntentGuildMembers | discordgo.IntentGuildPresences

	// open the websocket and begin listening
	err = discord.Open()
	if err != nil {
		log.Fatal("[DISCORDGO] error opening discord session |", err)
		os.Exit(1)
	}

	// write commands
	_, err = discord.ApplicationCommandBulkOverwrite(discord.State.User.ID, "", commands)
	if err != nil {
		log.Println("[COMMAND] error writing commands |", err)
		os.Exit(1)
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

// called when OfflineNotifier receives GuildMembersChunk
func checkOffline(s *discordgo.Session, event *discordgo.GuildMembersChunk) {
	guild, err := getGuild(s, event.GuildID)
	if err != nil {
		logMessage(s, "[CHECK OFFLINE] error getting guild |", err)
		return
	}

	// check status of bots in guild
	for _, bot := range guild.bots {
		botMember, err := s.GuildMember(guild.ID, bot.ID)
		if err != nil {
			logMessage(s, "[CHECK OFFLINE] error getting bot |", err)
		}
		currentStatus := findStatus(event.Presences, bot.ID)
		if currentStatus == bot.status {
			// if nothing changed, move on
			continue
		}
		if bot.status == "offline" || currentStatus == "offline" {
			addToQueue("ss", [4]string{guild.ID, bot.ID, currentStatus, "true"})
			if bot.status != "unknown" {
				deltaTime := calculateDeltaTime(bot.timestamp)
				embed := &discordgo.MessageEmbed{Timestamp: time.Now().UTC().Format(time.RFC3339)}
				if bot.status == "offline" {
					// online
					embed.Title = botMember.User.Username + " is back online"
					embed.Description = "```TOTAL DOWNTIME\n" + deltaTime + "```"
					embed.Color = onlineColor
				} else {
					// offline
					embed.Title = botMember.User.Username + " is now offline"
					embed.Description = "```TOTAL UPTIME\n" + deltaTime + "```"
					embed.Color = offlineColor
				}
				go sendEmbed(s, guild.CID, embed)
			}
		} else {
			addToQueue("ss", [4]string{guild.ID, bot.ID, currentStatus, "false"})
		}
	}
}

// called when OfflineNotifier connects to discord
func connect(s *discordgo.Session, event *discordgo.Connect) {
	connected = true
	s.UpdateWatchStatus(1, "soon, please wait")
	log.Println("[CONNECTED]")
}

// called when OfflineNotifier gets disconnected from discord
func disconnect(s *discordgo.Session, event *discordgo.Disconnect) {
	connected = false
	log.Println("[DISCONNECTED]")
}

// called when discord responds with the ready event
func ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("[READY]")
	if !startedFunctions {
		log.Println("[GOLANG] starting coroutines...")
		go requestBots(s)
		go queueHandler(s)
		startedFunctions = true
	}
}

// called when connection resumes
func resumed(s *discordgo.Session, event *discordgo.Resumed) {
	connected = true
	log.Println("[RESUMED]")
}

// ----- BOT COMMANDS

// receives slash command interactions and runs the respective command
func commandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.GuildLocale != nil {
		switch i.ApplicationCommandData().Name {
		case "assign":
			assign(s, i)
		case "invite":
			invite(s, i)
		case "list":
			list(s, i)
		case "stats":
			stats(s, i)
		case "stop":
			stop(s, i)
		case "support":
			support(s, i)
		}
	} else {
		switch i.ApplicationCommandData().Name {
		case "invite":
			invite(s, i)
		case "stats":
			stats(s, i)
		case "support":
			support(s, i)
		default:
			embed := []*discordgo.MessageEmbed{
				{
					Title:       "Command failed",
					Description: "You're not in a server!",
					Color:       failColor,
				},
			}
			data := &discordgo.InteractionResponseData{Embeds: embed}
			response := &discordgo.InteractionResponse{Type: 4, Data: data}
			go s.InteractionRespond(i.Interaction, response)

		}
	}
}

// assign - sets the channel that OfflineNotifier will use
func assign(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var embed []*discordgo.MessageEmbed
	hasPermission, _ := memberHasPermission(s, i.GuildID, i.Member.User.ID, discordgo.PermissionManageChannels)
	if hasPermission {
		addToQueue("ac", [4]string{i.GuildID, i.ChannelID})
		embed = []*discordgo.MessageEmbed{
			{
				Title: "Assign channel request successful",
				Color: successColor,
			},
		}
	} else {
		embed = []*discordgo.MessageEmbed{
			{
				Title:       "Assign channel request failed",
				Description: "User does not have manage channel permission",
				Color:       failColor,
			},
		}
	}
	data := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: data}
	go s.InteractionRespond(i.Interaction, response)
}

// invite - sends an invite link for the bot
func invite(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := []*discordgo.MessageEmbed{
		{
			Title: "Want OfflineNotifier in your server? Use this link!",
			URL:   inviteLink,
			Color: defaultColor,
		},
	}
	data := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: data}
	go s.InteractionRespond(i.Interaction, response)
}

// list - lists the bots being watched in this server
func list(s *discordgo.Session, i *discordgo.InteractionCreate) {
	discordGuild, _ := s.Guild(i.GuildID)
	guild, err := getGuild(s, discordGuild.ID)
	if err != nil {
		embed := []*discordgo.MessageEmbed{
			{
				Title: "Bots aren't being watched in " + discordGuild.Name,
				Color: failColor,
			},
		}
		data := &discordgo.InteractionResponseData{Embeds: embed}
		response := &discordgo.InteractionResponse{Type: 4, Data: data}
		go s.InteractionRespond(i.Interaction, response)
		return
	}

	// make embed
	embed := []*discordgo.MessageEmbed{
		{
			Title:     "Bots being watched in " + discordGuild.Name,
			Color:     defaultColor,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}
	for _, bot := range guild.bots {
		if len(embed[0].Fields) < 25 {
			botDisc, err := s.GuildMember(discordGuild.ID, bot.ID)
			if err != nil {
				logMessage(s, "[LIST] error getting bot |", err)
				return
			}
			deltaTime := calculateDeltaTime(bot.timestamp)
			deltaName := "UPTIME"
			if bot.status == "offline" || bot.status == "unknown" {
				deltaName = "DOWNTIME"
			}
			newField := &discordgo.MessageEmbedField{Name: botDisc.User.Username, Value: "```\nLAST STATUS\n" + bot.status + "\n-----------\n" + deltaName + "\n" + deltaTime + "```", Inline: true}
			embed[0].Fields = append(embed[0].Fields, newField)
		}
	}
	if len(embed[0].Fields) == 25 {
		embed[0].Fields = embed[0].Fields[:24]
		field := &discordgo.MessageEmbedField{Name: "CAN ONLY LIST FIRST 24 BOTS", Value: "```Sorry for the inconvenience```"}
		embed[0].Fields = append(embed[0].Fields, field)
	}
	data := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: data}
	go s.InteractionRespond(i.Interaction, response)
}

// stats - shows stats about OfflineNotifier
func stats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// calculate totals
	totalServers := int64(len(s.State.Guilds))
	guilds := getGuilds(s)
	totalActive := int64(len(guilds))
	totalBots := int64(0)
	for _, guild := range guilds {
		for range guild.bots {
			totalBots += 1
		}
	}

	// calculate uptime
	uptime := calculateDeltaTime(startTime)

	// make embed
	embed := []*discordgo.MessageEmbed{
		{
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
		},
	}
	data := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: data}
	go s.InteractionRespond(i.Interaction, response)
}

// stop - stops watching a server
func stop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var embed []*discordgo.MessageEmbed
	hasPermission, _ := memberHasPermission(s, i.GuildID, i.Member.User.ID, discordgo.PermissionManageChannels)
	if hasPermission {
		addToQueue("rg", [4]string{i.GuildID})
		embed = []*discordgo.MessageEmbed{
			{
				Title: "Stop request successful",
				Color: successColor,
			},
		}
	} else {
		embed = []*discordgo.MessageEmbed{
			{
				Title:       "Stop request failed",
				Description: "User does not have manage channel permission",
				Color:       failColor,
			},
		}
	}
	data := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: data}
	go s.InteractionRespond(i.Interaction, response)
}

// support - sends a server invite for nooby's bot sanctuary
func support(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := []*discordgo.MessageEmbed{
		{
			Title: "Need help with OfflineNotifier? Join this server!",
			URL:   "https://discord.gg/YDRKdkh",
			Color: defaultColor,
		},
	}
	data := &discordgo.InteractionResponseData{Embeds: embed}
	response := &discordgo.InteractionResponse{Type: 4, Data: data}
	go s.InteractionRespond(i.Interaction, response)
}

// ----- LIST FUNCTIONS

// looks through a discord presence list for a matching ID, returns the status
func findStatus(presenceList []*discordgo.Presence, userID string) string {
	for _, p := range presenceList {
		if p.User.ID == userID {
			return string(p.Status)
		}
	}
	return "offline"
}

// looks through all guilds to find a matching guild ID, returns that guild object
func getGuild(s *discordgo.Session, GID string) (guild Guild, err error) {
	content, err := ioutil.ReadFile("activeServersGO.json")
	if err != nil {
		logMessage(s, "[GET GUILD] error reading activeServersGO.json |", err)
	}

	var guildMap map[string]interface{}
	if err := json.Unmarshal(content, &guildMap); err != nil {
		panic(err)
	}

	for guildID, guildData := range guildMap {
		if guildID == GID {
			guild := Guild{
				ID:   guildID,
				bots: nil,
				CID:  "",
			}
			guildData := guildData.(map[string]interface{})
			for itemKey, itemValue := range guildData {
				if itemKey == "bots" {
					botMap := itemValue.(map[string]interface{})
					for botID, botData := range botMap {
						botData := botData.(map[string]interface{})
						timestamp := int64(botData["timestamp"].(float64))
						botStatus := botData["status"].(string)
						bot := Bot{botID, botStatus, timestamp}
						guild.bots = append(guild.bots, bot)
					}
				} else {
					guild.CID = itemValue.(string)
				}
			}
			return guild, nil
		}
	}
	emptyGuild := Guild{
		ID:   "",
		bots: nil,
		CID:  "",
	}
	return emptyGuild, errors.New("guild not found")
}

// looks through a bot ID list to find the matching ID, returns the index
func indexBID(list []Bot, ID string) (i int, err error) {
	for i = range list {
		if list[i].ID == ID {
			return i, nil
		}
	}
	return 0, errors.New("not in list")
}

// looks through a guild ID list to find the matching ID, returns the index
func indexGID(list []Guild, ID string) (i int, err error) {
	for i = range list {
		if list[i].ID == ID {
			return i, nil
		}
	}
	return 0, errors.New("not in list")
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

// reads from a json file and turns data into a guild list
func getGuilds(s *discordgo.Session) (guildList []Guild) {
	content, err := ioutil.ReadFile("activeServersGO.json")
	if err != nil {
		logMessage(s, "[GET GUILDS] error reading activeServersGO.json |", err)
	}

	var guildMap map[string]interface{}
	if err := json.Unmarshal(content, &guildMap); err != nil {
		panic(err)
	}

	for guildID, guildData := range guildMap {
		guild := Guild{
			ID:   guildID,
			bots: nil,
			CID:  "",
		}
		guildData := guildData.(map[string]interface{})
		for itemKey, itemValue := range guildData {
			if itemKey == "bots" {
				botMap := itemValue.(map[string]interface{})
				for botID, botData := range botMap {
					botData := botData.(map[string]interface{})
					timestamp := int64(botData["timestamp"].(float64))
					botStatus := botData["status"].(string)
					bot := Bot{botID, botStatus, timestamp}
					guild.bots = append(guild.bots, bot)
				}
			} else {
				guild.CID = itemValue.(string)
			}
		}
		guildList = append(guildList, guild)
	}
	return
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

// checks if a member has a certain permission in a guild, returns a bool
func memberHasPermission(s *discordgo.Session, guildID string, userID string, permission int) (bool, error) {
	member, err := s.State.Member(guildID, userID)
	if err != nil {
		if member, err = s.GuildMember(guildID, userID); err != nil {
			return false, err
		}
	}

	// Iterate through the role IDs stored in member.Roles
	// to check permissions
	for _, roleID := range member.Roles {
		role, err := s.State.Role(guildID, roleID)
		if err != nil {
			return false, err
		}
		if role.Permissions&int64(permission) != 0 {
			return true, nil
		}
	}

	return false, nil
}

// packs a guild list into a json structure
func packGuilds(guildList []Guild) string {
	// to whomever sees this function: i know it's very ugly and terrible but i wanna
	// defend myself for a second here by saying that the way golang does json is very
	// yucky and i dont want any part of it so i did this instead thanks for understanding
	jsonString := "{"
	for x, guild := range guildList {
		if x != 0 {
			jsonString += ", "
		}
		jsonString += "\"" + guild.ID + "\": {\"bots\": {"
		for i, bot := range guild.bots {
			if i != 0 {
				jsonString += ", "
			}
			jsonString += "\"" + bot.ID + "\": {\"status\": \"" + bot.status + "\", \"timestamp\": " + strconv.FormatInt(bot.timestamp, 10) + "}"
		}
		jsonString += "}, \"channel\": \"" + guild.CID + "\"}"
	}
	jsonString += "}"
	return jsonString
}

// sends an embed
func sendEmbed(s *discordgo.Session, CID string, embed *discordgo.MessageEmbed) {
	_, err := s.ChannelMessageSendEmbed(CID, embed)
	if err != nil {
		logMessage(s, "[SEND EMBED] embed failed to send |", err)
	}
}

// ----- LOOP FUNCTIONS

// loops forever; reads from action queue and does subsequent actions
func queueHandler(s *discordgo.Session) {
	for {
		time.Sleep(10 * time.Millisecond)
		if len(actionQueue) > 0 {
			guildList := getGuilds(s)
			for len(actionQueue) > 0 {
				request := actionQueue[0]
				switch request.action {
				// ASSIGN CHANNEL - [GID, CID]
				case "ac":
					GID := request.data[0]
					CID := request.data[1]

					index, err := indexGID(guildList, GID)
					if err != nil {
						guild := Guild{ID: GID, bots: nil, CID: CID}
						guildList = append(guildList, guild)
					} else {
						guildList[index].CID = CID
					}

				// SET STATUS - [GID, BID, status, changeTimestamp]
				case "ss":
					GID := request.data[0]
					BID := request.data[1]
					status := request.data[2]

					indexG, err := indexGID(guildList, GID)
					if err != nil {
						logMessage(s, "[SET STATUS] error indexing GID |", err)
						continue
					}
					indexB, err := indexBID(guildList[indexG].bots, BID)
					if err != nil {
						logMessage(s, "[SET STATUS] error indexing BID |", err)
						continue
					}
					guildList[indexG].bots[indexB].status = status
					if strings.ToLower(request.data[3]) == "true" {
						guildList[indexG].bots[indexB].timestamp = time.Now().Unix()
					}

				// ADD BOT - [GID, BID]
				case "ab":
					GID := request.data[0]
					BID := request.data[1]

					index, err := indexGID(guildList, GID)
					if err != nil {
						logMessage(s, "[ADD BOT] error indexing GID |", err)
					} else {
						bot := Bot{BID, "unknown", time.Now().Unix()}
						guildList[index].bots = append(guildList[index].bots, bot)
					}

				// REMOVE BOT - [GID, BID]
				case "rb":
					GID := request.data[0]
					BID := request.data[1]

					indexG, err := indexGID(guildList, GID)
					if err != nil {
						logMessage(s, "[REMOVE BOT] error indexing GID |", err)
					} else {
						indexB, err := indexBID(guildList[indexG].bots, BID)
						if err != nil {
							logMessage(s, "[REMOVE BOT] error indexing BID |", err)
						} else {
							guildList[indexG].bots = append(guildList[indexG].bots[:indexB], guildList[indexG].bots[indexB+1:]...)
						}
					}

				// REMOVE GUILD - [GID]
				case "rg":
					GID := request.data[0]

					indexG, err := indexGID(guildList, GID)
					if err != nil {
						logMessage(s, "[REMOVE GUILD] error indexing GID |", err)
					} else {
						guildList = append(guildList[:indexG], guildList[indexG+1:]...)
					}
				}

				// POP ACTION FROM QUEUE
				actionQueue = actionQueue[1:]
			}
			guildJson := []byte(packGuilds(guildList))
			ioutil.WriteFile("activeServersGO.json", guildJson, 0755)
		}
	}
}

// loops forever; goes through active guild list and checks for new bots,
// requests presence list of bots, and culls removed bots.
func requestBots(s *discordgo.Session) {
	for {
		time.Sleep(1 * time.Second)
		guildData := getGuilds(s)

		// update presence
		totalBots := int64(0)
		for _, guild := range guildData {
			for range guild.bots {
				totalBots += 1
			}
		}
		statusText := strconv.FormatInt(totalBots, 10) + " bots"
		s.UpdateWatchStatus(0, statusText)

		// range through guilds
		for _, guild := range guildData {
			// check if OfflineNotifier is still in guild
			_, err := s.Guild(guild.ID)
			if err != nil {
				logMessage(s, "[REQUEST BOTS] error getting guild |", err)
				if !connected {
					continue
				}
				addToQueue("rg", [4]string{guild.ID})
				continue
			}

			// check if OfflineNotifier is still in channel
			_, err = s.Channel(guild.CID)
			if err != nil {
				logMessage(s, "[REQUEST BOTS] error setting message channel |", err)
				if !connected {
					continue
				}
				addToQueue("rg", [4]string{guild.ID})
				continue
			}

			// get member list
			memberList, err := s.GuildMembers(guild.ID, "", 1000)
			if err != nil {
				logMessage(s, "[REQUEST BOTS] error getting guild members |", err)
				continue
			}

			var bots []Bot
			var requestList []string
			copy(bots, guild.bots)
			// add bots to request list
			for _, member := range memberList {
				if member.User.Bot && member.User.ID != s.State.User.ID {
					_, err := indexBID(guild.bots, member.User.ID)
					if err != nil {
						// bot is not in data yet, add them
						addToQueue("ab", [4]string{guild.ID, member.User.ID})
						continue
					}
					// pop element from list
					if len(bots) != 0 {
						remainIndex, _ := indexBID(bots, member.User.ID)
						bots[len(bots)-1], bots[remainIndex] = bots[remainIndex], bots[len(bots)-1]
						bots = bots[:len(bots)-1]
					}
					// add to request list
					requestList = append(requestList, member.User.ID)
				}
			}
			// cull remaining bots
			for _, cullBot := range bots {
				addToQueue("rb", [4]string{guild.ID, cullBot.ID})
			}
			// request member list
			err = s.RequestGuildMembersList(guild.ID, requestList, 0, "", true)
			if err != nil && err != discordgo.ErrWSNotFound {
				logMessage(s, "[REQUEST BOTS] error requesting bots list |", err)
			}
		}
	}
}

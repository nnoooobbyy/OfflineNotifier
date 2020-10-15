package main

import (
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"strconv"
)

var (
	actionQueue []Request
	startTime = time.Now().Unix()
	startedFunctions = false
	connected = false
	inviteLink = ""
	defaultColor = 0x7289da
	successColor = 0x76e06e
	failColor = 0xe06c6c
	onlineColor = 0x43b581
	offlineColor = 0x727c8a
)

// ----- STRUCTS
type Request struct {
	action string
	data [4]string
}

type Bot struct {
	ID string
	status string
	timestamp int64
}

type Guild struct {
	ID string
	bots []Bot
	CID string
}

// -----  MAIN
func main() {
	// LOADING ENV
	err := godotenv.Load()
	if err != nil {
		log.Fatal("[GODOTENV] error loading .env file |", err)
		os.Exit(1)
	}
	TOKEN := os.Getenv("DISCORD_TOKEN")
	inviteLink = os.Getenv("INVITE_LINK")

	// CREATING BOT INSTANCE
	discord, err := discordgo.New("Bot " + TOKEN)
	if err != nil {
		fmt.Println("[DISCORDGO] error creating discord session |", err)
		os.Exit(1)
	}

	// register callbacks
	discord.AddHandler(ready)
	discord.AddHandler(connect)
	discord.AddHandler(disconnect)
	discord.AddHandler(resumed)
	discord.AddHandler(messageHandler)
	
	// intents
	discord.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAll)

	// Open the websocket and begin listening.
	err = discord.Open()
	if err != nil {
		fmt.Println("[DISCORDGO] error opening discord session |", err)
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("[RUNNING] Offline Notifier is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	discord.Close()
}

// ----- BOT EVENTS
// called when discord responds with the ready event
func ready(s *discordgo.Session, event *discordgo.Ready) {
	fmt.Println("[READY] discord responded ready")
	if !startedFunctions {
		fmt.Println("[GO] starting coroutines...")
		go checkOffline(s)
		go queueHandler()
		startedFunctions = true
	}
}

// called when offline notifier gets disconnected from discord
func disconnect(s *discordgo.Session, event *discordgo.Disconnect){
	connected = false
	fmt.Println("[DISCONNECTED] disconnected from discord")
}

// called when offline notifier connects to discord
func connect(s *discordgo.Session, event *discordgo.Connect) {
	connected = true
	s.UpdateStatus(0,"soon, please wait")
	fmt.Println("[CONNECTED] connected to discord")
}

// called when offline notifier connects to discord
func resumed(s *discordgo.Session, event *discordgo.Resumed) {
	connected = true
	fmt.Println("[RESUMED] discord connection resumed")
}

// ----- BOT COMMANDS
// message handler
func messageHandler(s *discordgo.Session, message *discordgo.MessageCreate) {
	// checks
	if !message.Author.Bot {
		channel, err := s.State.Channel(message.ChannelID)
		if err != nil {
			return
		}

		// command formatting
		command := ""
		if len(message.Content) < 7 {
			command = strings.ToLower(message.Content+"       ")
		} else {
			command = strings.ToLower(message.Content[:7])
		}

		if channel.Type != discordgo.ChannelTypeDM {
			// funcs that can only be in servers
			hasPermission, _ := MemberHasPermission(s, message.GuildID, message.Author.ID, discordgo.PermissionManageChannels)
			if hasPermission {
				// funcs if user has manage channels permission
				if command[:7] == "$assign" { go assign(s, message);return }
				if command[:5] == "$stop" { go stop(s, message);return }
			}
			if command[:5] == "$list" { go list(s, message);return }
		}
		// funcs that can be in any channel
		if command[:7] == "$invite" { go invite(s, message);return }
		if command[:6] == "$stats" { go stats(s, message);return }
		if command[:5] == "$help" { go help(s, message);return }
	}
}

// $assign - Sets the channel that OfflineNotifier will use
func assign(s *discordgo.Session, message *discordgo.MessageCreate) {
	addToQueue("ac", [4]string{message.GuildID, message.ChannelID})
	embed := &discordgo.MessageEmbed{}
	embed.Title = "Assign channel request successful"
	embed.Color = successColor
	go sendEmbed(s, message.ChannelID, embed)

}

// $stop - OfflineNotifier will stop watching bots in this server
func stop(s *discordgo.Session, message *discordgo.MessageCreate) {
	addToQueue("rg", [4]string{message.GuildID})
	embed := &discordgo.MessageEmbed{}
	embed.Title = "Stop request successful"
	embed.Color = successColor
	go sendEmbed(s, message.ChannelID, embed)
}

// $list - Lists the bots being watched in this server
func list(s *discordgo.Session, message *discordgo.MessageCreate) {
	// find guild
	guild, _ := s.State.Guild(message.GuildID)
	guildList := getGuilds()
	index, err := indexGID(guildList, guild.ID)
	if err != nil {
		embed := &discordgo.MessageEmbed{}
		embed.Title = "Bots aren't being watched in " + guild.Name
		embed.Color = failColor
		go sendEmbed(s, message.ChannelID, embed)
		return
	}

	// make embed
	embed := &discordgo.MessageEmbed{Timestamp: time.Now().UTC().Format(time.RFC3339)}
	embed.Title = "Bots being watched in " + guild.Name
	embed.Color = defaultColor
	for _, bot := range guildList[index].bots {
		if len(embed.Fields) < 25 {
			botDisc, _ := s.State.Member(guild.ID, bot.ID)
			deltaTime := calculateDeltaTime(bot.timestamp)
			deltaName := "UPTIME"
			if bot.status == "offline" || bot.status == "unknown" {
				deltaName = "DOWNTIME"
			}
			newField := &discordgo.MessageEmbedField{Name: botDisc.User.Username, Value: "```\nLAST STATUS\n" + bot.status + "\n-----------\n" + deltaName + "\n" + deltaTime + "```", Inline: true}
			embed.Fields = append(embed.Fields, newField)
		}
	}
	if len(embed.Fields) == 25 {
		embed.Fields = embed.Fields[:24]
		field := &discordgo.MessageEmbedField{Name: "CAN ONLY LIST FIRST 24 BOTS", Value: "```Sorry for the inconvenience```"}
		embed.Fields = append(embed.Fields, field)
	}
	go sendEmbed(s, message.ChannelID, embed)
}

// $invite - DMs the user an invite link for the bot
func invite(s *discordgo.Session, message *discordgo.MessageCreate) {
	embed := &discordgo.MessageEmbed{}
	embed.Title = "Want OfflineNotifier in your server? Use this link!"
	embed.URL = inviteLink
	embed.Color = defaultColor
	authorDM, _ := s.UserChannelCreate(message.Author.ID)
	go sendEmbed(s, authorDM.ID, embed)
}

// $stats - shows stats about OfflineNotifier
func stats(s *discordgo.Session, message *discordgo.MessageCreate) {
	// calculate totals
	totalServers := int64(len(s.State.Guilds))
	guilds := getGuilds()
	totalActive := int64(len(guilds))
	totalBots := int64(0)
	for _, guild := range guilds {
		for _, _ = range guild.bots {
			totalBots += 1
		}
	}

	// calculate uptime
	uptime := calculateDeltaTime(startTime)

	// make embed
	embed := &discordgo.MessageEmbed{Timestamp: time.Now().UTC().Format(time.RFC3339)}
	embed.Title = "OfflineNotifier stats"
	embed.Color = defaultColor
	embed.Fields = []*discordgo.MessageEmbedField{
		&discordgo.MessageEmbedField{Name:   "Total servers", Value:  "```"+strconv.FormatInt(totalServers, 10)+"```", Inline: true},
		&discordgo.MessageEmbedField{Name:   "Active servers", Value:  "```"+strconv.FormatInt(totalActive, 10)+"```", Inline: true},
		&discordgo.MessageEmbedField{Name:   "Bots watching", Value:  "```"+strconv.FormatInt(totalBots, 10)+"```", Inline: true},
		&discordgo.MessageEmbedField{Name:   "Uptime", Value:  "```"+uptime+"```", Inline: true},
	}
	go sendEmbed(s, message.ChannelID, embed)
}

// $help - lists commands
func help(s *discordgo.Session, message *discordgo.MessageCreate) {
	embed := &discordgo.MessageEmbed{}
	embed.Title = "Command list"
	embed.Fields = []*discordgo.MessageEmbedField{
		&discordgo.MessageEmbedField{Name:   "$assign", Value:  "```Sets the channel that OfflineNotifier will use (MUST HAVE MANAGE CHANNEL PERMS TO USE)```", Inline: true},
		&discordgo.MessageEmbedField{Name:   "$stop", Value:  "```OfflineNotifier will stop watching bots in this server (MUST HAVE MANAGE CHANNEL PERMS TO USE)```", Inline: true},
		&discordgo.MessageEmbedField{Name:   "$list", Value:  "```Lists the bots being watched in this server (MUST BE IN AN ACTIVE SERVER TO USE)```", Inline: true},
		&discordgo.MessageEmbedField{Name:   "$invite", Value:  "```DMs the user an invite link for the bot```", Inline: true},
		&discordgo.MessageEmbedField{Name:   "$stats", Value:  "```Shows stats about OfflineNotifier```", Inline: true},
		&discordgo.MessageEmbedField{Name:   "$help", Value:  "```Lists commands```", Inline: true},
	}
	embed.Color = defaultColor
	go sendEmbed(s, message.ChannelID, embed)
}

// ----- LIST FUNCTIONS
func indexGID (list []Guild, ID string) (i int, err error){
	for i = range list {
		if list[i].ID == ID {
			return i, nil
		}
	}
	return 0, errors.New("not in list")
}

func indexBID (list []Bot, ID string) (i int, err error){
	for i = range list {
		if list[i].ID == ID {
			return i, nil
		}
	}
	return 0, errors.New("not in list")
}

func removeBot (list []Bot, index int) []Bot {
	return append(list[:index], list[index + 1:]...)
}

func removeGuild (list []Guild, index int) []Guild {
	return append(list[:index], list[index + 1:]...)
}

func findStatus (presenceList []*discordgo.Presence, userID string) string {
	for _, p := range presenceList {
		if p.User.ID == userID {
			return string(p.Status)
		}
	}
	return "offline"
}

// ----- CALCULATION FUNCTIONS
// calculate time delta
func calculateDeltaTime(unixTime int64) string{
	// calculate
	currentTime := time.Now().Unix()
	diffUnix := currentTime - unixTime
	diffTime := time.Unix(diffUnix, 0).UTC()

	// format
	day := strconv.FormatInt(int64((diffTime.YearDay() - 1) * (diffTime.Year() - 1969)), 10)
	hour := strconv.FormatInt(int64(diffTime.Hour()), 10)
	minute := strconv.FormatInt(int64(diffTime.Minute()), 10)
	second := strconv.FormatInt(int64(diffTime.Second()), 10)
	return day+"D "+hour+"H "+minute+"M "+second+"S"
}

// ----- FRAMEWORK FUNCTIONS
func sendMessage(s *discordgo.Session, CID string, message string){
	_, err := s.ChannelMessageSend(CID, message)
	if err != nil{
		fmt.Println("[SEND MESSAGE] message failed to send |", err)
	}
}

func sendEmbed(s *discordgo.Session, CID string, embed *discordgo.MessageEmbed){
	_, err := s.ChannelMessageSendEmbed(CID, embed)
	if err != nil{
		fmt.Println("[SEND EMBED] embed failed to send |", err)
	}
}

func MemberHasPermission(s *discordgo.Session, guildID string, userID string, permission int) (bool, error) {
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
		if role.Permissions&permission != 0 {
			return true, nil
		}
	}

	return false, nil
}

func addToQueue(action string, data [4]string){
	request := Request{action, data}
	actionQueue = append(actionQueue, request)
}

func getGuilds() (guildList []Guild){
	content, err := ioutil.ReadFile("activeServersGO.json")
	if err != nil {
		fmt.Println("[GET GUILDS] error reading activeServersGO.json |", err)
	}

	var guildMap map[string]interface{}
	if err := json.Unmarshal(content, &guildMap); err != nil {
		panic(err)
	}

	for guildID, guildData := range guildMap {
		guild := Guild{
			ID: guildID,
			bots:    nil,
			CID: 	 "",
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

func packGuilds(guildList []Guild) string {
	// to whomever sees this function: i know it's very ugly and terrible but i wanna
	// defend myself for a second here by saying that the way golang does json is very
	// yucky and i dont want any part of it so i did this instead thanks for understanding
	jsonString := "{"
	for x, guild := range guildList {
		if x != 0 {jsonString += ", "}
		jsonString += "\""+guild.ID+"\": {\"bots\": {"
		for i, bot := range guild.bots {
			if i != 0 {
				jsonString += ", "
			}
			jsonString += "\""+bot.ID+"\": {\"status\": \""+bot.status+"\", \"timestamp\": "+strconv.FormatInt(bot.timestamp, 10)+"}"
		}
		jsonString += "}, \"channel\": \""+guild.CID+"\"}"
	}
	jsonString += "}"
	return jsonString
}

// ----- LOOP FUNCTIONS
func queueHandler(){
	for true {
		time.Sleep(10 * time.Millisecond)
		if len(actionQueue) > 0 {
			guildList := getGuilds()
			for len(actionQueue) > 0 {
				request := actionQueue[0]
				switch request.action {
				// ASSIGN CHANNEL - [GID, CID]
				case "ac":
					GID := request.data[0]
					CID := request.data[1]

					index, err := indexGID(guildList, GID)
					if err != nil {
						fmt.Println("[ASSIGN CHANNEL] error indexing ID |", err)
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
						fmt.Println("[SET STATUS] error indexing ID |", err)
						continue
					}
					indexB, err := indexBID(guildList[indexG].bots, BID)
					if err != nil {
						fmt.Println("[SET STATUS] error indexing ID |", err)
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
						fmt.Println("[ADD BOT] error indexing ID |", err)
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
						fmt.Println("[REMOVE BOT] error indexing ID |", err)
					} else {
						indexB, err := indexBID(guildList[indexG].bots, BID)
						if err != nil {
							fmt.Println("[REMOVE BOT] error indexing ID |", err)
						} else {
							guildList[indexG].bots = removeBot(guildList[indexG].bots, indexB)
						}
					}

				// REMOVE GUILD - [GID]
				case "rg":
					GID := request.data[0]

					indexG, err := indexGID(guildList, GID)
					if err != nil {
						fmt.Println("[REMOVE GUILD] error indexing ID |", err)
					} else {
						guildList = removeGuild(guildList, indexG)
					}
				}

				// POP ACTION FROM QUEUE
				actionQueue = actionQueue[1:len(actionQueue)]
			}
			guildJson := []byte(packGuilds(guildList))
			ioutil.WriteFile("activeServersGO.json", guildJson, 0755)
		}
	}
}

func checkOffline(s *discordgo.Session){
	for true {
		time.Sleep(1 * time.Second)
		guildData := getGuilds()

		// update presence
		totalBots := int64(0)
		for _, guild := range guildData {
			for _, _ = range guild.bots {
				totalBots += 1
			}
		}

		statusText := "with "+strconv.FormatInt(totalBots, 10)+" bots"
		s.UpdateStatus(0,statusText)

		for _, guild := range guildData{

			discGuild, err := s.State.Guild(guild.ID)
			if err != nil{
				fmt.Println("[CHECK OFFLINE] error setting currentGuild |", err)
				if !connected {
					continue
				}
				addToQueue("rg", [4]string{guild.ID})
				continue
			}

			// check if offline notifier is still in channel
			_, err = s.Channel(guild.CID)
			if err != nil{
				fmt.Println("[CHECK OFFLINE] error setting messageChannel |", err)
				if !connected {
					continue
				}
				addToQueue("rg", [4]string{guild.ID})
				continue
			}

			// get member list
			memberList, err := s.GuildMembers(guild.ID, "", 1000)
			if err != nil{
				fmt.Println("[CHECK OFFLINE] error getting guild members |", err)
				continue
			}

			var botsRemaining []Bot
			copy(botsRemaining, guild.bots)
			presenceList := discGuild.Presences

			for _, member := range memberList {
				if member.User.Bot && member.User.ID != s.State.User.ID {
					index, err := indexBID(guild.bots, member.User.ID)
					if err != nil{
						// bot is not in data yet, add them
						addToQueue("ab", [4]string{guild.ID, member.User.ID})
						continue
					}

					// pop element from list
					if len(botsRemaining) != 0{
						remainIndex, _ := indexBID(botsRemaining, member.User.ID)
						botsRemaining[len(botsRemaining)-1], botsRemaining[remainIndex] = botsRemaining[remainIndex], botsRemaining[len(botsRemaining)-1]
						botsRemaining = botsRemaining[:len(botsRemaining)-1]
					}

					currentStatus := findStatus(presenceList, member.User.ID)
					if currentStatus == guild.bots[index].status {
						// if nothing changed, move on
						continue
					}
					if guild.bots[index].status == "offline" || currentStatus == "offline" {
						addToQueue("ss", [4]string{guild.ID, guild.bots[index].ID, currentStatus, "true"})
						if guild.bots[index].status != "unknown"{
							deltaTime := calculateDeltaTime(guild.bots[index].timestamp)
							embed := &discordgo.MessageEmbed{Timestamp: time.Now().UTC().Format(time.RFC3339)}
							if guild.bots[index].status == "offline" {
								// online
								embed.Title = member.User.Username +" is back online"
								embed.Description = "```TOTAL DOWNTIME\n"+deltaTime+"```"
								embed.Color = onlineColor
							} else {
								// offline
								embed.Title = member.User.Username +" is now offline"
								embed.Description = "```TOTAL UPTIME\n"+deltaTime+"```"
								embed.Color = offlineColor
							}
							go sendEmbed(s, guild.CID, embed)
						}
					} else {
						addToQueue("ss", [4]string{guild.ID, guild.bots[index].ID, currentStatus, "false"})
					}
				}
			}
			// cull remaining bots
			for _, cullBot := range botsRemaining {
				addToQueue("rb", [4]string{guild.ID, cullBot.ID})
			}
		}
	}
}
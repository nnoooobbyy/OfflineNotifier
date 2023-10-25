# Discord bot that can tell you whenever a bot in a server goes offline or online
OfflineNotifier makes it easy to keep track of when a bot goes down. It's useful for ensuring upkeep of you or others' bots.

**Join [nooby's bot sanctuary](https://discord.gg/YDRKdkh) for support.**

### Example
![screenshot example](https://i.ibb.co/6sG9ZvV/Screenshot-2023-10-19-at-11-44-54.png)

### Commands
- /invite - Sends an invite link for the bot
- /list server - List bots in the current server
- /list subscriptions - List bots you're subscribed to
- /notify subscribe - [MUST ALLOW DMs] Subscribes to a bot
- /notify unsubscribe - Unsubscribes from a bot
- /privacy - Sends OfflineNotifier's privacy policy
- /stats - shows stats about OfflineNotifier
- /support - Need help with OfflineNotifier? Join this server!
- /watch set - Set the channel OfflineNotifier will send messages in & starts watching a server
- /watch stop - Stops watching a server

## Dependencies
[DiscordGo](github.com/bwmarrin/discordgo)
[GoDotEnv](https://github.com/joho/godotenv)
[errors](github.com/pkg/errors)

## Build

This assumes you already have a working Go environment setup and that
dependencies are correctly installed on your system.

From within the OfflineNotifier folder, run the below command to compile the
code.

```sh
go build
```

## Requirements

OfflineNotifier requires the following files/folders within the OfflineNotifier folder.

- data.json file containing `{}`
- OfflineNotifier.env file containing
```
DISCORD_TOKEN=(your token here)
OWNER_ID=(your discord user ID here)
INVITE_LINK=(invite link for your bot here)
```
- empty logs folder

To do this, open a Terminal window and run:

1. `cp data.json.example data.json`
2. `cp OfflineNotifier.env.example OfflineNotifier.env` 

## Usage

The below example shows how to start the bot from the OfflineNotifier folder.

```sh
./OfflineNotifier
```

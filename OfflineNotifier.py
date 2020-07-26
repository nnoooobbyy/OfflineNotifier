import os
import sys
import json
import asyncio
import regex as re
import inspect

from colorama import init, Fore
from datetime import datetime
from dotenv import load_dotenv
from discord import Activity
from discord import ActivityType
from discord import embeds
from discord import Status
from discord import errors
from discord.ext import commands

init()

# offlinebot :D
colors = {'n': Fore.LIGHTWHITE_EX, 's': Fore.LIGHTGREEN_EX, 'f': Fore.LIGHTRED_EX}
successColor = embeds.Colour.from_rgb(118,224,110)
failColor = embeds.Colour.from_rgb(224,108,108)
onlineColor = embeds.Colour.from_rgb(67,181,129)
offlineColor = embeds.Colour.from_rgb(114,124,138)
actionQueue = []
waitTime = 2 # minutes
liveTime = datetime.now()

# ----- DEFINITIONS
# PRINT DEFS
# failure print
def fprint(message):
    caller = inspect.stack()[1][3]
    print(colors['f'] + f"{datetime.now().strftime('%H:%M:%S')} | {caller} | {message}" + colors['n'])

# success print
def sprint(message):
    caller = inspect.stack()[1][3]
    print(colors['s'] + f"{datetime.now().strftime('%H:%M:%S')} | {caller} | {message}" + colors['n'])

# neutral print
def nprint(message):
    caller = inspect.stack()[1][3]
    print(colors['n'] + f"{datetime.now().strftime('%H:%M:%S')} | {caller} | {message}")

# ----- ASYNC DEFS
# handles message sending
async def sendMessage(location, message):
    try:
        nprint("sending message")
        if isinstance(message, embeds.Embed):
            await location.send(embed=message)
        else:
            await location.send(f"{message}")
        sprint("message sent successfully")
    except errors.Forbidden:
        fprint("message failed to send")

# handles all requests in queue every interval
async def queueHandler():
    while True:
        if actionQueue:
            # ACTIVE SERVERS JSON READ
            with open('activeServers.json') as readFile:
                serverJson = json.load(readFile)
            while actionQueue:
                data = actionQueue[0]
                action = data[0]
                
                # ASSIGN CHANNEL - [GID, CID]
                if action == 'ac':
                    nprint("ASSIGN CHANNEL")
                    GID = str(data[1][0])
                    CID = data[1][1]
                    try:
                        if CID != serverJson[GID]['channel']:
                            # CHANGE ASSIGNED CHANNEL
                            serverJson[GID]['channel'] = CID
                            sprint("assign channel successful")
                            response = embeds.Embed(title="ASSIGNMENT SUCCESSFUL", description=f"```OfflineNotifier now assigned to #{bot.get_channel(CID)}```", colour=successColor)
                        else:
                            # SAME ASSIGNED CHANNEL
                            fprint("assign channel failed | already assigned")
                            response = embeds.Embed(title="ASSIGNMENT FAILED", description=f"```OfflineNotifier already assigned to #{bot.get_channel(CID)}```", colour=failColor)
                    except KeyError:
                        # NEW GUILD
                        serverJson[GID] = {'bots': {}, 'channel': CID}
                        sprint("assign channel successful | new guild added")
                        response = embeds.Embed(title="ASSIGNMENT SUCCESSFUL", description=f"```OfflineNotifier assigned to #{bot.get_channel(CID)}```", colour=successColor)
                    await sendMessage(bot.get_channel(CID), response)

                # SET STATUS - [GID, BID, status]
                elif action == 'ss':
                    nprint("SET STATUS")
                    GID = str(data[1][0])
                    BID = str(data[1][1])
                    status = data[1][2]
                    oldStatus = serverJson[GID]['bots'][BID]
                    serverJson[GID]['bots'][BID] = status
                    sprint("set status successful")

                # ADD BOT - [GID, BID]
                elif action == 'ab':
                    nprint("ADD BOT")
                    GID = str(data[1][0])
                    BID = str(data[1][1])
                    serverJson[GID]['bots'][BID] = "unknown"
                    sprint("add bot successful")

                # REMOVE BOT - [GID, BID]
                elif action == 'rb':
                    nprint("REMOVE BOT")
                    GID = str(data[1][0])
                    BID = str(data[1][1])
                    try:
                        del serverJson[GID]['bots'][BID]
                        sprint("remove bot successful")
                    except:
                        fprint("remove bot failed | not in list")

                # REMOVE GUILD - [GID]
                elif action == 'rg':
                    nprint("REMOVE GUILD")
                    GID = str(data[1][0])
                    try:
                        del serverJson[GID]
                        sprint("remove guild successful")
                    except:
                        fprint("remove guild failed | not in dict")

                actionQueue.pop(0)

            # ACTIVE SERVERS JSON WRITE
            with open('activeServers.json', 'w') as writeFile:
                json.dump(serverJson, writeFile)

        await asyncio.sleep(1)

# adds actions to the action queue
async def addToQueue(action, data):
    valid = True
    if action == 'ss':
        checkData = data[:2]
    else: 
        checkData = data
    # checks the data to make sure it's correct
    for ID in checkData:
        ID = str(ID)
        if re.match('^[0-9]+$', ID):
            pass
        else:
            valid = False
    if valid:
        actionQueue.append([action, data])
        sprint(f"'{action}' added to queue")
    else:
        fprint(f"'{action}' not valid")
        fprint(f"{data}")
    return valid

# Checks if each server bot is offline every interval
async def checkOffline():
    while True:
        with open('activeServers.json') as readFile:
            data = json.load(readFile)

        # CHANGE PRESENCE
        totalBots = 0
        for GID in data:
            for BID in data[GID]['bots']:
                totalBots += 1
        activity = Activity(type=ActivityType.watching, name=f"{totalBots} bots")
        await bot.change_presence(status=Status.idle, activity=activity)

        # loops through each active server
        for GID in data:
            currentGuild = bot.get_guild(int(GID))

            # checks to see if offlinenotifier is still in guild
            if not currentGuild:
                fprint("no longer in guild, removing from dict")
                await addToQueue('rg', [GID])
                continue

            # checks to see if offlinenotifier is still in channel
            messageChannel = currentGuild.get_channel(data[GID]['channel'])
            if not messageChannel:
                fprint("no longer in channel, removing guild from dict")
                await addToQueue('rg', [GID])
                continue
            
            # culling bots that aren't in the server anymore
            for BID in data[GID]['bots']:
                if not currentGuild.get_member(int(BID)):
                    fprint("bot no longer in guild, removing from list")
                    await addToQueue('rb', [GID, BID])

            # loop through each user for bots
            for member in currentGuild.members:
                if member.bot and member.id != bot.user.id:
                    BID = str(member.id)
                    try:
                        status = str(member.status)
                        # if nothing changed, move on
                        if status == data[GID]['bots'][BID]:
                            continue
                        await addToQueue('ss', [GID, BID, status])
                        if status == "offline":
                            # BOT NOW OFFLINE
                            response = embeds.Embed(title=f"{member} is now offline", colour=offlineColor)
                            response.timestamp = datetime.utcnow()
                            await sendMessage(messageChannel, response)
                        elif data[GID]['bots'][BID] == "offline":
                            # BOT BACK ONLINE
                            response = embeds.Embed(title=f"{member} is back online", colour=onlineColor)
                            response.timestamp = datetime.utcnow()
                            await sendMessage(messageChannel, response)
                    except KeyError:
                        # ADD NEW BOT TO LIST
                        await addToQueue('ab', [GID, BID])
            
        for i in range(waitTime):
            if waitTime - i == 1:
                activity = Activity(type=ActivityType.watching, name="in less than a minute")
            else:
                activity = Activity(type=ActivityType.watching, name=f"in {waitTime - i} minutes")
            await bot.change_presence(activity=activity)
            await asyncio.sleep(60)

# env variables
load_dotenv()
TOKEN = os.getenv('DISCORD_TOKEN')

bot = commands.AutoShardedBot(command_prefix='$')

# ----- BOT EVENTS
# triggered when bot is ready
@bot.event
async def on_ready():
    liveTime = datetime.now()
    sprint(f'{bot.user.name} ready at {liveTime.strftime("%Y-%m-%d %H:%M:%S")}')
    # starting async tasks
    await asyncio.gather(
        checkOffline(),
        queueHandler(),
    )

# triggered when bot connects
@bot.event
async def on_connect():
    liveTime = datetime.now()
    sprint(f'{bot.user.name} connected to Discord at {liveTime.strftime("%Y-%m-%d %H:%M:%S")}')
    activity = Activity(type=ActivityType.watching, name="soon, please wait")
    await bot.change_presence(status=Status.idle, activity=activity)

# triggered when bot resumes
@bot.event
async def on_resumed():
    liveTime = datetime.now()
    sprint(f'{bot.user.name} resumed at {liveTime.strftime("%Y-%m-%d %H:%M:%S")}')

# triggered when shard is ready
@bot.event
async def on_shard_ready(shard):
    sprint(f'shard {shard} ready at {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}')

# triggered when bot is disconnected
@bot.event
async def on_disconnect():
    fprint(f'{bot.user.name} disconnected from Discord at {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}')

# #triggered when an exception is raised
@bot.event
async def on_error(event, *args, **kwargs):
    with open('err.log', 'a') as f:
        fprint("!!! WARNING !!! exception raised, check err.log for more details")
        f.write(f'\n-------------------------\nException raised at {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}\n{sys.exc_info()}')

# triggered when an error occurs in a command
@bot.event
async def on_command_error(ctx, error):
    if isinstance(error, commands.errors.CommandNotFound):
        return
    elif isinstance(error, commands.errors.NoPrivateMessage):
        response = embeds.Embed(title="COMMAND FAILED", description="```You can only say this command in a server```", colour=failColor)
        await sendMessage(ctx, response)
        return
    raise error

# ----- BOT COMMANDS
# $assign - Sets the channel that OfflineNotifier will use
@bot.command(name='assign', help='Sets the channel that OfflineNotifier will use')
@commands.guild_only()
async def assign(ctx):
    nprint("user requested to assign a channel")
    # check if the user can manage channels
    if not ctx.author.guild_permissions.manage_channels:
        fprint("user cannot manage channels")
        response = embeds.Embed(title="ASSIGNMENT FAILED", description="```Only someone who can manage channels can use this command```", colour=failColor)
        await sendMessage(ctx, response)
        return
    # obtain guild ID
    try:
        sourceGID = ctx.guild.id
    except AttributeError:
        sourceGID = "No guild ID"
    # obtain channel ID
    try:
        sourceCID = ctx.channel.id
    except AttributeError:
        sourceGID = "No channel ID"
    assignResult = await addToQueue('ac', [sourceGID, sourceCID])
    if assignResult:
        response = embeds.Embed(title="REQUEST SUCCESSFUL", description="```Request to assign channel successful```", colour=successColor)
    else:
        response = embeds.Embed(title="REQUEST FAILED", description="```Server/channel couldn't be found```", colour=failColor)
    await sendMessage(ctx, response)

# $stop - OfflineNotifier will stop watching bots in this server
@bot.command(name='stop', help='OfflineNotifier will stop watching bots in this server')
@commands.guild_only()
async def stop(ctx):
    nprint("user requested to stop watching bots")
    # check if the user can manage channels
    if not ctx.author.guild_permissions.manage_channels:
        fprint("user cannot manage channels")
        response = embeds.Embed(title="STOP FAILED", description="```Only someone who can manage channels can use this command```", colour=failColor)
        await sendMessage(ctx, response)
        return
    # obtain guild ID
    try:
        sourceGID = ctx.guild.id
    except AttributeError:
        sourceGID = "No guild ID"
    assignResult = await addToQueue('rg', [sourceGID])
    if assignResult:
        response = embeds.Embed(title="REQUEST SUCCESSFUL", description="```Request to stop watching successful```", colour=successColor)
    else:
        response = embeds.Embed(title="REQUEST FAILED", description="```Unknown error```", colour=failColor)
    await sendMessage(ctx, response)

# $list - Lists the bots being watched in this server
@bot.command(name='list', help='Lists the bots being watched in this server')
@commands.guild_only()
async def list(ctx):
    nprint("user requested to display list")
    GID = str(ctx.guild.id)
    listColor = embeds.Colour.from_rgb(114, 137, 218)
    serverName = bot.get_guild(int(GID))
    response = embeds.Embed(title=f"Bots being watched in {serverName}", colour=listColor)
    response.timestamp = datetime.utcnow()
    with open('activeServers.json') as readFile:
        serverJson = json.load(readFile)
    try:
        for BID in serverJson[GID]['bots']:
            botName = bot.get_user(int(BID))
            status = serverJson[GID]['bots'][BID]
            response.add_field(name=f"{botName}", value=f"```{status}```")
    except KeyError:
        response = embeds.Embed(title="LIST FAILED", description="```Bots are not being tracked in this server```", colour=failColor)
    await sendMessage(ctx, response)

# $invite - DMs the user an invite link for the bot
@bot.command(name='invite', help='DMs the user an invite link for the bot')
async def invite(ctx):
    nprint("user requested invite link")
    # variables
    inviteLink = "https://discord.com/api/oauth2/authorize?client_id=722352429865369663&permissions=75776&scope=bot"
    inviteColor = embeds.Colour.from_rgb(114, 137, 218)

    # creating and sending invite
    inviteEmbed = embeds.Embed(title="Want OfflineNotifier in your server? Use this link!", url=inviteLink, colour=inviteColor)
    user = bot.get_user(int(ctx.message.author.id))
    DM = await user.create_dm()
    await sendMessage(DM, inviteEmbed)

# $stats - shows stats about OfflineNotifier
@bot.command(name='stats', help='Shows stats about OfflineNotifier')
async def stats(ctx):
    nprint("user requested to display stats")
    statsColor = embeds.Colour.from_rgb(114, 137, 218)

    with open('activeServers.json') as readFile:
        serverJson = json.load(readFile)

    # CALCULATE TOTALS
    totalServers = len(bot.guilds)
    totalActive = len(serverJson)
    totalBots = 0
    for GID in serverJson:
        for BID in serverJson[GID]['bots']:
            totalBots += 1

    # CALCULATE UPTIME
    currentTime = datetime.now()
    uptime = (datetime.min + (currentTime - liveTime)).time()
    uptimeDay = int(currentTime.strftime('%d')) - int(liveTime.strftime('%d'))

    # MAKE EMBED
    statsEmbed = embeds.Embed(title="OfflineNotifier stats", colour=statsColor)
    statsEmbed.timestamp = datetime.utcnow()
    statsEmbed.add_field(name="Total servers", value=f"```{totalServers}```")
    statsEmbed.add_field(name="Active servers", value=f"```{totalActive}```")
    statsEmbed.add_field(name="Bots watching", value=f"```{totalBots}```")
    statsEmbed.add_field(name="Uptime", value=f"```{uptimeDay} d {uptime.strftime('%H h %M m %S s')}```")
    await sendMessage(ctx, statsEmbed)
 
# starting the bot
bot.run(TOKEN)
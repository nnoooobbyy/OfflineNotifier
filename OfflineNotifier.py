import os
import sys
import json
import asyncio
import regex as re
import inspect
import time

from colorama import init, Fore
from datetime import datetime
from dotenv import load_dotenv
from discord import Activity
from discord import ActivityType
from discord import embeds
from discord import Intents
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

# variables
botVersion = "PYTHON 3.1"
actionQueue = []
connected = False
waitTime = 5 # second(s)
startTime = time.time()

# ----- DEFINITIONS
# PRINT DEFS
# failure print
def fprint(message):
    caller = inspect.stack()[1][3]
    print(colors['f'] + f"{time.strftime('%H:%M:%S')} | {caller} | {message}" + colors['n'])

# success print
def sprint(message):
    caller = inspect.stack()[1][3]
    print(colors['s'] + f"{time.strftime('%H:%M:%S')} | {caller} | {message}" + colors['n'])

# neutral print
def nprint(message):
    caller = inspect.stack()[1][3]
    print(colors['n'] + f"{time.strftime('%H:%M:%S')} | {caller} | {message}")

# ----- ASYNC DEFS
# calculate time difference
async def calculateDeltaTime(pastTime):
    currentTime = time.time()
    diff = currentTime - pastTime
    return time.gmtime(diff)

# handles message sending
async def sendMessage(location, message):
    try:
        if isinstance(message, embeds.Embed):
            await location.send(embed=message)
        else:
            await location.send(f"{message}")
    except errors.Forbidden:
        fprint("message failed to send | forbidden")

# handles all requests in queue every interval
async def queueHandler():
    while True:
        if actionQueue:
            # ACTIVE SERVERS JSON READ
            with open('activeServersPY.json') as readFile:
                serverJson = json.load(readFile)
            while actionQueue:
                data = actionQueue[0]
                action = data[0]
                
                # ASSIGN CHANNEL - [GID, CID]
                if action == 'ac':
                    GID = str(data[1][0])
                    CID = data[1][1]
                    try:
                        if CID != serverJson[GID]['channel']:
                            # CHANGE ASSIGNED CHANNEL
                            serverJson[GID]['channel'] = CID
                            response = embeds.Embed(title="ASSIGNMENT SUCCESSFUL", description=f"```OfflineNotifier now assigned to #{bot.get_channel(CID)}```", colour=successColor)
                        else:
                            # SAME ASSIGNED CHANNEL
                            fprint("assign channel failed | already assigned")
                            response = embeds.Embed(title="ASSIGNMENT FAILED", description=f"```OfflineNotifier already assigned to #{bot.get_channel(CID)}```", colour=failColor)
                    except KeyError:
                        # NEW GUILD
                        serverJson[GID] = {'bots': {}, 'channel': CID}
                        sprint("new guild added")
                        response = embeds.Embed(title="ASSIGNMENT SUCCESSFUL", description=f"```OfflineNotifier assigned to #{bot.get_channel(CID)}```", colour=successColor)
                    await sendMessage(bot.get_channel(CID), response)

                # SET STATUS - [GID, BID, status, changeTimestamp]
                elif action == 'ss':
                    GID = str(data[1][0])
                    BID = str(data[1][1])
                    status = data[1][2]
                    changeTimestamp = data[1][3]
                    serverJson[GID]['bots'][BID]['status'] = status
                    if changeTimestamp:
                        serverJson[GID]['bots'][BID]['timestamp'] = time.time()

                # ADD BOT - [GID, BID]
                elif action == 'ab':
                    GID = str(data[1][0])
                    BID = str(data[1][1])
                    serverJson[GID]['bots'][BID] = {'status': "unknown", 'timestamp': time.time()}

                # REMOVE BOT - [GID, BID]
                elif action == 'rb':
                    GID = str(data[1][0])
                    BID = str(data[1][1])
                    try:
                        del serverJson[GID]['bots'][BID]
                    except KeyError:
                        fprint("remove bot failed | not in dict")

                # REMOVE GUILD - [GID]
                elif action == 'rg':
                    GID = str(data[1][0])
                    try:
                        del serverJson[GID]
                    except KeyError:
                        fprint("remove guild failed | not in dict")

                # POP ACTION FROM QUEUE
                actionQueue.pop(0)

            # ACTIVE SERVERS JSON WRITE
            with open('activeServersPY.json', 'w') as writeFile:
                json.dump(serverJson, writeFile)

        await asyncio.sleep(0.1)

# adds actions to the action queue
async def addToQueue(action, data):
    valid = True
    if action == 'ss':
        checkData = data[:2]
    else: 
        checkData = data
    # checks to make sure the data is valid
    for ID in checkData:
        ID = str(ID)
        if re.match('^[0-9]+$', ID):
            pass
        else:
            valid = False
    if valid:
        actionQueue.append([action, data])
    else:
        fprint(f"'{action}' not added to queue: invalid\n{data}")
    return valid

# Checks if each server bot is offline every interval
async def checkOffline():
    while True:
        with open('activeServersPY.json') as readFile:
            data = json.load(readFile)

        # CHANGE PRESENCE
        totalBots = 0
        for GID in data:
            for BID in data[GID]['bots']:
                totalBots += 1
        activity = Activity(type=ActivityType.watching, name=f"{totalBots} bots")
        await bot.change_presence(activity=activity)

        # loops through each active server
        for GID in data:

            # checks to see if offlinenotifier is still in guild
            currentGuild = bot.get_guild(int(GID))
            if not currentGuild:
                if not connected: continue
                fprint("no longer in guild, removing from dict")
                await addToQueue('rg', [GID])
                continue

            # checks to see if offlinenotifier is still in channel
            messageChannel = currentGuild.get_channel(data[GID]['channel'])
            if not messageChannel:
                if not connected: continue
                fprint("no longer in channel, removing guild from dict")
                await addToQueue('rg', [GID])
                continue
            
            # culling bots that aren't in the server anymore
            for BID in data[GID]['bots']:
                if not currentGuild.get_member(int(BID)) and connected:
                    fprint("bot no longer in guild, removing from list")
                    await addToQueue('rb', [GID, BID])

            # loop through each user for bots
            for member in currentGuild.members:
                if member.bot and member.id != bot.user.id:
                    BID = str(member.id)
                    try:
                        status = str(member.status)
                        # if nothing changed, move on
                        if status == data[GID]['bots'][BID]['status']:
                            continue
                        if data[GID]['bots'][BID]['status'] == "offline" or status == "offline":
                            # BOT HAS GONE ONLINE/OFFLINE
                            await addToQueue('ss', [GID, BID, status, True])

                            # if the bot just got added, don't say anything
                            if data[GID]['bots'][BID]['status'] != "unknown":
                                # calculate uptime/downtime
                                deltaTime = await calculateDeltaTime(data[GID]['bots'][BID]['timestamp'])

                                # create response
                                response = embeds.Embed(title=f"{member} is {'now offline' if status == 'offline' else 'back online'}",description=f"```TOTAL {'UPTIME' if status == 'offline' else 'DOWNTIME'}\n{(deltaTime.tm_yday - 1) * (deltaTime.tm_year - 1969)}D {deltaTime.tm_hour}H {deltaTime.tm_min}M {deltaTime.tm_sec}S```", colour=offlineColor if status == 'offline' else onlineColor)
                                response.timestamp = datetime.utcnow()
                                await sendMessage(messageChannel, response)
                        else:
                            await addToQueue('ss', [GID, BID, status, False])
                    except KeyError:
                        # ADD NEW BOT TO LIST
                        await addToQueue('ab', [GID, BID])
            
        await asyncio.sleep(waitTime)

# env variables
load_dotenv()
TOKEN = os.getenv('DISCORD_TOKEN')
inviteLink = os.getenv('INVITE_LINK')

# intent
intents = Intents.all()

bot = commands.AutoShardedBot(command_prefix='$', intents=intents)

# ----- BOT EVENTS
# triggered when bot is ready
@bot.event
async def on_ready():
    sprint(f'{bot.user.name} ready at {time.strftime("%m/%d/%Y %H:%M:%S")}')
    # starting async tasks
    await asyncio.gather(
        checkOffline(),
        queueHandler(),
    )

# triggered when bot connects
@bot.event
async def on_connect():
    sprint(f'{bot.user.name} connected at {time.strftime("%m/%d/%Y %H:%M:%S")}')
    connected = True
    activity = Activity(type=ActivityType.watching, name="soon, please wait")
    await bot.change_presence(status=Status.idle, activity=activity)

# triggered when bot resumes
@bot.event
async def on_resumed():
    sprint(f'{bot.user.name} resumed at {time.strftime("%m/%d/%Y %H:%M:%S")}')
    connected = True

# triggered when shard is ready
@bot.event
async def on_shard_ready(shard):
    sprint(f'shard {shard} ready at {time.strftime("%m/%d/%Y %H:%M:%S")}')

# triggered when bot is disconnected
@bot.event
async def on_disconnect():
    fprint(f'{bot.user.name} disconnected at {time.strftime("%m/%d/%Y %H:%M:%S")}')
    connected = False

# #triggered when an exception is raised
@bot.event
async def on_error(event, *args, **kwargs):
    with open('err.log', 'a') as f:
        fprint("!!! WARNING !!! exception raised, check err.log for more details")
        f.write(f'\n-------------------------\nException raised at {time.strftime("%m/%d/%Y %H:%M:%S")}\n{sys.exc_info()}')

# triggered when an error occurs in a command
@bot.event
async def on_command_error(ctx, error):
    if isinstance(error, commands.errors.CommandNotFound):
        return
    elif isinstance(error, commands.errors.NoPrivateMessage):
        response = embeds.Embed(title="COMMAND FAILED", description="```You can only use this command in a server```", colour=failColor)
        await sendMessage(ctx, response)
        return
    raise error

# ----- BOT COMMANDS
# $assign - Sets the channel that OfflineNotifier will use
@bot.command(name='assign', help='Sets the channel that OfflineNotifier will use')
@commands.guild_only()
async def assign(ctx):
    # check if the user can manage channels
    if not ctx.author.guild_permissions.manage_channels:
        response = embeds.Embed(title="ASSIGNMENT FAILED", description="```Only someone who can manage channels can use this command```", colour=failColor)
        await sendMessage(ctx, response)
        return
    # obtain guild ID
    try:
        sourceGID = ctx.guild.id
    except AttributeError:
        sourceGID = "No GID"
    # obtain channel ID
    try:
        sourceCID = ctx.channel.id
    except AttributeError:
        sourceCID = "No CID"
    assignResult = await addToQueue('ac', [sourceGID, sourceCID])
    if assignResult:
        response = embeds.Embed(title="REQUEST SUCCESSFUL", description="```Assign channel requested```", colour=successColor)
    else:
        response = embeds.Embed(title="REQUEST FAILED", description="```Server/channel couldn't be found```", colour=failColor)
    await sendMessage(ctx, response)

# $stop - OfflineNotifier will stop watching bots in this server
@bot.command(name='stop', help='OfflineNotifier will stop watching bots in this server')
@commands.guild_only()
async def stop(ctx):
    # check if the user can manage channels
    if not ctx.author.guild_permissions.manage_channels:
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
        response = embeds.Embed(title="REQUEST SUCCESSFUL", description="```Stop watching requested```", colour=successColor)
    else:
        response = embeds.Embed(title="REQUEST FAILED", description="```Unknown error```", colour=failColor)
    await sendMessage(ctx, response)

# $list - Lists the bots being watched in this server
@bot.command(name='list', help='Lists the bots being watched in this server')
@commands.guild_only()
async def list(ctx):
    GID = str(ctx.guild.id)
    listColor = embeds.Colour.from_rgb(114, 137, 218)
    serverName = bot.get_guild(int(GID))
    response = embeds.Embed(title=f"Bots being watched in {serverName}", colour=listColor)
    response.timestamp = datetime.utcnow()
    with open('activeServersPY.json') as readFile:
        serverJson = json.load(readFile)
    try:
        for BID in serverJson[GID]['bots']:
            if len(response.fields) < 25:
                botName = bot.get_user(int(BID))
                deltaTime = await calculateDeltaTime(serverJson[GID]['bots'][BID]['timestamp'])

                status = serverJson[GID]['bots'][BID]['status']
                response.add_field(name=f"{botName}", value=f"```\nLAST STATUS\n{status}\n{'------' if status.lower() != 'offline' else '--------'}\n{'UPTIME' if status.lower() != 'offline' else 'DOWNTIME'}\n{(deltaTime.tm_yday - 1) * (deltaTime.tm_year - 1969)}D {deltaTime.tm_hour}H {deltaTime.tm_min}M {deltaTime.tm_sec}S```")
        if len(response.fields) == 25:
            response.remove_field(24)
            response.add_field(inline=False, name="CAN ONLY LIST FIRST 24 BOTS", value="```Sorry for the inconvenience```")
    except KeyError:
        response = embeds.Embed(title="LIST FAILED", description="```Bots are not being tracked in this server```", colour=failColor)
    await sendMessage(ctx, response)

# $invite - DMs the user an invite link for the bot
@bot.command(name='invite', help='DMs the user an invite link for the bot')
async def invite(ctx):
    inviteColor = embeds.Colour.from_rgb(114, 137, 218)

    # creating and sending invite
    inviteEmbed = embeds.Embed(title="Want OfflineNotifier in your server? Use this link!", url=inviteLink, colour=inviteColor)
    user = bot.get_user(int(ctx.message.author.id))
    DM = await user.create_dm()
    await sendMessage(DM, inviteEmbed)

# $stats - shows stats about OfflineNotifier
@bot.command(name='stats', help='Shows stats about OfflineNotifier')
async def stats(ctx):
    statsColor = embeds.Colour.from_rgb(114, 137, 218)

    with open('activeServersPY.json') as readFile:
        serverJson = json.load(readFile)

    # CALCULATE TOTALS
    totalServers = len(bot.guilds)
    totalActive = len(serverJson)
    totalBots = 0
    for GID in serverJson:
        for BID in serverJson[GID]['bots']:
            totalBots += 1

    # CALCULATE UPTIME
    uptime = await calculateDeltaTime(startTime)

    # MAKE EMBED
    statsEmbed = embeds.Embed(title="OfflineNotifier stats", colour=statsColor)
    statsEmbed.timestamp = datetime.utcnow()
    statsEmbed.add_field(name="Total servers", value=f"```{totalServers}```")
    statsEmbed.add_field(name="Active servers", value=f"```{totalActive}```")
    statsEmbed.add_field(name="Bots watching", value=f"```{totalBots}```")
    statsEmbed.add_field(name="Bot version", value=f"```{botVersion}```")
    statsEmbed.add_field(name="Uptime", value=f"```{(uptime.tm_yday - 1) * (uptime.tm_year - 1969)}D {uptime.tm_hour}H {uptime.tm_min}M {uptime.tm_sec}S```")
    await sendMessage(ctx, statsEmbed)
 
# starting the bot
bot.run(TOKEN)
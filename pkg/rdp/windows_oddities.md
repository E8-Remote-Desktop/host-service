# Windows Security

This document describes how E8 is able to display UAC and run at system logon.

# System Services & Session 0

First we need to get into a bit of history: System Services first started being a thing around the windows 9x days. Back then they would start as the default user account, aka the first administrator account who logged into the system. This means that system services could directly interact with the desktop and capture the desktop.

With Windows Vista+ system services changed. They are now isolated to "Session 0", Session 0 does not have any desktop and cannot interact with User Desktop Sessions at all (except audio for some reason??).

# UAC & The Secure Desktop

The inital stages of Windows logon (the login screen up until right after your password is entered) and the UAC prompts all run in something called the Secure Desktop. The Secure Desktop in essense is an entirely different session launched by the currently logged in User. It has some extra security on it as well, including not allowing regular interaction from the user-mode desktop to reach it.

# Attaching to User Sessions in Session 0

In session 0 we can do everything short of actually capturing the desktop and sending input. For example all processing, network packetization/sending/recieve, will be handled in Session 0.

## Video/Audio

Video/Audio on windows is primarly handled by DXGI right now to be replaced by the IDD driver **soon** (which will actually solve this whole problem with 0 fuss but whatever). What we need to do to capture Video and Audio is escape session 0, by starting a process as the user which can stream video to a UDP pipeline that our session 0 app can pickup.

This is done by polling the WinLogon/Desktop API to see if the desktop session has changed. If it has we 1) Close down our old video/audio session; 2) Grab the user token needed to start processes as that user in a desktop session 3) Start the video/audio streams as that user. This will work for both secure desktop, the login screen (which is also secure desktop), and switching users.

This is where the stream_helper comes in, when you run rdp.exe -stream TOKEN it starts a new stream as the new user. The system uses named pipes to communicate close and restart commands as well as any errors in the stream.

## Input

Input is a similar story, we do all the same steps as video however the helper is extremely minimal. It is just a wrapper around SendInput that gets restarted. Named pipes are used much more heavily here as all key processing happens in session 0 and then a named pipe will send all input to the input helper to actually act on the input.

# Elevation

This is an entirely different component not to be confused with the session switching that was discussed earlier. When elevating a process (Run as Administrator) that process is spawned as an "elevated process." When elevated windows are on the screen all interaction from the users underlying session is no longer allowed, this includes capture and input but for some reason does not include audio (bug??). However unlike the Session 0 escaping, Microsoft provides a direct method to circumvent this with the uiAccess permisson, which allow apps to interact with elevated windows in the same user session even if the associated programs were not launched with high-enough permissions to technically be able to interact with those windows

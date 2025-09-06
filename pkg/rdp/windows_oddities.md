# Windows Security

This document describes how E8 is able to display UAC and run at system logon.

# System Services & Session 0

First we need to get into a bit of history: System Services first started being a thing around the windows 9x days. Back then they would start as the default user account, aka the first administrator account who logged into the system. This means that system services could directly interact with the desktop and capture the desktop.

With Windows Vista+ system services changed. They are now isolated to "Session 0", Session 0 does not have any desktop and cannot interact with User Desktop Sessions at all (except audio for some reason??). Despite Session 0 services running with System permissions, they cannot easily interact with the desktop.

# UAC & The Secure Desktop

The inital stages of Windows logon (the login screen up until right after your password is entered) and the UAC prompts all run in something called the Secure Desktop. The Secure Desktop in essense is an entirely different session launched by the currently logged in User. It has some extra security on it as well, including not allowing regular interaction from the user-mode desktop to reach it.

# Attaching to User Sessions in Session 0

In session 0 we can do everything short of actually capturing the desktop and sending input. For example all processing, network packetization/sending/recieve, will be handled in Session 0.

## Video/Audio

Video/Audio on windows is primarly handled by DXGI right now to be replaced by the IDD driver **soon** (which will actually solve this whole problem with 0 fuss but whatever). What we need to do to capture Video and Audio is escape session 0, by starting a process as the user which can stream video to a UDP pipeline that our session 0 app can pickup.

This is done by registering an event listener for the EVENT_SYSTEM_DESKTOPSWITCH with the WinLogon/Desktop API to see if the desktop session has changed. If it has we 1) Close down our old video/audio session; 2) Grab the user token needed to start processes as that user in a desktop session 3) Start the video/audio streams as that user. This will work for both secure desktop, the login screen (which is also secure desktop), and switching users.

More specifically, it polls for the OpenInputDesktop, i.e the desktop recieving input. With UAC and WinLogon the problem becomes that they run as the system user but on the users desktop. Unforunately we can't start with the users token and say that it's the system user running it. We need to take the system token (OpenProcessToken) and add the attributes to tell it what desktop to use. First we need to give it the permissions to have a desktop (SeTcbPrivilege, SeAssignTokenPrivillege) you can do this by calling LookupPrivilegeValue to get what it currently is and then assing with AdjustTokenPrivileges. Then get the session UAC is on, (WTSGetActiveConsoleSessionID). Then we need to duplcicate the token (yes the session 0 token), (DuplicateTokenEx) then we can re-assign it to the users session, SetTokenInformation(TokenInformationClass: TokenSessionID(not the console session id), TokenInformation: &TokenSessionID, TokenLength: DWORD32) we can then use GetUserObjectInformation to get the name of the desktop "\\WinSta0\\fdjkfjdk". Then finally launch the helper app by setting STARTUPINFOEX's lpDesktop parameter to the name of the UAC desktop and then call CreateProcessAsUser.

Method for Regular User:

This is where the stream_helper comes in, when you run rdp.exe -stream TOKEN it starts a new stream as the new user. The system uses named pipes to communicate close and restart commands as well as any errors in the stream.

## Input

Input is a similar story, we do all the same steps as video however the helper is extremely minimal. It is just a wrapper around SendInput that gets restarted. Named pipes are used much more heavily here as all key processing happens in session 0 and then a named pipe will send all input to the input helper to actually act on the input.

# Elevation

This is an entirely different component not to be confused with the session switching that was discussed earlier. When elevating a process (Run as Administrator) that process is spawned as an "elevated process." When elevated windows are on the screen all interaction from the users underlying session is no longer allowed, this includes capture and input but for some reason does not include audio (bug??). However unlike the Session 0 escaping, Microsoft provides a direct method to circumvent this with the uiAccess permisson, which allow apps to interact with elevated windows in the same user session even if the associated programs were not launched with high-enough permissions to technically be able to interact with those windows

# Pseudocode

Step 1:
Check to see what mode RDP was launched in (config file, "service" "regular")

System Service (UAC/WinLogon) ("service")

1. RDPStreamConnector calls streamer.Start()
2. Streamer attempts to start in service mode with escalation, assumes session 0 start.
3. Streamer must determine Token/Desktop/Other parameters to launch StreamHelper
   a. Register Desktop Change Event Handler on the Restart() function (just calls Cancel() then Start())
   b. Determine if it is UAC/WinLogon or Regular 'Default' Session (here we assume it's Secure/other)
   c. Get Process Handle (GetCurrentProcess()) (returns processHandle)
   d. Get Process Token for Duplication (OpenProcessToken(processHandle, TOKEN_DUPLICATE | TOKEN_QUERY, &output_token)) (returns boolean of success or not, the token goes into output_token)
   e. Lookup luid of the SeTcbPrivllege and save to type
      ```LUID luidLUID luid; LookupPrivilegeValue(NULL, TEXT("SeTcbPrivilege"), &luid);  
      ```
   e. Enable the privilleges 
   ```
   TOKEN_PRIVILEGES tp;
   tp.PrivilegeCount = 1;
   tp.Privileges[0].Luid = luid;
   tp.Privileges[0].Attributes = SE_PRIVILEGE_ENABLED;
   AdjustTokenPrivileges(TokenHandle: output_token, DisableAllPrivileges: false, NewState: &tp, BufferLength: sizeof(TOKEN_PRIVILEGES), PreviousStateOutput: NULL, ReturnLengthOutput: NULL)
4. Streamer starts named pipe server in go func with channel, waits for client to connect

System Service (Regular User) ("service")

1. RDPStreamConnector calls streamer.Start()

Regular Execution ("regular")

1. RDPStreamConnector calls streamer.Start()
2. Streamer determines it should start in regular mode without escalation
3. Streamer starts named pipe server in go func with channel, waits for client to connect
4. Streamer launches StreamHelper via rdp.exe in stream mode (-stream parameter)
5. The second client connects send Start via channel
6. When Close is called on Streamer, send close via channel.

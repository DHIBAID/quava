## Definitions:
1. Client: Android device
2. Host/Daemon: Linux device
3. DE: Desktop Environment

## Broad Feature list:
- [x] Ping 
- [x] Ring
- [x] Unpair
- [x] Device status
- [x] Clipboard Sync
- [x] Notification Sync
- [x] Remote Input
- [x] Multimedia Controls
- [x] SSH
- [ ] Pre-saved command running
- [ ] SFTP/SMB
- [ ] Exit Node/VPN
- [ ] Screenshare/RDP
- [ ] SMS Interaction
- [ ] Password Manager(?)
- [ ] 2FA code sync(?)
- [ ] DE specific tools/extensions(?)


## QOL functionality:
- [x] Auto-connect to paired devices
- [ ] Mute host audio when an incoming call is detected
- [ ] Pause host audio when an active call is detected
- [ ] Customization of UI (maybe custom CSS)


## In depth features:
### Ping
- [x] Send a push notification to client from host
- [ ] Send a push notification to host from client

### Ring
- [x] Play the alarm sound on the client device when invoked from host

### Unpair
- [x] Unpair the both devices when request comes from either side
- [ ] Delete all data related to the other device when unpairing

### Device status
- [x] Show the battery percentage of the client device on the host
- [ ] Show the battery percentage of the host device on the client
- [x] Show the network status of the client device on the host
- [ ] Show the network status of the host device on the client
- [x] Show the storage status of the client device on the host
- [ ] Show the storage status of the host device on the client
- [x] Show bluetooth status of the client device on the host
- [ ] Show bluetooth status of the host device on the client
- [ ] Allow the user to set a threshold for battery percentage and notify when it goes below that threshold
- [ ] Allow user to modify Wifi/Bluetooth state of the client device from the host
- [ ] Allow user to modify Wifi/Bluetooth state of the host device from the client

### Clipboard Sync
- [ ] Sync the clipboard of the client device with the host device (manual push only)
- [x] Sync the clipboard of the host device with the client device
- [ ] Allow sending data other than text (images, files, etc.) from the host to the client device
- [ ] Allow sending data other than text (images, files, etc.) from the client to the host device

### Notification Sync
- [x] Sync the notifications of the client device with the host device
- [x] Sync the notifications of the host device with the client device
- [x] Allow user to pick notification sync direction (host to client, client to host, both, none)
- [x] Allow user to interact with the notifications (reply, dismiss, etc.) from the host device
- [ ] Allow user to interact with the notifications (reply, dismiss, etc.) from the client device
- [x] Allow user to block specific notifications from the client device
- [x] Allow user to disable grouping of notifications from the client device
- [x] Allow user to modify slient notifications behaviour for pushing to the device 
- [x] Allow user to block specific notification types from the client device
- [x] Allow user to use the app icon instead of notification icon for notifications from the client device

### Remote Input
- [x] Allow user to control the host device from the client device (mouse, keyboard)
- [x] Allow user to block remote control requests from the client device
- [ ] 
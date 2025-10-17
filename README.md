# 📧 Email Monitor

A powerful, cross-platform email notification monitor with system tray integration. Monitor multiple email accounts simultaneously with advanced filtering rules and folder management.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-%3E%3D1.19-blue)
![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)

## ✨ Features

- 🔔 **Real-time Desktop Notifications** - Native system notifications for new emails
- 📬 **Multiple Account Support** - Monitor unlimited email accounts simultaneously
- 🎯 **Advanced Filtering** - Keyword and sender-based filters (include/exclude)
- 📁 **Folder Management** - Monitor all folders, specific folders, or exclude folders
- 🔄 **Auto-refresh** - Configurable check intervals per account
- 🗂️ **Smart Tracking** - Never get duplicate notifications
- 🎨 **System Tray Integration** - Runs quietly in the background
- 📊 **Real-time Status** - See unread count and last check time
- 🔍 **Folder Discovery** - List and save all available folders
- 📝 **Comprehensive Logging** - Track all activities
- 💾 **OS-Standard Storage** - Files stored in proper OS directories

## 🖥️ Supported Platforms

- ✅ Linux (X11 and Wayland)
- ✅ macOS
- ✅ Windows

## 📋 Requirements

### General
- Go 1.19 or higher
- IMAP-enabled email account(s)

### Linux (Debian/Ubuntu)
```bash
sudo apt-get install libgtk-3-dev libayatana-appindicator3-dev
```

### Linux (Arch)
```bash
sudo pacman -S gtk3 libayatana-appindicator
```

### macOS & Windows
No additional dependencies required!

## 🚀 Installation

### Option 1: Build from Source

```bash
# Clone the repository
git clone https://git.mydustb.in/KunalGautam/email-notifier
cd email-monitor

# Install dependencies
go mod download

# Build
go build -o email-monitor

# Run
./email-monitor
```

### Option 2: Download Binary

Download the latest release from the [Releases](https://git.mydustb.in/KunalGautam/email-notifier) page.

## 📂 File Locations

Email Monitor follows OS-specific conventions for storing configuration and data files:

### Linux
```
~/.config/email-monitor/
├── config.json
├── email-monitor.log
├── folders_list.json
├── icon.png (optional)
└── notification_history/
    ├── user1_at_example.com.json
    └── user2_at_gmail.com.json
```

### macOS
```
~/Library/Application Support/email-monitor/
├── config.json
├── email-monitor.log
├── folders_list.json
├── icon.png (optional)
└── notification_history/
```

### Windows
```
%APPDATA%\email-monitor\
├── config.json
├── email-monitor.log
├── folders_list.json
├── icon.png (optional)
└── notification_history\
```

**Finding Your Files:**
- Right-click the tray icon → "Open App Directory"
- Or check the console output when starting the application

## ⚙️ Configuration

On first run, `config.json` will be created automatically in the appropriate OS directory. The application will show you the exact location.

### Editing Configuration

**Linux/macOS:**
```bash
# Edit config
nano ~/.config/email-monitor/config.json
# or on macOS
nano ~/Library/Application\ Support/email-monitor/config.json
```

**Windows:**
```cmd
notepad %APPDATA%\email-monitor\config.json
```

### Configuration Example

```json
{
  "accounts": [
    {
      "email": "work@company.com",
      "server": "imap.gmail.com",
      "port": 993,
      "username": "work@company.com",
      "password": "your-app-password",
      "protocol": "imap",
      "include_keyword": ["urgent", "invoice"],
      "exclude_keyword": ["promotion", "newsletter"],
      "include_email": ["boss@company.com"],
      "exclude_email": ["spam@example.com"],
      "check_interval": 120,
      "check_history": 1000,
      "enable_notification_sound": true,
      "folder_mode": "all",
      "include_folders": ["INBOX", "Work"],
      "exclude_folders": ["Spam", "Trash"]
    },
    {
      "email": "personal@gmail.com",
      "server": "imap.gmail.com",
      "port": 993,
      "username": "personal@gmail.com",
      "password": "your-app-password",
      "protocol": "imap",
      "include_keyword": [],
      "exclude_keyword": ["social", "promotion"],
      "include_email": [],
      "exclude_email": [],
      "check_interval": 60,
      "check_history": 500,
      "enable_notification_sound": false,
      "folder_mode": "exclude",
      "include_folders": [],
      "exclude_folders": ["Spam", "Trash", "Promotions"]
    }
  ]
}
```

### Configuration Options

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `email` | string | Email address for display | Required |
| `server` | string | IMAP server address | Required |
| `port` | int | IMAP port (usually 993) | Required |
| `username` | string | Login username | Required |
| `password` | string | Login password | Required |
| `protocol` | string | Protocol (currently only "imap") | `"imap"` |
| `check_interval` | int | Seconds between checks | `120` |
| `check_history` | int | Max notification history | `1000` |
| `enable_notification_sound` | bool | Play sound with notifications | `true` |
| `folder_mode` | string | Folder checking mode (see below) | `"all"` |
| `include_folders` | array | Folders to include (if mode is "include") | `[]` |
| `exclude_folders` | array | Folders to exclude (if mode is "exclude") | `[]` |
| `include_keyword` | array | Only notify if subject contains these | `[]` |
| `exclude_keyword` | array | Don't notify if subject contains these | `[]` |
| `include_email` | array | Only notify from these senders | `[]` |
| `exclude_email` | array | Don't notify from these senders | `[]` |

### Folder Modes

#### 1. Check All Folders (`"all"`)
```json
"folder_mode": "all"
```
Monitors all folders in the mailbox.

#### 2. Include Specific Folders (`"include"`)
```json
"folder_mode": "include",
"include_folders": ["INBOX", "Work", "Important"]
```
Only monitors the specified folders. Use this when you only want to monitor a few specific folders.

#### 3. Exclude Folders (`"exclude"`)
```json
"folder_mode": "exclude",
"exclude_folders": ["Spam", "Trash", "Drafts", "Sent"]
```
Monitors all folders except the specified ones. Use this when you want to monitor most folders but skip a few.

## 🔐 Email Provider Setup

### Gmail
1. Enable IMAP in Gmail settings
2. Create an [App Password](https://support.google.com/accounts/answer/185833)
3. Use settings:
   - Server: `imap.gmail.com`
   - Port: `993`
   - Password: Your app password (not your Gmail password)

### Outlook/Office 365
- Server: `outlook.office365.com`
- Port: `993`
- Username: Your full email address
- Password: Your account password

### Yahoo Mail
1. Enable IMAP in Yahoo Mail settings
2. Generate an app password
3. Use settings:
   - Server: `imap.mail.yahoo.com`
   - Port: `993`

### Other Providers
Check your email provider's IMAP settings documentation.

## 🎮 Usage

### Starting the Monitor

```bash
./email-monitor
```

On first run, you'll see:
```
📁 Email Monitor
Application directory: /home/user/.config/email-monitor

✅ Sample config created: /home/user/.config/email-monitor/config.json
Please edit it with your email settings and restart.
```

The application will minimize to your system tray.

### System Tray Menu

Right-click the tray icon to access:

#### Account Status
- **✅ Status: Running** - Current monitor status
- **📧 Account Info** - Shows unread count and last check time per account
  - Format: `📧 user@example.com - Unread: 5 (Last: 14:32:15)`

#### Actions
- **🔍 Check All Now** - Manually check all accounts immediately
- **🗑️ Clear All History** - Reset notification tracking for all accounts

#### Folder Management
- **📁 Folder Management**
  - **📋 List All Folders** - Retrieve and save complete folder list from all accounts
  - **👁️ View Saved Folder List** - Display previously saved folders

#### System
- **🔄 Restart Monitor** - Restart email monitoring for all accounts
- **⚙️ Reload Config** - Reload configuration file without restarting app
- **📂 Open App Directory** - Show the application directory path
- **📄 View File Locations** - Display all file paths
- **❌ Exit** - Close the application

### Command Line Options

```bash
# Run in foreground with console output
./email-monitor

# Run in background (Linux/macOS)
./email-monitor &

# Stop background process
pkill email-monitor
```

### Finding Your Files

The easiest way to find your configuration and logs:

1. **From the tray menu:**
   - Right-click tray icon → "Open App Directory"
   - Or: "View File Locations" to see all paths

2. **From terminal:**
```bash
# Linux
ls -la ~/.config/email-monitor/

# macOS
ls -la ~/Library/Application\ Support/email-monitor/

# Windows
dir %APPDATA%\email-monitor
```

## 🔧 Advanced Features

### Discovering Available Folders

Before setting up folder filters, you can discover all available folders:

1. Start the application
2. Right-click tray icon → "Folder Management" → "List All Folders"
3. Wait for the notification confirming the list is saved
4. View the list: "Folder Management" → "View Saved Folder List"

The folder list is saved to `folders_list.json` and includes all folders for all configured accounts.

### Filter Rules Priority

Filters are applied in this order:
1. **Exclude Email** - Blocks specific senders (highest priority)
2. **Exclude Keywords** - Blocks subjects with certain words
3. **Include Email** - Allows specific senders (if any include filters are set)
4. **Include Keywords** - Allows subjects with certain words (if any include filters are set)

**Important:** If you set any include filters (email or keyword), ONLY emails matching those filters will notify. This is useful for high-priority monitoring.

### Example Filter Scenarios

#### Monitor Only Important Work Emails
```json
"include_keyword": ["urgent", "important", "invoice", "action required"],
"exclude_keyword": ["out of office", "automated"],
"folder_mode": "include",
"include_folders": ["INBOX", "Work"]
```

#### Block All Marketing/Promotions
```json
"exclude_keyword": ["unsubscribe", "promotion", "advertisement", "newsletter"],
"exclude_folders": ["Promotions", "Social", "Updates"],
"folder_mode": "exclude"
```

#### VIP Senders Only
```json
"include_email": ["boss@company.com", "ceo@company.com", "client@important.com"],
"exclude_keyword": ["automated", "no-reply"],
"folder_mode": "all"
```

#### Personal Account - No Spam
```json
"exclude_email": ["noreply@", "marketing@"],
"exclude_keyword": ["unsubscribe", "opt-out"],
"exclude_folders": ["Spam", "Trash", "Promotions", "Social"],
"folder_mode": "exclude"
```

## 📊 Understanding Unread Count

The unread count shown in the tray menu represents:
- **Total unread emails** across all monitored folders
- Updated after each check cycle
- Respects your folder mode settings (all/include/exclude)

Example: If you're monitoring 3 folders and have 2, 3, and 1 unread emails respectively, the count will show 6.

## 🐛 Troubleshooting

### No Tray Icon on Linux (Wayland)

Wayland has limited system tray support. Solutions:

1. **Install GNOME Shell Extension** (GNOME):
   - [AppIndicator and KStatusNotifierItem Support](https://extensions.gnome.org/extension/615/appindicator-support/)
   - TopIcons Plus

2. **Use KDE Plasma** - Has native tray support on Wayland

3. **Switch to X11 session** temporarily

4. The application still works! Check logs and use "Check All Now" for manual checks.

### Configuration File Not Found

The config file should be created automatically. If it's missing:

```bash
# Check if directory exists
ls ~/.config/email-monitor/  # Linux
ls ~/Library/Application\ Support/email-monitor/  # macOS
dir %APPDATA%\email-monitor  # Windows

# If missing, run the app once to create it
./email-monitor
```

### Connection Errors

```
[user@example.com] Error: failed to connect: dial tcp: lookup imap.example.com: no such host
```

**Solutions:**
- Verify IMAP is enabled in your email account settings
- Check firewall isn't blocking port 993
- Ensure correct server address (check your provider's documentation)
- Verify credentials are correct
- For Gmail, ensure you're using an App Password, not your regular password
- Try connecting manually using an IMAP client to verify settings

### No Notifications Appearing

**Check System Notifications:**
```bash
# Test if notifications work
notify-send "Test" "This is a test notification"  # Linux
```

**Verify Settings:**
1. Check `enable_notification_sound` is `true` in config
2. Ensure notification filters aren't blocking all emails
3. Check system notification settings (Do Not Disturb mode?)
4. Review logs for errors: `tail -f ~/.config/email-monitor/email-monitor.log`

### Incorrect Unread Count

**Possible causes:**
- `folder_mode` excludes folders with unread emails
- IMAP server reporting incorrect counts
- Multiple devices marking emails as read

**Solutions:**
1. Use "List All Folders" to see all available folders
2. Verify your `folder_mode` and folder lists
3. Try "Check All Now" to force refresh
4. Check logs for any folder access errors

### Application Won't Start

**Check logs:**
```bash
tail -50 ~/.config/email-monitor/email-monitor.log  # Linux/macOS
type %APPDATA%\email-monitor\email-monitor.log  # Windows
```

**Common issues:**
- Missing dependencies (Linux): Install GTK and AppIndicator libraries
- Permission issues: Ensure the application has execute permissions
- Port already in use: Check if another instance is running

### High CPU Usage

**Optimization tips:**
- Increase `check_interval` (e.g., 300 seconds for 5-minute checks)
- Use `folder_mode: "include"` to monitor only essential folders
- Reduce number of monitored accounts
- Use exclude filters to skip large folders (Sent, All Mail)

## 🔄 Running at Startup

### Linux (systemd)

Create `~/.config/systemd/user/email-monitor.service`:

```ini
[Unit]
Description=Email Notification Monitor
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/email-monitor
Restart=on-failure
RestartSec=10
Environment=DISPLAY=:0

[Install]
WantedBy=default.target
```

**Note:** Update `ExecStart` with the full path to your binary:
```bash
# Find full path
which email-monitor
# or
readlink -f ./email-monitor
```

Enable and start:
```bash
systemctl --user daemon-reload
systemctl --user enable email-monitor.service
systemctl --user start email-monitor.service
systemctl --user status email-monitor.service
```

### macOS (launchd)

Create `~/Library/LaunchAgents/com.user.email-monitor.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.email-monitor</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/email-monitor</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/email-monitor.out</string>
    <key>StandardErrorPath</key>
    <string>/tmp/email-monitor.err</string>
</dict>
</plist>
```

Load:
```bash
launchctl load ~/Library/LaunchAgents/com.user.email-monitor.plist
launchctl start com.user.email-monitor
```

### Windows (Task Scheduler)

1. Press `Win + R`, type `taskschd.msc`, press Enter
2. Click "Create Basic Task"
3. Name: "Email Monitor"
4. Trigger: "When I log on"
5. Action: "Start a program"
6. Program: Browse to `email-monitor.exe`
7. Finish

**Alternative (Startup Folder):**
1. Press `Win + R`, type `shell:startup`, press Enter
2. Create a shortcut to `email-monitor.exe` in the opened folder

## 📝 Logging

Logs are automatically written to `email-monitor.log` in the application directory.

### Viewing Logs

**Linux/macOS:**
```bash
# View full log
cat ~/.config/email-monitor/email-monitor.log

# Follow log in real-time
tail -f ~/.config/email-monitor/email-monitor.log

# View last 50 lines
tail -n 50 ~/.config/email-monitor/email-monitor.log

# Search for errors
grep ERROR ~/.config/email-monitor/email-monitor.log
```

**Windows:**
```cmd
type %APPDATA%\email-monitor\email-monitor.log
```

### Log Entries Include:
- Application startup and configuration loading
- Connection attempts and results
- New email notifications sent
- Filter applications (what was blocked/allowed)
- Errors and warnings
- Account status updates
- Folder listing operations

### Log Levels:
- `INFO`: Normal operations
- `WARNING`: Non-critical issues
- `ERROR`: Problems that need attention

## 🧹 Uninstalling

### Complete Removal

**Linux:**
```bash
# Stop the service if running
systemctl --user stop email-monitor.service
systemctl --user disable email-monitor.service
rm ~/.config/systemd/user/email-monitor.service

# Remove application files
rm -rf ~/.config/email-monitor

# Remove binary
rm /usr/local/bin/email-monitor  # or wherever you installed it
```

**macOS:**
```bash
# Stop the service
launchctl unload ~/Library/LaunchAgents/com.user.email-monitor.plist
rm ~/Library/LaunchAgents/com.user.email-monitor.plist

# Remove application files
rm -rf ~/Library/Application\ Support/email-monitor

# Remove binary
rm /usr/local/bin/email-monitor
```

**Windows:**
1. Remove from Task Scheduler or Startup folder
2. Delete `%APPDATA%\email-monitor`
3. Delete `email-monitor.exe`

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [go-imap](https://github.com/emersion/go-imap) - IMAP client library
- [beeep](https://github.com/gen2brain/beeep) - Cross-platform notifications
- [systray](https://github.com/getlantern/systray) - System tray integration

## 📧 Support

- **Issues**: [Issues](https://git.mydustb.in/KunalGautam/email-notifier/issues)
- **WiKi**: [Wiki](https://git.mydustb.in/KunalGautam/email-notifier/wiki)

## 🗺️ Roadmap

- [ ] OAuth2 authentication support (Gmail, Outlook)
- [ ] Web interface for configuration
- [ ] GUI configuration editor
- [ ] Email actions from notifications (mark as read, archive)
- [ ] Custom notification templates
- [ ] Multiple notification profiles (work hours, weekend)
- [ ] Statistics and analytics dashboard
- [ ] Secure password storage (system keyring integration)
- [ ] POP3 protocol support
- [ ] Email preview in notifications
- [ ] Mobile companion app
- [ ] System Keyring, instead of plaintext config file

## ⚡ Performance Tips

- Set reasonable `check_interval` (60-300 seconds recommended)
- Use `folder_mode: "include"` to monitor only essential folders
- Limit `check_history` to what you actually need (default 1000 is good)
- Use exclude filters to skip spam/marketing folders
- Monitor only essential accounts during work hours
- Use exclude_folders for "All Mail", "Sent", "Drafts" (Gmail)

## 🔒 Security Notes

⚠️ **Important Security Considerations:**

- Passwords are stored in **plain text** in `config.json`
- The config file has standard file permissions (644 on Unix, normal on Windows)
- **Best practices:**
  - Use app-specific passwords when available (Gmail, Yahoo, etc.)
  - Restrict `config.json` permissions: `chmod 600 ~/.config/email-monitor/config.json`
  - Never commit `config.json` to version control
  - Add `config.json` to `.gitignore` if developing
  - Regularly rotate passwords
  - Don't share your config file
  - Consider encrypting your home directory (OS-level protection)

**Future Improvement:** System keyring integration is planned for secure password storage.

## 💡 Tips & Tricks

### Temporary Disable Monitoring
Instead of exiting, use "Clear All History" to reset and get notifications again later.

### Testing Filters
1. Set `check_interval` to 30 seconds for testing
2. Send test emails to yourself
3. Watch the logs: `tail -f ~/.config/email-monitor/email-monitor.log`
4. Adjust filters based on results
5. Increase `check_interval` back to 120-300 for production

### Multiple Profiles
You can run multiple instances with different configs:
```bash
# Copy the binary
cp email-monitor email-monitor-work
cp email-monitor email-monitor-personal

# Each will use its own config directory based on the binary name
# (requires minor code modification)
```

### Backup Your Settings
```bash
# Linux/macOS
tar -czf email-monitor-backup.tar.gz ~/.config/email-monitor

# Restore
tar -xzf email-monitor-backup.tar.gz -C ~/
```

---

**Made with ❤️ for productive email management**

**Version:** 0.0.2
**Last Updated:** 2025 Oct 17
**Maintainer:** Kunal Gautam

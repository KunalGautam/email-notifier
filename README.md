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
git clone https://github.com/yourusername/email-monitor.git
cd email-monitor

# Install dependencies
go mod download

# Build
go build -o email-monitor

# Run
./email-monitor
```

### Option 2: Download Binary

Download the latest release from the [Releases](https://github.com/yourusername/email-monitor/releases) page.

## ⚙️ Configuration

On first run, `config.json` will be created automatically. Edit it with your email settings:

```json
{
  "accounts": [
    {
      "email": "user@example.com",
      "server": "imap.gmail.com",
      "port": 993,
      "username": "user@example.com",
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
Only monitors the specified folders.

#### 3. Exclude Folders (`"exclude"`)
```json
"folder_mode": "exclude",
"exclude_folders": ["Spam", "Trash", "Drafts"]
```
Monitors all folders except the specified ones.

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

### Yahoo Mail
- Server: `imap.mail.yahoo.com`
- Port: `993`

### Other Providers
Check your email provider's IMAP settings documentation.

## 🎮 Usage

### Starting the Monitor

```bash
./email-monitor
```

The application will minimize to your system tray.

### System Tray Menu

Right-click the tray icon to access:

- **✅ Status: Running** - Current monitor status
- **📧 Account Info** - Shows unread count and last check time per account
- **🔍 Check All Now** - Manually check all accounts immediately
- **🗑️ Clear All History** - Reset notification tracking
- **📁 Folder Management**
  - **📋 List All Folders** - Retrieve and save folder list
  - **👁️ View Saved Folder List** - Display saved folders
- **🔄 Restart Monitor** - Restart email monitoring
- **⚙️ Reload Config** - Reload configuration without restarting
- **📄 View Logs Location** - Show file paths
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

## 📁 File Structure

```
.
├── email-monitor              # Binary executable
├── config.json                # Configuration file
├── email-monitor.log          # Application logs
├── folders_list.json          # Saved folder list
├── notification_history/      # Per-account notification history
│   ├── user1_at_example.com.json
│   └── user2_at_gmail.com.json
└── icon.png                   # Optional custom icon
```

## 🔧 Advanced Features

### Filter Rules Priority

Filters are applied in this order:
1. **Exclude Email** - Blocks specific senders
2. **Exclude Keywords** - Blocks subjects with certain words
3. **Include Email** - Allows specific senders (if set)
4. **Include Keywords** - Allows subjects with certain words (if set)

If include filters are set, ONLY matching emails will notify.

### Example Filter Scenarios

#### Only Important Emails
```json
"include_keyword": ["urgent", "important", "invoice"],
"exclude_keyword": ["newsletter", "promotion"]
```

#### Boss Emails Only
```json
"include_email": ["boss@company.com", "ceo@company.com"],
"exclude_keyword": ["automated"]
```

#### Block Spam
```json
"exclude_email": ["spam@example.com", "marketing@vendor.com"],
"exclude_keyword": ["unsubscribe", "promotion", "advertisement"]
```

## 🐛 Troubleshooting

### No Tray Icon on Linux (Wayland)

Wayland has limited system tray support. Solutions:

1. **Install GNOME Shell Extension** (GNOME):
   - AppIndicator and KStatusNotifierItem Support
   - TopIcons Plus

2. **Switch to X11 session** temporarily

3. The application still works! Use `Check All Now` for manual checks.

### Connection Errors

- Verify IMAP is enabled in your email settings
- Check firewall isn't blocking port 993
- Ensure correct server address and credentials
- For Gmail, use App Password, not regular password

### No Notifications

1. Check system notification settings
2. Verify `enable_notification_sound` in config
3. Check notification filters aren't blocking emails
4. Review logs: `cat email-monitor.log`

### Incorrect Unread Count

- Ensure `folder_mode` is set correctly
- Check if excluded folders contain unread emails
- Try "Check All Now" to force refresh

## 🔄 Running at Startup

### Linux (systemd)

Create `~/.config/systemd/user/email-monitor.service`:

```ini
[Unit]
Description=Email Notification Monitor
After=network.target

[Service]
Type=simple
WorkingDirectory=/path/to/email-monitor
ExecStart=/path/to/email-monitor/email-monitor
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
```

Enable and start:
```bash
systemctl --user enable email-monitor.service
systemctl --user start email-monitor.service
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
        <string>/path/to/email-monitor</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>/path/to/email-monitor</string>
</dict>
</plist>
```

Load:
```bash
launchctl load ~/Library/LaunchAgents/com.user.email-monitor.plist
```

### Windows

1. Press `Win + R`
2. Type `shell:startup`
3. Create a shortcut to `email-monitor.exe` in the opened folder

## 📊 Logging

Logs are written to `email-monitor.log`. View them:

```bash
# View full log
cat email-monitor.log

# Follow log in real-time
tail -f email-monitor.log

# View last 50 lines
tail -n 50 email-monitor.log
```

Log entries include:
- Connection attempts
- New email notifications
- Filter applications
- Errors and warnings
- Account status updates

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
- [godotenv](https://github.com/joho/godotenv) - Environment file parser

## 📧 Support

- **Issues**: [GitHub Issues](https://github.com/yourusername/email-monitor/issues)
- **Discussions**: [GitHub Discussions](https://github.com/yourusername/email-monitor/discussions)

## 🗺️ Roadmap

- [ ] OAuth2 authentication support
- [ ] Web interface for configuration
- [ ] Email reply from notification
- [ ] Custom notification templates
- [ ] Multiple notification profiles
- [ ] Statistics and analytics
- [ ] Mobile companion app

## ⚡ Performance Tips

- Set reasonable `check_interval` (60-300 seconds recommended)
- Use folder filters to reduce checking overhead
- Limit `check_history` to what you need
- Use exclude filters to skip spam/marketing folders
- Monitor only essential accounts

## 🔒 Security Notes

- Passwords are stored in plain text in `config.json`
- Use app-specific passwords when available
- Keep `config.json` permissions restricted: `chmod 600 config.json`
- Don't commit `config.json` to version control
- Regularly rotate passwords
- Consider using system keyring integration (future feature)

---

**Made with ❤️ for productive email management**
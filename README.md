<img src="https://r2cdn.perplexity.ai/pplx-full-logo-primary-dark%402x.png" style="height:64px;margin-right:32px"/>

# Write a readme.md file for following code

Based on the code analysis, I can now create a comprehensive README.md file for this email monitoring application.

# Email Monitor

A lightweight system tray application for monitoring multiple email accounts with support for **IMAP** and **POP3** protocols. Built with Go, it features desktop notifications, advanced filtering, and a web-based dashboard for easy management.[^1]

## Features

### Core Functionality

- **Multi-account support** - Monitor multiple email accounts simultaneously with independent configurations[^1]
- **Dual protocol support** - Works with both IMAP and POP3 email servers[^1]
- **System tray integration** - Runs quietly in the background with unread count display[^1]
- **Desktop notifications** - Instant alerts for new emails matching your filters[^1]
- **Web dashboard** - Browser-based interface for configuration and monitoring[^1]


### Security

- **Secure password storage** - Passwords are stored in your system's keyring (Keychain on macOS, Secret Service on Linux, Credential Manager on Windows)[^1]
- **Automatic migration** - Converts plaintext passwords from config to keyring on first run[^1]


### Filtering Options

- **Keyword filtering** - Include or exclude emails based on subject/body keywords[^1]
- **Sender filtering** - Include or exclude specific email addresses[^1]
- **Folder filtering** - Monitor all folders, specific folders, or exclude certain folders[^1]
- **Notification history** - Prevents duplicate notifications with configurable history limit[^1]


### Monitoring Features

- **Configurable check intervals** - Set custom polling intervals per account[^1]
- **Real-time status** - View unread count and last check time for each account[^1]
- **Manual checking** - Trigger immediate checks for all accounts[^1]
- **Connection testing** - Verify credentials and server settings before saving[^1]


## Installation

### Prerequisites

- Go 1.25.3 or higher[^1]
- System keyring support (most modern operating systems have this built-in)[^1]

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


### Build from Source

```bash
git clone https://git.mydustb.in/KunalGautam/email-notifier
cd email-monitor
go build -o email-monitor main.go
```


### Dependencies

The application uses the following Go packages:[^1]

- `github.com/emersion/go-imap` - IMAP client library
- `github.com/knadh/go-pop3` - POP3 client library
- `github.com/gen2brain/beeep` - Desktop notifications
- `github.com/getlantern/systray` - System tray integration
- `github.com/zalando/go-keyring` - Secure password storage

Install dependencies with:

```bash
go mod download
```


## Configuration

### First Run

On first launch, the application creates a sample configuration file at:[^1]

- **Linux:** `~/.config/email-monitor/config.json`
- **macOS:** `~/Library/Application Support/email-monitor/config.json`
- **Windows:** `%APPDATA%\email-monitor\config.json`


### Configuration File Structure

```json
{
  "accounts": [
    {
      "email": "user@example.com",
      "server": "imap.example.com",
      "port": 993,
      "username": "user@example.com",
      "protocol": "imap",
      "include_keyword": [],
      "exclude_keyword": [],
      "include_email": [],
      "exclude_email": [],
      "check_interval": 120,
      "check_history": 1000,
      "enable_notification_sound": true,
      "folder_mode": "all",
      "include_folders": [],
      "exclude_folders": []
    }
  ]
}
```


### Configuration Options

**Account Settings:**

- `email` - Email address for display purposes[^1]
- `server` - Mail server hostname[^1]
- `port` - Server port (typically 993 for IMAP, 995 for POP3)[^1]
- `username` - Login username[^1]
- `protocol` - Either "imap" or "pop3"[^1]

**Filtering:**

- `include_keyword` - Only notify for emails containing these keywords[^1]
- `exclude_keyword` - Skip emails containing these keywords[^1]
- `include_email` - Only notify for emails from these senders[^1]
- `exclude_email` - Skip emails from these senders[^1]

**Monitoring:**

- `check_interval` - Seconds between checks (default: 120)[^1]
- `check_history` - Number of recent emails to check (default: 1000)[^1]
- `enable_notification_sound` - Play sound with notifications[^1]

**Folder Settings (IMAP only):**

- `folder_mode` - "all", "include", or "exclude"[^1]
- `include_folders` - Folders to monitor when mode is "include"[^1]
- `exclude_folders` - Folders to skip when mode is "exclude"[^1]


### Password Management

Passwords are **NOT** stored in the configuration file. Use the web dashboard to set passwords, which are securely stored in your system keyring.[^1]

## Usage

### Running the Application

```bash
./email-monitor
```

The application will:

1. Start as a system tray icon with unread count display[^1]
2. Launch a local web server for the dashboard[^1]
3. Begin monitoring configured accounts at specified intervals[^1]

### Web Dashboard

Click **"Open Dashboard"** in the system tray menu to access the web interface. The dashboard allows you to:[^1]

- Add, edit, and delete email accounts[^1]
- Set passwords securely[^1]
- Test connection settings[^1]
- Fetch available folders (IMAP)[^1]
- View real-time status and unread counts[^1]
- Trigger manual email checks[^1]
- Clear notification history[^1]


### System Tray Menu

- **Open Dashboard** - Launch the web interface[^1]
- **Check All Accounts** - Manually trigger immediate check[^1]
- **Per-account items** - Show unread count for each account[^1]
- **Quit** - Stop monitoring and exit[^1]


## API Endpoints

The web server exposes these REST endpoints:[^1]

- `GET /` - Dashboard interface
- `GET /api/accounts` - List all accounts
- `POST /api/accounts/add` - Add new account
- `POST /api/accounts/update` - Update existing account
- `POST /api/accounts/delete` - Remove account
- `POST /api/accounts/test` - Test connection
- `POST /api/accounts/folders` - Fetch IMAP folders
- `GET /api/status` - Get monitoring status
- `POST /api/check-all` - Trigger manual check
- `POST /api/clear-history` - Clear notification history
- `POST /api/restart` - Restart application


## File Locations

All application data is stored in the platform-specific configuration directory:[^1]

- `config.json` - Account configuration (passwords excluded)
- `email-monitor.log` - Application logs
- `folders_list.json` - Cached IMAP folder lists
- `notification_history/` - Notification tracking to prevent duplicates


## Troubleshooting

### Connection Issues

- Verify server address and port are correct[^1]
- Use the "Test Connection" button in the dashboard[^1]
- Check if your email provider requires app-specific passwords
- Ensure firewall allows outbound connections on the specified port


### No Notifications

- Check filter settings aren't too restrictive[^1]
- Verify `check_interval` isn't too high[^1]
- Review logs in `email-monitor.log`[^1]
- Clear notification history if emails were already notified[^1]


### Password Issues

- Re-enter password through the web dashboard[^1]
- Ensure system keyring is accessible
- Check logs for keyring-related errors[^1]


## Platform Support

- **Linux** - Full support with system keyring integration[^1]
- **macOS** - Full support with Keychain integration[^1]
- **Windows** - Full support with Credential Manager integration[^1]


## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please submit issues and pull requests to the repository.

<div align="center">⁂</div>

[^1]: main.go


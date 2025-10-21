package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
	"github.com/knadh/go-pop3"
	"github.com/zalando/go-keyring"
)

const (
	keyringService = "email-monitor"
)

type AccountConfig struct {
	Email                   string   `json:"email"`
	Server                  string   `json:"server"`
	Port                    int      `json:"port"`
	Username                string   `json:"username"`
	Password                string   `json:"password,omitempty"`
	Protocol                string   `json:"protocol"` // "imap" or "pop3"
	IncludeKeyword          []string `json:"include_keyword"`
	ExcludeKeyword          []string `json:"exclude_keyword"`
	IncludeEmail            []string `json:"include_email"`
	ExcludeEmail            []string `json:"exclude_email"`
	CheckInterval           int      `json:"check_interval"`
	CheckHistory            int      `json:"check_history"`
	EnableNotificationSound bool     `json:"enable_notification_sound"`
	FolderMode              string   `json:"folder_mode"`
	IncludeFolders          []string `json:"include_folders"`
	ExcludeFolders          []string `json:"exclude_folders"`
	notifiedEmails          map[string]bool
	lastCheckTime           time.Time
	unreadCount             int
	mu                      sync.RWMutex
	stopChan                chan bool
	ticker                  *time.Ticker
}

type Config struct {
	Accounts []AccountConfig `json:"accounts"`
}

var (
	config           Config
	accountMenuItems map[string]*systray.MenuItem
	mu               sync.RWMutex
	appDir           string
	configFile       string
	logFile          string
	foldersListFile  string
	historyDir       string
	webServerPort    int
	webServerURL     string
)

func init() {
	var err error
	appDir, err = getAppDir()
	if err != nil {
		log.Fatalf("Failed to get application directory: %v", err)
	}

	if err := os.MkdirAll(appDir, 0755); err != nil {
		log.Fatalf("Failed to create application directory: %v", err)
	}

	configFile = filepath.Join(appDir, "config.json")
	logFile = filepath.Join(appDir, "email-monitor.log")
	foldersListFile = filepath.Join(appDir, "folders_list.json")
	historyDir = filepath.Join(appDir, "notification_history")
}

func getAppDir() (string, error) {
	oss := runtime.GOOS
	var baseDir string
	home := os.Getenv("HOME")

	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		baseDir = xdgConfig
	} else {
		switch oss {
		case "darwin":
			baseDir = filepath.Join(home, "Library", "Application Support")
		case "linux":
			baseDir = filepath.Join(home, ".config")
		case "windows":
			baseDir = os.Getenv("APPDATA")
		default:
			return "", fmt.Errorf("unable to determine user config directory")
		}
	}

	return filepath.Join(baseDir, "email-monitor"), nil
}

func setPassword(email, password string) error {
	return keyring.Set(keyringService, email, password)
}

func getPassword(email string) (string, error) {
	return keyring.Get(keyringService, email)
}

func deletePassword(email string) error {
	return keyring.Delete(keyringService, email)
}

func migratePasswordsToKeyring() {
	migrated := false
	for i := range config.Accounts {
		if config.Accounts[i].Password != "" {
			if err := setPassword(config.Accounts[i].Email, config.Accounts[i].Password); err != nil {
				log.Printf("[%s] Failed to migrate password to keyring: %v", config.Accounts[i].Email, err)
			} else {
				log.Printf("[%s] Migrated password to keyring", config.Accounts[i].Email)
				config.Accounts[i].Password = ""
				migrated = true
			}
		}
	}

	if migrated {
		if err := saveConfig(); err != nil {
			log.Printf("Failed to save config after migration: %v", err)
		}
	}
}

func main() {
	setupLogging()

	log.Printf("Application directory: %s", appDir)
	fmt.Printf("📧 Email Monitor (IMAP & POP3)\n")
	fmt.Printf("Application directory: %s\n\n", appDir)

	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}

	migratePasswordsToKeyring()

	os.MkdirAll(historyDir, 0755)

	for i := range config.Accounts {
		config.Accounts[i].notifiedEmails = make(map[string]bool)
		config.Accounts[i].stopChan = make(chan bool)
		loadNotifiedEmails(&config.Accounts[i])
		cleanupOldNotifications(&config.Accounts[i])
	}

	log.Printf("Starting email monitor for %d accounts", len(config.Accounts))

	go startWebServer()

	systray.Run(onReady, onExit)
}

func setupLogging() {
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(f)
	}
}

func loadConfig() error {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return createSampleConfig()
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}

	if len(config.Accounts) == 0 {
		return fmt.Errorf("no accounts configured")
	}

	for i := range config.Accounts {
		if config.Accounts[i].CheckInterval == 0 {
			config.Accounts[i].CheckInterval = 120
		}
		if config.Accounts[i].CheckHistory == 0 {
			config.Accounts[i].CheckHistory = 1000
		}
		if config.Accounts[i].Protocol == "" {
			config.Accounts[i].Protocol = "imap"
		}
		if config.Accounts[i].FolderMode == "" {
			config.Accounts[i].FolderMode = "all"
		}
	}

	return nil
}

func createSampleConfig() error {
	sampleConfig := Config{
		Accounts: []AccountConfig{
			{
				Email:                   "user@example.com",
				Server:                  "imap.example.com",
				Port:                    993,
				Username:                "user@example.com",
				Protocol:                "imap",
				IncludeKeyword:          []string{},
				ExcludeKeyword:          []string{},
				IncludeEmail:            []string{},
				ExcludeEmail:            []string{},
				CheckInterval:           120,
				CheckHistory:            1000,
				EnableNotificationSound: true,
				FolderMode:              "all",
				IncludeFolders:          []string{},
				ExcludeFolders:          []string{},
			},
		},
	}

	data, err := json.MarshalIndent(sampleConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to create sample config: %v", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write sample config: %v", err)
	}

	log.Printf("Created sample config file: %s", configFile)
	fmt.Printf("✅ Sample config created: %s\n", configFile)
	fmt.Printf("Please edit it with your email settings and restart.\n")
	fmt.Printf("\nNote: Passwords are stored securely in your system's keyring, not in the config file.\n")
	fmt.Printf("Supported protocols: IMAP and POP3\n")

	beeep.Notify("Email Monitor - Setup Required",
		fmt.Sprintf("Config file created at:\n%s\n\nPlease edit and restart.", configFile), "")

	os.Exit(0)
	return nil
}

func saveConfig() error {
	configCopy := Config{Accounts: make([]AccountConfig, len(config.Accounts))}
	for i, acc := range config.Accounts {
		configCopy.Accounts[i] = acc
		configCopy.Accounts[i].Password = ""
	}

	data, err := json.MarshalIndent(configCopy, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
}

func startWebServer() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	webServerPort = listener.Addr().(*net.TCPAddr).Port
	webServerURL = fmt.Sprintf("http://127.0.0.1:%d", webServerPort)
	listener.Close()

	log.Printf("Starting web server on %s", webServerURL)

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/accounts", handleAccounts)
	http.HandleFunc("/api/accounts/add", handleAddAccount)
	http.HandleFunc("/api/accounts/update", handleUpdateAccount)
	http.HandleFunc("/api/accounts/delete", handleDeleteAccount)
	http.HandleFunc("/api/accounts/test", handleTestConnection)
	http.HandleFunc("/api/accounts/folders", handleFetchFolders)
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/check-all", handleCheckAll)
	http.HandleFunc("/api/clear-history", handleClearHistory)
	http.HandleFunc("/api/restart", handleRestart)

	log.Fatal(http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", webServerPort), nil))
}

func onReady() {
	systray.SetIcon(getIconData())
	systray.SetTitle("📧")
	systray.SetTooltip(fmt.Sprintf("Email Monitor - Click to open"))

	time.Sleep(500 * time.Millisecond)

	systray.SetTooltip(fmt.Sprintf("Email Monitor (IMAP & POP3)\nClick to open dashboard\n%s", webServerURL))

	mOpen := systray.AddMenuItem("🖥️ Open Dashboard", "Open web dashboard")

	go func() {
		for {
			<-mOpen.ClickedCh
			log.Printf("Opening dashboard: %s", webServerURL)
			openBrowser(webServerURL)
		}
	}()

	for i := range config.Accounts {
		go startMonitoring(&config.Accounts[i])
	}
}

func onExit() {
	log.Println("Email monitor stopped")
	for i := range config.Accounts {
		if config.Accounts[i].stopChan != nil {
			close(config.Accounts[i].stopChan)
		}
		if config.Accounts[i].ticker != nil {
			config.Accounts[i].ticker.Stop()
		}
	}
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}
	if err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("home").Parse(`
		<!DOCTYPE html>
<html>
<head>
    <title>Email Monitor Dashboard</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Arial, sans-serif;
            background: #f5f5f5;
            padding: 20px;
        }
        .container { max-width: 1200px; margin: 0 auto; }
        .header {
            background: white;
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .header h1 { color: #333; margin-bottom: 10px; }
        .protocol-badge {
            display: inline-block;
            padding: 3px 8px;
            border-radius: 12px;
            font-size: 11px;
            font-weight: bold;
            margin-left: 8px;
        }
        .protocol-imap { background: #2196f3; color: white; }
        .protocol-pop3 { background: #ff9800; color: white; }
        .security-note {
            background: #e8f5e9;
            border-left: 4px solid #4caf50;
            padding: 10px 15px;
            margin-top: 10px;
            border-radius: 4px;
            font-size: 14px;
            color: #2e7d32;
        }
        .actions {
            display: flex;
            gap: 10px;
            margin-top: 15px;
        }
        .btn {
            padding: 10px 20px;
            border: none;
            border-radius: 5px;
            cursor: pointer;
            font-size: 14px;
            transition: all 0.3s;
        }
        .btn-primary { background: #007bff; color: white; }
        .btn-primary:hover { background: #0056b3; }
        .btn-success { background: #28a745; color: white; }
        .btn-success:hover { background: #218838; }
        .btn-danger { background: #dc3545; color: white; }
        .btn-danger:hover { background: #c82333; }
        .btn-warning { background: #ffc107; color: black; }
        .btn-warning:hover { background: #e0a800; }
        .accounts-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .account-card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .account-card h3 {
            color: #333;
            margin-bottom: 15px;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .account-card .detail {
            margin: 8px 0;
            font-size: 14px;
            color: #666;
        }
        .account-card .detail strong {
            color: #333;
            display: inline-block;
            width: 140px;
        }
        .account-actions {
            margin-top: 15px;
            display: flex;
            gap: 8px;
        }
        .btn-sm {
            padding: 6px 12px;
            font-size: 12px;
        }
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.5);
            z-index: 1000;
        }
        .modal-content {
            background: white;
            margin: 50px auto;
            padding: 30px;
            border-radius: 8px;
            max-width: 600px;
            max-height: 80vh;
            overflow-y: auto;
        }
        .form-group {
            margin-bottom: 15px;
        }
        .form-group label {
            display: block;
            margin-bottom: 5px;
            color: #333;
            font-weight: 500;
        }
        .form-group input, .form-group select {
            width: 100%;
            padding: 8px;
            border: 1px solid #ddd;
            border-radius: 4px;
            font-size: 14px;
        }
        .toast {
            position: fixed;
            top: 20px;
            right: 20px;
            background: #333;
            color: white;
            padding: 15px 20px;
            border-radius: 5px;
            display: none;
            z-index: 2000;
        }
        .toast.show { display: block; }
        .toast.success { background: #28a745; }
        .toast.error { background: #dc3545; }
        .keyring-badge {
            display: inline-block;
            background: #4caf50;
            color: white;
            padding: 2px 8px;
            border-radius: 12px;
            font-size: 11px;
            margin-left: 8px;
        }
        .protocol-note {
            background: #fff3cd;
            border-left: 4px solid #ff9800;
            padding: 10px 15px;
            margin-top: 10px;
            border-radius: 4px;
            font-size: 13px;
            color: #856404;
        }
        #folderSettings {
            display: none;
        }
        #folderSelection, #editFolderSelection {
            display: none;
        }
        .folder-list-container {
            max-height: 150px;
            overflow-y: auto;
            border: 1px solid #ddd;
            padding: 10px;
            border-radius: 4px;
            background: #f9f9f9;
        }
        .folder-checkbox-item {
            margin-bottom: 5px;
        }
        .folder-checkbox-label {
            display: flex;
            align-items: center;
            cursor: pointer;
        }
        .folder-checkbox-label input {
            margin-right: 8px;
            width: auto;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Email Monitor Dashboard</h1>
            <div class="security-note">
                🔒 <strong>Secure Storage:</strong> Passwords are stored in your system's keyring (not in config files)
            </div>
            <p style="margin-top: 10px;"><strong>Supported Protocols:</strong> <span class="protocol-badge protocol-imap">IMAP</span> <span class="protocol-badge protocol-pop3">POP3</span></p>
            <p style="margin-top: 5px;">Application Directory: {{.AppDir}}</p>
            <div class="actions">
                <button class="btn btn-primary" onclick="showAddModal()">Add Account</button>
                <button class="btn btn-success" onclick="checkAll()">Check All Now</button>
                <button class="btn btn-warning" onclick="clearHistory()">Clear History</button>
                <button class="btn btn-danger" onclick="restartMonitor()">Restart</button>
            </div>
        </div>

        <div id="accounts" class="accounts-grid"></div>
    </div>

    <div id="addModal" class="modal">
        <div class="modal-content">
            <h2>Add Account</h2>
            <form id="addForm">
                <div class="form-group">
                    <label>Protocol</label>
                    <select id="protocol" onchange="updateProtocolSettings()">
                        <option value="imap">IMAP (recommended)</option>
                        <option value="pop3">POP3</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Provider</label>
                    <select id="provider" onchange="setProvider()">
                        <option value="custom">Custom</option>
                        <option value="gmail">Gmail</option>
                        <option value="outlook">Outlook</option>
                        <option value="yahoo">Yahoo</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Email</label>
                    <input type="email" id="email" required>
                </div>
                <div class="form-group">
                    <label id="serverLabel">Server</label>
                    <input type="text" id="server" required>
                </div>
                <div class="form-group">
                    <label>Port</label>
                    <input type="number" id="port" value="993" required>
                </div>
                <div class="form-group">
                    <label>Username</label>
                    <input type="text" id="username" required>
                </div>
                <div class="form-group">
                    <label>Password <span class="keyring-badge">🔒 Secure</span></label>
                    <input type="password" id="password" required>
                    <small style="color:#666;">Password will be stored securely in system keyring</small>
                </div>
                <div class="form-group">
                    <label>Check Interval (seconds)</label>
                    <input type="number" id="interval" value="120" required>
                </div>
                <div id="folderSettings">
                    <div class="protocol-note">
                        ℹ️ <strong>Note:</strong> Folder selection is only available for IMAP. POP3 only accesses the inbox.
                    </div>
                    <div class="form-group">
                        <label>Folder Mode</label>
                        <select id="folderMode" onchange="updateFolderMode()">
                            <option value="all">All Folders</option>
                            <option value="include">Include Specific Folders</option>
                            <option value="exclude">Exclude Specific Folders</option>
                        </select>
                    </div>
                    <div id="folderSelection">
                        <div class="form-group">
                            <button type="button" class="btn btn-primary btn-sm" onclick="fetchFolders()">📁 Fetch Folders from Server</button>
                            <small style="color:#666;display:block;margin-top:5px;">Click to retrieve available folders from your email account</small>
                        </div>
                        <div class="form-group">
                            <label id="folderListLabel">Select Folders</label>
                            <div id="folderList" class="folder-list-container">
                                <p style="color:#999;text-align:center;">Click "Fetch Folders" to load available folders</p>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- FILTERS SECTION -->
                <div class="form-group">
                    <label>🔍 Filters (Optional)</label>
                    <small style="color:#666;display:block;margin-bottom:10px;">Configure keyword and email filters to control which notifications you receive</small>
                </div>

                <div class="form-group">
                    <label>Include Keywords (comma-separated)</label>
                    <input type="text" id="includeKeywords" placeholder="urgent, invoice, payment">
                    <small style="color:#666;">Only notify for emails with these keywords in subject (leave empty for all)</small>
                </div>

                <div class="form-group">
                    <label>Exclude Keywords (comma-separated)</label>
                    <input type="text" id="excludeKeywords" placeholder="newsletter, marketing, promo">
                    <small style="color:#666;">Never notify for emails with these keywords in subject</small>
                </div>

                <div class="form-group">
                    <label>Include Email Addresses (comma-separated)</label>
                    <input type="text" id="includeEmails" placeholder="boss@company.com, client@example.com">
                    <small style="color:#666;">Only notify for emails from these addresses (leave empty for all)</small>
                </div>

                <div class="form-group">
                    <label>Exclude Email Addresses (comma-separated)</label>
                    <input type="text" id="excludeEmails" placeholder="spam@example.com, noreply@marketing.com">
                    <small style="color:#666;">Never notify for emails from these addresses</small>
                </div>

                <div style="display: flex; gap: 10px; margin-top: 20px;">
                    <button type="button" class="btn btn-primary" onclick="testConnection()">Test Connection</button>
                    <button type="submit" class="btn btn-success">Save</button>
                    <button type="button" class="btn btn-danger" onclick="closeModal()">Cancel</button>
                </div>
            </form>
        </div>
    </div>

    <div id="editModal" class="modal">
        <div class="modal-content">
            <h2>Edit Account</h2>
            <form id="editForm">
                <input type="hidden" id="editIndex">
                <div class="form-group">
                    <label>Protocol</label>
                    <select id="editProtocol" readonly disabled style="background:#f5f5f5;">
                        <option value="imap">IMAP</option>
                        <option value="pop3">POP3</option>
                    </select>
                    <small style="color:#666;">Protocol cannot be changed</small>
                </div>
                <div class="form-group">
                    <label>Email</label>
                    <input type="email" id="editEmail" required readonly style="background:#f5f5f5;">
                    <small style="color:#666;">Email cannot be changed (used as keyring identifier)</small>
                </div>
                <div class="form-group">
                    <label id="editServerLabel">Server</label>
                    <input type="text" id="editServer" required>
                </div>
                <div class="form-group">
                    <label>Port</label>
                    <input type="number" id="editPort" required>
                </div>
                <div class="form-group">
                    <label>Username</label>
                    <input type="text" id="editUsername" required>
                </div>
                <div class="form-group">
                    <label>Password <span class="keyring-badge">🔒 Secure</span></label>
                    <input type="password" id="editPassword" placeholder="Leave empty to keep existing">
                    <small style="color:#666;">Leave empty to keep current password</small>
                </div>
                <div class="form-group">
                    <label>Check Interval (seconds)</label>
                    <input type="number" id="editInterval" required>
                </div>
                <div id="editFolderSettings">
                    <div class="form-group">
                        <label>Folder Mode</label>
                        <select id="editFolderMode" onchange="updateEditFolderMode()">
                            <option value="all">All Folders</option>
                            <option value="include">Include Specific</option>
                            <option value="exclude">Exclude Specific</option>
                        </select>
                    </div>
                    <div id="editFolderSelection">
                        <div class="form-group">
                            <button type="button" class="btn btn-primary btn-sm" onclick="fetchEditFolders()">📁 Fetch Folders from Server</button>
                            <small style="color:#666;display:block;margin-top:5px;">Click to retrieve available folders</small>
                        </div>
                        <div class="form-group">
                            <label id="editFolderListLabel">Select Folders</label>
                            <div id="editFolderList" class="folder-list-container">
                                <p style="color:#999;text-align:center;">Click "Fetch Folders" to load available folders</p>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="form-group">
                    <label>🔍 Filters (Optional)</label>
                    <small style="color:#666;display:block;margin-bottom:10px;">Configure keyword and email filters</small>
                </div>

                <div class="form-group">
                    <label>Include Keywords (comma-separated)</label>
                    <input type="text" id="editIncludeKeywords" placeholder="urgent, invoice, payment">
                    <small style="color:#666;">Only notify for emails with these keywords in subject</small>
                </div>

                <div class="form-group">
                    <label>Exclude Keywords (comma-separated)</label>
                    <input type="text" id="editExcludeKeywords" placeholder="newsletter, marketing">
                    <small style="color:#666;">Never notify for emails with these keywords</small>
                </div>

                <div class="form-group">
                    <label>Include Email Addresses (comma-separated)</label>
                    <input type="text" id="editIncludeEmails" placeholder="boss@company.com">
                    <small style="color:#666;">Only notify for emails from these addresses</small>
                </div>

                <div class="form-group">
                    <label>Exclude Email Addresses (comma-separated)</label>
                    <input type="text" id="editExcludeEmails" placeholder="spam@example.com">
                    <small style="color:#666;">Never notify for emails from these addresses</small>
                </div>

                <div style="display: flex; gap: 10px; margin-top: 20px;">
                    <button type="submit" class="btn btn-success">Save</button>
                    <button type="button" class="btn btn-danger" onclick="closeEditModal()">Cancel</button>
                </div>
            </form>
        </div>
    </div>

    <div id="toast" class="toast"></div>

    <script>
        let availableFolders = [];
        let selectedFolders = [];
        let editAvailableFolders = [];
        let editSelectedFolders = [];

        function updateProtocolSettings() {
            const protocol = document.getElementById('protocol').value;
            const folderSettings = document.getElementById('folderSettings');
            const serverLabel = document.getElementById('serverLabel');

            if (protocol === 'pop3') {
                folderSettings.style.display = 'none';
                serverLabel.textContent = 'POP3 Server';
                document.getElementById('port').value = '995';
            } else {
                folderSettings.style.display = 'block';
                serverLabel.textContent = 'IMAP Server';
                document.getElementById('port').value = '993';
            }
            updateFolderMode();
        }

        function updateFolderMode() {
            const folderMode = document.getElementById('folderMode').value;
            const folderSelection = document.getElementById('folderSelection');
            const folderListLabel = document.getElementById('folderListLabel');

            if (folderMode === 'all') {
                folderSelection.style.display = 'none';
            } else {
                folderSelection.style.display = 'block';
                folderListLabel.textContent = folderMode === 'include' ? 'Include These Folders' : 'Exclude These Folders';
            }
        }

        function updateEditFolderMode() {
            const folderMode = document.getElementById('editFolderMode').value;
            const folderSelection = document.getElementById('editFolderSelection');
            const folderListLabel = document.getElementById('editFolderListLabel');

            if (folderMode === 'all') {
                folderSelection.style.display = 'none';
            } else {
                folderSelection.style.display = 'block';
                folderListLabel.textContent = folderMode === 'include' ? 'Include These Folders' : 'Exclude These Folders';
            }
        }

        async function fetchFolders() {
            const server = document.getElementById('server').value;
            const port = parseInt(document.getElementById('port').value);
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            const protocol = document.getElementById('protocol').value;

            if (!server || !username || !password) {
                showToast('Please fill in server, username, and password first', 'error');
                return;
            }

            try {
                const response = await fetch('/api/accounts/folders', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ server, port, username, password, protocol })
                });
                const result = await response.json();

                if (result.success) {
                    availableFolders = result.folders || [];
                    renderFolderList();
                    showToast(result.message, 'success');
                } else {
                    showToast(result.message, 'error');
                }
            } catch (error) {
                showToast('Failed to fetch folders: ' + error, 'error');
            }
        }

        async function fetchEditFolders() {
            const index = parseInt(document.getElementById('editIndex').value);
            const accounts = await (await fetch('/api/accounts')).json();
            const acc = accounts[index];

            const password = document.getElementById('editPassword').value || '';

            try {
                const response = await fetch('/api/accounts/folders', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        server: acc.server,
                        port: acc.port,
                        username: acc.username,
                        password: password || 'dummy',
                        protocol: acc.protocol
                    })
                });
                const result = await response.json();

                if (result.success) {
                    editAvailableFolders = result.folders || [];
                    renderEditFolderList();
                    showToast(result.message, 'success');
                } else {
                    showToast(result.message, 'error');
                }
            } catch (error) {
                showToast('Failed to fetch folders: ' + error, 'error');
            }
        }

        function renderFolderList() {
            const container = document.getElementById('folderList');
            if (availableFolders.length === 0) {
                container.innerHTML = '<p style="color:#999;text-align:center;">No folders found</p>';
                return;
            }

            container.innerHTML = availableFolders.map(folder => {
                const isSelected = selectedFolders.includes(folder);
                const escapedFolder = folder.replace(/'/g, "\\'");
                return ` + "`" + `
                    <div class="folder-checkbox-item">
                        <label class="folder-checkbox-label">
                            <input type="checkbox" value="${folder}"
                                ${isSelected ? 'checked' : ''}
                                onchange="toggleFolder('${escapedFolder}')">
                            <span>${folder}</span>
                        </label>
                    </div>
                ` + "`" + `;
            }).join('');
        }

        function renderEditFolderList() {
            const container = document.getElementById('editFolderList');
            if (editAvailableFolders.length === 0) {
                container.innerHTML = '<p style="color:#999;text-align:center;">No folders found</p>';
                return;
            }

            container.innerHTML = editAvailableFolders.map(folder => {
                const isSelected = editSelectedFolders.includes(folder);
                const escapedFolder = folder.replace(/'/g, "\\'");
                return ` + "`" + `
                    <div class="folder-checkbox-item">
                        <label class="folder-checkbox-label">
                            <input type="checkbox" value="${folder}"
                                ${isSelected ? 'checked' : ''}
                                onchange="toggleEditFolder('${escapedFolder}')">
                            <span>${folder}</span>
                        </label>
                    </div>
                ` + "`" + `;
            }).join('');
        }

        function toggleFolder(folder) {
            const index = selectedFolders.indexOf(folder);
            if (index > -1) {
                selectedFolders.splice(index, 1);
            } else {
                selectedFolders.push(folder);
            }
        }

        function toggleEditFolder(folder) {
            const index = editSelectedFolders.indexOf(folder);
            if (index > -1) {
                editSelectedFolders.splice(index, 1);
            } else {
                editSelectedFolders.push(folder);
            }
        }

        function showToast(message, type = 'success') {
            const toast = document.getElementById('toast');
            toast.textContent = message;
            toast.className = 'toast show ' + type;
            setTimeout(() => toast.className = 'toast', 3000);
        }

        function setProvider() {
            const provider = document.getElementById('provider').value;
            const protocol = document.getElementById('protocol').value;
            const servers = {
                gmail: {
                    imap: { server: 'imap.gmail.com', port: 993 },
                    pop3: { server: 'pop.gmail.com', port: 995 }
                },
                outlook: {
                    imap: { server: 'outlook.office365.com', port: 993 },
                    pop3: { server: 'outlook.office365.com', port: 995 }
                },
                yahoo: {
                    imap: { server: 'imap.mail.yahoo.com', port: 993 },
                    pop3: { server: 'pop.mail.yahoo.com', port: 995 }
                }
            };
            if (servers[provider] && servers[provider][protocol]) {
                document.getElementById('server').value = servers[provider][protocol].server;
                document.getElementById('port').value = servers[provider][protocol].port;
            }
        }

        function showAddModal() {
            document.getElementById('addModal').style.display = 'block';
            document.getElementById('addForm').reset();
            selectedFolders = [];
            availableFolders = [];
            updateProtocolSettings();
        }

        function closeModal() {
            document.getElementById('addModal').style.display = 'none';
        }

        function closeEditModal() {
            document.getElementById('editModal').style.display = 'none';
        }

        async function testConnection() {
            const data = {
                protocol: document.getElementById('protocol').value,
                server: document.getElementById('server').value,
                port: parseInt(document.getElementById('port').value),
                username: document.getElementById('username').value,
                password: document.getElementById('password').value
            };

            try {
                const response = await fetch('/api/accounts/test', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(data)
                });
                const result = await response.json();
                showToast(result.message, result.success ? 'success' : 'error');
            } catch (error) {
                showToast('Test failed: ' + error, 'error');
            }
        }

        document.getElementById('addForm').addEventListener('submit', async (e) => {
            e.preventDefault();

            const folderMode = document.getElementById('folderMode').value;
            let includeFolders = [];
            let excludeFolders = [];

            if (folderMode === 'include') {
                includeFolders = selectedFolders;
            } else if (folderMode === 'exclude') {
                excludeFolders = selectedFolders;
            }

            const data = {
                protocol: document.getElementById('protocol').value,
                email: document.getElementById('email').value,
                server: document.getElementById('server').value,
                port: parseInt(document.getElementById('port').value),
                username: document.getElementById('username').value,
                password: document.getElementById('password').value,
                check_interval: parseInt(document.getElementById('interval').value),
                folder_mode: folderMode,
                include_folders: includeFolders,
                exclude_folders: excludeFolders,
                include_keyword: document.getElementById('includeKeywords').value.split(',').map(s => s.trim()).filter(s => s),
                exclude_keyword: document.getElementById('excludeKeywords').value.split(',').map(s => s.trim()).filter(s => s),
                include_email: document.getElementById('includeEmails').value.split(',').map(s => s.trim()).filter(s => s),
                exclude_email: document.getElementById('excludeEmails').value.split(',').map(s => s.trim()).filter(s => s)
            };

            try {
                const response = await fetch('/api/accounts/add', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(data)
                });
                if (response.ok) {
                    showToast('Account added successfully (password stored securely)');
                    closeModal();
                    loadAccounts();
                } else {
                    showToast('Failed to add account', 'error');
                }
            } catch (error) {
                showToast('Error: ' + error, 'error');
            }
        });

        document.getElementById('editForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const index = parseInt(document.getElementById('editIndex').value);

            const folderMode = document.getElementById('editFolderMode').value;
            let includeFolders = [];
            let excludeFolders = [];

            if (folderMode === 'include') {
                includeFolders = editSelectedFolders;
            } else if (folderMode === 'exclude') {
                excludeFolders = editSelectedFolders;
            }

            const data = {
                index: index,
                email: document.getElementById('editEmail').value,
                server: document.getElementById('editServer').value,
                port: parseInt(document.getElementById('editPort').value),
                username: document.getElementById('editUsername').value,
                password: document.getElementById('editPassword').value,
                check_interval: parseInt(document.getElementById('editInterval').value),
                folder_mode: folderMode,
                include_folders: includeFolders,
                exclude_folders: excludeFolders,
                include_keyword: document.getElementById('editIncludeKeywords').value.split(',').map(s => s.trim()).filter(s => s),
                exclude_keyword: document.getElementById('editExcludeKeywords').value.split(',').map(s => s.trim()).filter(s => s),
                include_email: document.getElementById('editIncludeEmails').value.split(',').map(s => s.trim()).filter(s => s),
                exclude_email: document.getElementById('editExcludeEmails').value.split(',').map(s => s.trim()).filter(s => s)
            };

            try {
                const response = await fetch('/api/accounts/update', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(data)
                });
                if (response.ok) {
                    showToast('Account updated successfully');
                    closeEditModal();
                    loadAccounts();
                } else {
                    showToast('Failed to update account', 'error');
                }
            } catch (error) {
                showToast('Error: ' + error, 'error');
            }
        });

        function editAccount(index) {
            fetch('/api/accounts')
                .then(r => r.json())
                .then(accounts => {
                    const acc = accounts[index];
                    document.getElementById('editIndex').value = index;
                    document.getElementById('editProtocol').value = acc.protocol;
                    document.getElementById('editEmail').value = acc.email;
                    document.getElementById('editServer').value = acc.server;
                    document.getElementById('editPort').value = acc.port;
                    document.getElementById('editUsername').value = acc.username;
                    document.getElementById('editPassword').value = '';
                    document.getElementById('editInterval').value = acc.check_interval;
                    document.getElementById('editFolderMode').value = acc.folder_mode;
                    document.getElementById('editIncludeKeywords').value = (acc.include_keyword || []).join(', ');
                    document.getElementById('editExcludeKeywords').value = (acc.exclude_keyword || []).join(', ');
                    document.getElementById('editIncludeEmails').value = (acc.include_email || []).join(', ');
                    document.getElementById('editExcludeEmails').value = (acc.exclude_email || []).join(', ');

                    editAvailableFolders = [];
                    editSelectedFolders = [];

                    if (acc.folder_mode === 'include') {
                        editSelectedFolders = acc.include_folders || [];
                    } else if (acc.folder_mode === 'exclude') {
                        editSelectedFolders = acc.exclude_folders || [];
                    }

                    if (acc.protocol === 'pop3') {
                        document.getElementById('editFolderSettings').style.display = 'none';
                    } else {
                        document.getElementById('editFolderSettings').style.display = 'block';
                    }

                    updateEditFolderMode();
                    document.getElementById('editModal').style.display = 'block';
                });
        }

        async function deleteAccount(index) {
            if (!confirm('Are you sure you want to delete this account? This will also remove the password from keyring.')) return;

            try {
                const response = await fetch('/api/accounts/delete', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ index: index })
                });
                if (response.ok) {
                    showToast('Account deleted successfully');
                    loadAccounts();
                } else {
                    showToast('Failed to delete account', 'error');
                }
            } catch (error) {
                showToast('Error: ' + error, 'error');
            }
        }

        async function checkAll() {
            try {
                await fetch('/api/check-all', { method: 'POST' });
                showToast('Checking all accounts...');
            } catch (error) {
                showToast('Error: ' + error, 'error');
            }
        }

        async function clearHistory() {
            if (!confirm('Clear all notification history?')) return;
            try {
                await fetch('/api/clear-history', { method: 'POST' });
                showToast('History cleared');
            } catch (error) {
                showToast('Error: ' + error, 'error');
            }
        }

        async function restartMonitor() {
            try {
                await fetch('/api/restart', { method: 'POST' });
                showToast('Monitor restarting...');
                setTimeout(loadAccounts, 2000);
            } catch (error) {
                showToast('Error: ' + error, 'error');
            }
        }

        async function loadAccounts() {
            try {
                const response = await fetch('/api/accounts');
                const accounts = await response.json();

                const container = document.getElementById('accounts');
                if (accounts.length === 0) {
                    container.innerHTML = '<p style="text-align:center;color:#666;">No accounts configured. Click "Add Account" to get started.</p>';
                    return;
                }

                container.innerHTML = accounts.map((acc, index) => {
                    const protocolClass = acc.protocol === 'pop3' ? 'protocol-pop3' : 'protocol-imap';
                    const protocolText = acc.protocol.toUpperCase();
                    return ` + "`" + `
                    <div class="account-card">
                        <h3>${acc.email} <span class="protocol-badge ${protocolClass}">${protocolText}</span></h3>
                        <div class="detail"><strong>Server:</strong> ${acc.server}:${acc.port}</div>
                        <div class="detail"><strong>Interval:</strong> ${acc.check_interval}s</div>
                        ${acc.protocol === 'imap' ? ` + "`" + `<div class="detail"><strong>Folder Mode:</strong> ${acc.folder_mode}</div>` + "`" + ` : ''}
                        ${acc.protocol === 'imap' && acc.folder_mode === 'include' && acc.include_folders && acc.include_folders.length > 0 ?
                            ` + "`" + `<div class="detail"><strong>Include Folders:</strong> ${acc.include_folders.join(', ')}</div>` + "`" + ` : ''}
                        ${acc.protocol === 'imap' && acc.folder_mode === 'exclude' && acc.exclude_folders && acc.exclude_folders.length > 0 ?
                            ` + "`" + `<div class="detail"><strong>Exclude Folders:</strong> ${acc.exclude_folders.join(', ')}</div>` + "`" + ` : ''}
                        ${acc.include_keyword && acc.include_keyword.length > 0 ?
                            ` + "`" + `<div class="detail"><strong>Include Keywords:</strong> ${acc.include_keyword.join(', ')}</div>` + "`" + ` : ''}
                        ${acc.exclude_keyword && acc.exclude_keyword.length > 0 ?
                            ` + "`" + `<div class="detail"><strong>Exclude Keywords:</strong> ${acc.exclude_keyword.join(', ')}</div>` + "`" + ` : ''}
                        ${acc.include_email && acc.include_email.length > 0 ?
                            ` + "`" + `<div class="detail"><strong>Include Emails:</strong> ${acc.include_email.join(', ')}</div>` + "`" + ` : ''}
                        ${acc.exclude_email && acc.exclude_email.length > 0 ?
                            ` + "`" + `<div class="detail"><strong>Exclude Emails:</strong> ${acc.exclude_email.join(', ')}</div>` + "`" + ` : ''}
                        <div class="detail"><strong>Last Check:</strong> ${acc.last_check || 'Never'}</div>
                        <div class="account-actions">
                            <button class="btn btn-primary btn-sm" onclick="editAccount(${index})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteAccount(${index})">Delete</button>
                        </div>
                    </div>
                    ` + "`" + `;
                }).join('');
            } catch (error) {
                console.error('Failed to load accounts:', error);
            }
        }

        loadAccounts();
        setInterval(loadAccounts, 10000);
    </script>
</body>
</html>`))
	data := struct {
		AppDir string
	}{
		AppDir: appDir,
	}

	tmpl.Execute(w, data)
}

func handleAccounts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type AccountResponse struct {
		Email          string   `json:"email"`
		Server         string   `json:"server"`
		Port           int      `json:"port"`
		Username       string   `json:"username"`
		Protocol       string   `json:"protocol"`
		CheckInterval  int      `json:"check_interval"`
		FolderMode     string   `json:"folder_mode"`
		IncludeFolders []string `json:"include_folders"`
		ExcludeFolders []string `json:"exclude_folders"`
		LastCheck      string   `json:"last_check"`
		IncludeKeyword []string `json:"include_keyword"`
		ExcludeKeyword []string `json:"exclude_keyword"`
		IncludeEmail   []string `json:"include_email"`
		ExcludeEmail   []string `json:"exclude_email"`
	}

	accounts := make([]AccountResponse, len(config.Accounts))
	for i, acc := range config.Accounts {
		acc.mu.RLock()
		lastCheck := ""
		if !acc.lastCheckTime.IsZero() {
			lastCheck = acc.lastCheckTime.Format("15:04:05")
		}
		acc.mu.RUnlock()

		accounts[i] = AccountResponse{
			Email:          acc.Email,
			Server:         acc.Server,
			Port:           acc.Port,
			Username:       acc.Username,
			Protocol:       acc.Protocol,
			CheckInterval:  acc.CheckInterval,
			FolderMode:     acc.FolderMode,
			IncludeFolders: acc.IncludeFolders,
			ExcludeFolders: acc.ExcludeFolders,
			IncludeKeyword: acc.IncludeKeyword,
			ExcludeKeyword: acc.ExcludeKeyword,
			IncludeEmail:   acc.IncludeEmail,
			ExcludeEmail:   acc.ExcludeEmail,
			LastCheck:      lastCheck,
		}
	}

	json.NewEncoder(w).Encode(accounts)
}

func handleAddAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var newAccount struct {
		Email          string   `json:"email"`
		Server         string   `json:"server"`
		Port           int      `json:"port"`
		Username       string   `json:"username"`
		Password       string   `json:"password"`
		Protocol       string   `json:"protocol"`
		CheckInterval  int      `json:"check_interval"`
		FolderMode     string   `json:"folder_mode"`
		IncludeFolders []string `json:"include_folders"`
		ExcludeFolders []string `json:"exclude_folders"`
		IncludeKeyword []string `json:"include_keyword"`
		ExcludeKeyword []string `json:"exclude_keyword"`
		IncludeEmail   []string `json:"include_email"`
		ExcludeEmail   []string `json:"exclude_email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&newAccount); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := setPassword(newAccount.Email, newAccount.Password); err != nil {
		http.Error(w, fmt.Sprintf("Failed to store password in keyring: %v", err), http.StatusInternalServerError)
		return
	}

	acc := AccountConfig{
		Email:                   newAccount.Email,
		Server:                  newAccount.Server,
		Port:                    newAccount.Port,
		Username:                newAccount.Username,
		Protocol:                newAccount.Protocol,
		CheckInterval:           newAccount.CheckInterval,
		CheckHistory:            1000,
		EnableNotificationSound: true,
		FolderMode:              newAccount.FolderMode,
		IncludeFolders:          newAccount.IncludeFolders,
		ExcludeFolders:          newAccount.ExcludeFolders,
		IncludeKeyword:          newAccount.IncludeKeyword,
		ExcludeKeyword:          newAccount.ExcludeKeyword,
		IncludeEmail:            newAccount.IncludeEmail,
		ExcludeEmail:            newAccount.ExcludeEmail,
		notifiedEmails:          make(map[string]bool),
		stopChan:                make(chan bool),
	}

	config.Accounts = append(config.Accounts, acc)

	if err := saveConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go startMonitoring(&config.Accounts[len(config.Accounts)-1])

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var update struct {
		Index          int      `json:"index"`
		Email          string   `json:"email"`
		Server         string   `json:"server"`
		Port           int      `json:"port"`
		Username       string   `json:"username"`
		Password       string   `json:"password"`
		CheckInterval  int      `json:"check_interval"`
		FolderMode     string   `json:"folder_mode"`
		IncludeFolders []string `json:"include_folders"`
		ExcludeFolders []string `json:"exclude_folders"`
		IncludeKeyword []string `json:"include_keyword"`
		ExcludeKeyword []string `json:"exclude_keyword"`
		IncludeEmail   []string `json:"include_email"`
		ExcludeEmail   []string `json:"exclude_email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if update.Index < 0 || update.Index >= len(config.Accounts) {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	acc := &config.Accounts[update.Index]

	if update.Password != "" {
		if err := setPassword(acc.Email, update.Password); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update password in keyring: %v", err), http.StatusInternalServerError)
			return
		}
	}

	acc.Server = update.Server
	acc.Port = update.Port
	acc.Username = update.Username
	acc.CheckInterval = update.CheckInterval
	acc.FolderMode = update.FolderMode
	acc.IncludeFolders = update.IncludeFolders
	acc.ExcludeFolders = update.ExcludeFolders
	acc.IncludeKeyword = update.IncludeKeyword
	acc.ExcludeKeyword = update.ExcludeKeyword
	acc.IncludeEmail = update.IncludeEmail
	acc.ExcludeEmail = update.ExcludeEmail

	if err := saveConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	acc.stopChan <- true
	time.Sleep(100 * time.Millisecond)
	acc.stopChan = make(chan bool)
	go startMonitoring(acc)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Index int `json:"index"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Index < 0 || req.Index >= len(config.Accounts) {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	email := config.Accounts[req.Index].Email

	config.Accounts[req.Index].stopChan <- true

	if err := deletePassword(email); err != nil {
		log.Printf("Failed to delete password from keyring: %v", err)
	}

	config.Accounts = append(config.Accounts[:req.Index], config.Accounts[req.Index+1:]...)

	if err := saveConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func handleFetchFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Server   string `json:"server"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		Protocol string `json:"protocol"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Protocol == "pop3" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "POP3 does not support folder listing (inbox only)",
		})
		return
	}

	c, err := client.DialTLS(fmt.Sprintf("%s:%d", req.Server, req.Port), nil)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Connection failed: %v", err),
		})
		return
	}
	defer c.Logout()

	if err := c.Login(req.Username, req.Password); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Login failed: %v", err),
		})
		return
	}

	mailboxes := make(chan *imap.MailboxInfo, 100)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	var folders []string
	for m := range mailboxes {
		folders = append(folders, m.Name)
	}

	if err := <-done; err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to list folders: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"folders": folders,
		"message": fmt.Sprintf("Successfully retrieved %d folders", len(folders)),
	})
}

func handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var test struct {
		Protocol string `json:"protocol"`
		Server   string `json:"server"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&test); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if test.Protocol == "pop3" {
		if err := testPOP3Connection(test.Server, test.Port, test.Username, test.Password); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("POP3 connection failed: %v", err),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "✅ POP3 connection successful!",
		})
		return
	}

	c, err := client.DialTLS(fmt.Sprintf("%s:%d", test.Server, test.Port), nil)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Connection failed: %v", err),
		})
		return
	}
	defer c.Logout()

	if err := c.Login(test.Username, test.Password); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Login failed: %v", err),
		})
		return
	}

	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	folderCount := 0
	for range mailboxes {
		folderCount++
	}

	if err := <-done; err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("List folders failed: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("✅ IMAP connected successfully! Found %d folders", folderCount),
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accounts": len(config.Accounts),
		"running":  true,
	})
}

func handleCheckAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go checkAllAccounts()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "checking"})
}

func handleClearHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go clearAllHistory()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}

func handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go restartAllMonitors()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "restarting"})
}

func startMonitoring(acc *AccountConfig) {
	log.Printf("[%s] Monitor started (protocol: %s, interval: %ds)", acc.Email, acc.Protocol, acc.CheckInterval)

	acc.ticker = time.NewTicker(time.Duration(acc.CheckInterval) * time.Second)
	defer acc.ticker.Stop()

	if acc.Protocol == "pop3" {
		checkNewEmailsPOP3(acc)
	} else {
		checkNewEmails(acc)
	}

	for {
		select {
		case <-acc.ticker.C:
			if acc.Protocol == "pop3" {
				checkNewEmailsPOP3(acc)
			} else {
				checkNewEmails(acc)
			}
		case <-acc.stopChan:
			log.Printf("[%s] Monitor stopped", acc.Email)
			return
		}
	}
}

func checkNewEmails(acc *AccountConfig) error {
	c, err := connectToIMAP(acc)
	if err != nil {
		log.Printf("[%s] Connect error: %v", acc.Email, err)
		return err
	}
	defer c.Logout()

	folders := getFoldersToCheck(acc, c)
	totalUnread := 0
	newNotifications := false

	for _, folder := range folders {
		mbox, err := c.Select(folder, false)
		if err != nil {
			log.Printf("[%s] Select %s error: %v", acc.Email, folder, err)
			continue
		}

		totalUnread += int(mbox.Unseen)

		if mbox.Messages == 0 {
			continue
		}

		criteria := imap.NewSearchCriteria()
		criteria.WithoutFlags = []string{imap.SeenFlag}
		ids, err := c.Search(criteria)
		if err != nil || len(ids) == 0 {
			continue
		}

		seqset := new(imap.SeqSet)
		seqset.AddNum(ids...)

		messages := make(chan *imap.Message, len(ids))
		done := make(chan error, 1)
		go func() {
			done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid}, messages)
		}()

		for msg := range messages {
			if msg.Envelope != nil && msg.Uid > 0 {
				emailID := generateEmailID(folder, msg.Uid, msg.Envelope.MessageId)

				acc.mu.Lock()
				alreadyNotified := acc.notifiedEmails[emailID]
				acc.mu.Unlock()

				if !alreadyNotified && applyFilters(acc, msg.Envelope) {
					showNotification(acc, folder, msg.Envelope)
					acc.mu.Lock()
					acc.notifiedEmails[emailID] = true
					acc.mu.Unlock()
					newNotifications = true
				}
			}
		}
		<-done
	}

	acc.mu.Lock()
	acc.lastCheckTime = time.Now()
	acc.unreadCount = totalUnread
	acc.mu.Unlock()

	if newNotifications {
		saveNotifiedEmails(acc)
	}

	return nil
}

func checkNewEmailsPOP3(acc *AccountConfig) error {
	password, err := getPassword(acc.Email)
	if err != nil {
		log.Printf("%s: Failed to get password: %v", acc.Email, err)
		return err
	}

	p := pop3.New(pop3.Opt{
		Host:       acc.Server,
		Port:       acc.Port,
		TLSEnabled: true,
	})

	c, err := p.NewConn()
	if err != nil {
		log.Printf("%s: POP3 Connect error: %v", acc.Email, err)
		return err
	}
	defer c.Quit()

	if err := c.Auth(acc.Username, password); err != nil {
		log.Printf("%s: POP3 Auth error: %v", acc.Email, err)
		return err
	}

	msgCount, _, err := c.Stat()
	if err != nil {
		log.Printf("%s: POP3 Stat error: %v", acc.Email, err)
		return err
	}

	newNotifications := false
	for i := 1; i <= msgCount; i++ {
		msg, err := c.Retr(i)
		if err != nil {
			continue
		}

		// Read the body from io.Reader
		bodyBytes, err := io.ReadAll(msg.Body)
		if err != nil {
			continue
		}
		bodyStr := string(bodyBytes)

		emailID := fmt.Sprintf("pop3-%d-%s", i, extractMessageID(bodyStr))

		acc.mu.Lock()
		alreadyNotified := acc.notifiedEmails[emailID]
		acc.mu.Unlock()

		if !alreadyNotified {
			from := msg.Header.Get("From")
			subject := msg.Header.Get("Subject")

			if applyFiltersPOP3(acc, from, subject) {
				showNotificationPOP3(acc, from, subject)
				acc.mu.Lock()
				acc.notifiedEmails[emailID] = true
				acc.mu.Unlock()
				newNotifications = true
			}
		}
	}

	acc.mu.Lock()
	acc.lastCheckTime = time.Now()
	acc.unreadCount = msgCount
	acc.mu.Unlock()

	if newNotifications {
		saveNotifiedEmails(acc)
	}

	return nil
}

func testPOP3Connection(server string, port int, username, password string) error {
	// Initialize the POP3 client
	p := pop3.New(pop3.Opt{
		Host:       server,
		Port:       port,
		TLSEnabled: true,
	})

	// Create a new connection
	c, err := p.NewConn()
	if err != nil {
		return err
	}
	defer c.Quit()

	// Authenticate
	if err := c.Auth(username, password); err != nil {
		return err
	}

	// Test with Stat command
	_, _, err = c.Stat()
	return err
}

func connectToIMAP(acc *AccountConfig) (*client.Client, error) {
	password, err := getPassword(acc.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to get password from keyring: %v", err)
	}

	c, err := client.DialTLS(fmt.Sprintf("%s:%d", acc.Server, acc.Port), nil)
	if err != nil {
		return nil, err
	}

	if err := c.Login(acc.Username, password); err != nil {
		c.Logout()
		return nil, err
	}

	return c, nil
}

func getFoldersToCheck(acc *AccountConfig, c *client.Client) []string {
	switch acc.FolderMode {
	case "include":
		return acc.IncludeFolders
	case "exclude":
		allFolders := listFolders(c)
		excludeMap := make(map[string]bool)
		for _, f := range acc.ExcludeFolders {
			excludeMap[f] = true
		}
		var result []string
		for _, f := range allFolders {
			if !excludeMap[f] {
				result = append(result, f)
			}
		}
		return result
	default:
		return listFolders(c)
	}
}

func listFolders(c *client.Client) []string {
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	var folders []string
	for m := range mailboxes {
		folders = append(folders, m.Name)
	}
	<-done

	return folders
}

func applyFilters(acc *AccountConfig, env *imap.Envelope) bool {
	var senderEmail string
	if len(env.From) > 0 && env.From[0].MailboxName != "" && env.From[0].HostName != "" {
		senderEmail = env.From[0].MailboxName + "@" + env.From[0].HostName
	}

	subject := strings.ToLower(env.Subject)

	for _, excludeEmail := range acc.ExcludeEmail {
		if strings.EqualFold(senderEmail, excludeEmail) {
			return false
		}
	}

	for _, keyword := range acc.ExcludeKeyword {
		if strings.Contains(subject, strings.ToLower(keyword)) {
			return false
		}
	}

	hasIncludeFilters := len(acc.IncludeEmail) > 0 || len(acc.IncludeKeyword) > 0

	if hasIncludeFilters {
		if len(acc.IncludeEmail) > 0 {
			for _, includeEmail := range acc.IncludeEmail {
				if strings.EqualFold(senderEmail, includeEmail) {
					return true
				}
			}
		}

		if len(acc.IncludeKeyword) > 0 {
			for _, keyword := range acc.IncludeKeyword {
				if strings.Contains(subject, strings.ToLower(keyword)) {
					return true
				}
			}
		}

		return false
	}

	return true
}

func applyFiltersPOP3(acc *AccountConfig, from, subject string) bool {
	senderEmail := extractEmailAddress(from)
	subjectLower := strings.ToLower(subject)

	for _, excludeEmail := range acc.ExcludeEmail {
		if strings.EqualFold(senderEmail, excludeEmail) {
			return false
		}
	}

	for _, keyword := range acc.ExcludeKeyword {
		if strings.Contains(subjectLower, strings.ToLower(keyword)) {
			return false
		}
	}

	hasIncludeFilters := len(acc.IncludeEmail) > 0 || len(acc.IncludeKeyword) > 0

	if hasIncludeFilters {
		if len(acc.IncludeEmail) > 0 {
			for _, includeEmail := range acc.IncludeEmail {
				if strings.EqualFold(senderEmail, includeEmail) {
					return true
				}
			}
		}

		if len(acc.IncludeKeyword) > 0 {
			for _, keyword := range acc.IncludeKeyword {
				if strings.Contains(subjectLower, strings.ToLower(keyword)) {
					return true
				}
			}
		}

		return false
	}

	return true
}

func showNotification(acc *AccountConfig, folder string, env *imap.Envelope) {
	var sender string
	if len(env.From) > 0 {
		if env.From[0].MailboxName != "" && env.From[0].HostName != "" {
			sender = env.From[0].MailboxName + "@" + env.From[0].HostName
		} else if env.From[0].PersonalName != "" {
			sender = env.From[0].PersonalName
		} else {
			sender = "Unknown"
		}
	} else {
		sender = "Unknown"
	}

	subject := env.Subject
	if subject == "" {
		subject = "(No Subject)"
	}

	displaySubject := subject
	if len(displaySubject) > 50 {
		displaySubject = displaySubject[:47] + "..."
	}

	title := fmt.Sprintf("📧 %s [%s]", acc.Email, folder)
	message := fmt.Sprintf("From: %s\nSubject: %s", sender, displaySubject)

	var err error
	if acc.EnableNotificationSound {
		err = beeep.Notify(title, message, "")
	} else {
		err = beeep.Alert(title, message, "")
	}

	if err != nil {
		log.Printf("[%s] Notification error: %v", acc.Email, err)
	}

	log.Printf("[%s][%s] NEW EMAIL - From: %s | Subject: %s", acc.Email, folder, sender, subject)
}

func showNotificationPOP3(acc *AccountConfig, from, subject string) {
	sender := extractEmailAddress(from)
	if sender == "" {
		sender = from
	}

	if subject == "" {
		subject = "(No Subject)"
	}

	displaySubject := subject
	if len(displaySubject) > 50 {
		displaySubject = displaySubject[:47] + "..."
	}

	title := fmt.Sprintf("📧 %s [POP3]", acc.Email)
	message := fmt.Sprintf("From: %s\nSubject: %s", sender, displaySubject)

	var err error
	if acc.EnableNotificationSound {
		err = beeep.Notify(title, message, "")
	} else {
		err = beeep.Alert(title, message, "")
	}

	if err != nil {
		log.Printf("[%s] Notification error: %v", acc.Email, err)
	}

	log.Printf("[%s][POP3] NEW EMAIL - From: %s | Subject: %s", acc.Email, sender, subject)
}

func extractHeader(msg, header string) string {
	lines := strings.Split(msg, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(header)+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, header+":"))
		}
	}
	return ""
}

func extractMessageID(msg string) string {
	msgID := extractHeader(msg, "Message-ID")
	if msgID != "" {
		return msgID
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func extractEmailAddress(from string) string {
	// Extract email from "Name <email@example.com>" format
	if idx := strings.Index(from, "<"); idx != -1 {
		if endIdx := strings.Index(from[idx:], ">"); endIdx != -1 {
			return from[idx+1 : idx+endIdx]
		}
	}
	// Return as-is if no angle brackets
	return strings.TrimSpace(from)
}

func generateEmailID(folder string, uid uint32, messageID string) string {
	if messageID != "" {
		return fmt.Sprintf("%s-%d-%s", folder, uid, messageID)
	}
	return fmt.Sprintf("%s-%d", folder, uid)
}

func loadNotifiedEmails(acc *AccountConfig) {
	filename := filepath.Join(historyDir, sanitizeFilename(acc.Email)+".json")
	file, err := os.ReadFile(filename)
	if err != nil {
		return
	}

	var emails []string
	if err := json.Unmarshal(file, &emails); err != nil {
		return
	}

	for _, email := range emails {
		acc.notifiedEmails[email] = true
	}
}

func saveNotifiedEmails(acc *AccountConfig) error {
	acc.mu.RLock()
	emails := make([]string, 0, len(acc.notifiedEmails))
	for email := range acc.notifiedEmails {
		emails = append(emails, email)
	}
	acc.mu.RUnlock()

	data, err := json.MarshalIndent(emails, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(historyDir, sanitizeFilename(acc.Email)+".json")
	return os.WriteFile(filename, data, 0644)
}

func cleanupOldNotifications(acc *AccountConfig) {
	if len(acc.notifiedEmails) > acc.CheckHistory {
		log.Printf("[%s] Cleanup history (current: %d, max: %d)", acc.Email, len(acc.notifiedEmails), acc.CheckHistory)
		count := 0
		for k := range acc.notifiedEmails {
			if count > acc.CheckHistory/2 {
				break
			}
			delete(acc.notifiedEmails, k)
			count++
		}
		saveNotifiedEmails(acc)
	}
}

func sanitizeFilename(s string) string {
	return strings.ReplaceAll(s, "@", "_at_")
}

func checkAllAccounts() {
	var wg sync.WaitGroup
	for i := range config.Accounts {
		wg.Add(1)
		go func(acc *AccountConfig) {
			defer wg.Done()
			if acc.Protocol == "pop3" {
				checkNewEmailsPOP3(acc)
			} else {
				checkNewEmails(acc)
			}
		}(&config.Accounts[i])
	}
	wg.Wait()
	beeep.Notify("Email Monitor", "Manual check completed", "")
}

func clearAllHistory() {
	for i := range config.Accounts {
		config.Accounts[i].mu.Lock()
		config.Accounts[i].notifiedEmails = make(map[string]bool)
		config.Accounts[i].mu.Unlock()
		saveNotifiedEmails(&config.Accounts[i])
	}
	beeep.Notify("Email Monitor", "History cleared", "")
}

func restartAllMonitors() {
	for i := range config.Accounts {
		config.Accounts[i].stopChan <- true
		time.Sleep(100 * time.Millisecond)
		config.Accounts[i].stopChan = make(chan bool)
		go startMonitoring(&config.Accounts[i])
	}
	beeep.Notify("Email Monitor", "Monitors restarted", "")
}

func getIconData() []byte {
	iconPath := filepath.Join(appDir, "icon.png")
	icon, err := os.ReadFile(iconPath)
	if err == nil {
		return icon
	}

	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00, 0x16,
		0x08, 0x06, 0x00, 0x00, 0x00, 0xC4, 0xB4, 0x6C, 0x3B, 0x00, 0x00, 0x00,
		0x09, 0x70, 0x48, 0x59, 0x73, 0x00, 0x00, 0x0B, 0x13, 0x00, 0x00, 0x0B,
		0x13, 0x01, 0x00, 0x9A, 0x9C, 0x18, 0x00, 0x00, 0x00, 0xDA, 0x49, 0x44,
		0x41, 0x54, 0x48, 0x4B, 0xED, 0x95, 0x4D, 0x0A, 0x82, 0x40, 0x10, 0x46,
		0xDF, 0x9A, 0xB4, 0x87, 0xE0, 0x0D, 0xBC, 0x86, 0x17, 0xF0, 0x1A, 0x5E,
		0xC2, 0x95, 0xBC, 0x80, 0xE0, 0x0D, 0x3C, 0x80, 0x17, 0x50, 0xA8, 0x20,
		0x08, 0x82, 0x20, 0x08, 0x82, 0x68, 0x30, 0x99, 0x49, 0x67, 0x32, 0x99,
		0xCC, 0x64, 0x32, 0x1F, 0x92, 0x9D, 0xF9, 0x98, 0x37, 0xFB, 0x98, 0xF7,
		0x7E, 0x33, 0xB3, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xF8, 0x5F, 0xE0, 0x7F, 0x81, 0x7F, 0x0D, 0xFC, 0x3B, 0xF0,
		0x9F, 0xC0, 0x3F, 0x07, 0xFE, 0x0B, 0xF8, 0x77, 0xE0, 0x3F, 0x80, 0xFF,
		0x02, 0xFE, 0x13, 0xF8, 0x2F, 0xE0, 0xBF, 0x80, 0xFF, 0x06, 0xFE, 0x0B,
		0xF8, 0x1F, 0xE0, 0x7F, 0x81, 0xFF, 0x03, 0xFE, 0x1F, 0xF8, 0x3F, 0xE0,
		0xFF, 0x81, 0xFF, 0x05, 0xFE, 0x1F, 0xF8, 0x3F, 0xE0, 0xFF, 0x80, 0xFF,
		0x07, 0xFE, 0x0F, 0xF8, 0x7F, 0xE0, 0xFF, 0x80, 0xFF, 0x03, 0xFE, 0x1F,
		0xF8, 0x1F, 0xE0, 0xFF, 0x81, 0xFF, 0x05, 0xFE, 0x0F, 0xF8, 0x7F, 0x80,
		0xF8, 0x01, 0xE0, 0x07, 0x80, 0x1F, 0x00, 0x7E, 0x00, 0xF8, 0x01, 0xE0,
		0x07, 0x80, 0x1F, 0x00, 0x7E, 0x00, 0x78, 0x00, 0xF0, 0x03, 0xC0, 0x0F,
		0x00, 0x3F, 0x00, 0xFC, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xF8, 0x00, 0x35, 0x99, 0x0F, 0x64, 0xBA, 0xDF, 0x42, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}
}

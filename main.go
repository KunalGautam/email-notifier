package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
)

type AccountConfig struct {
	Email                   string   `json:"email"`
	Server                  string   `json:"server"`
	Port                    int      `json:"port"`
	Username                string   `json:"username"`
	Password                string   `json:"password"`
	Protocol                string   `json:"protocol"`
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
)

func init() {
	// Get OS-specific application directory
	var err error
	appDir, err = getAppDir()
	if err != nil {
		log.Fatalf("Failed to get application directory: %v", err)
	}

	// Create application directory if it doesn't exist
	if err := os.MkdirAll(appDir, 0755); err != nil {
		log.Fatalf("Failed to create application directory: %v", err)
	}

	// Set file paths
	configFile = filepath.Join(appDir, "config.json")
	logFile = filepath.Join(appDir, "email-monitor.log")
	foldersListFile = filepath.Join(appDir, "folders_list.json")
	historyDir = filepath.Join(appDir, "notification_history")
}

func getAppDir() (string, error) {
	var baseDir string
	
	// Get OS-specific config directory
	switch {
	case os.Getenv("XDG_CONFIG_HOME") != "":
		// Linux with XDG
		baseDir = os.Getenv("XDG_CONFIG_HOME")
	case os.Getenv("APPDATA") != "":
		// Windows
		baseDir = os.Getenv("APPDATA")
	case os.Getenv("HOME") != "":
		// macOS and Linux fallback
		home := os.Getenv("HOME")
		if _, err := os.Stat(filepath.Join(home, "Library")); err == nil {
			// macOS
			baseDir = filepath.Join(home, "Library", "Application Support")
		} else {
			// Linux fallback
			baseDir = filepath.Join(home, ".config")
		}
	default:
		return "", fmt.Errorf("unable to determine user config directory")
	}

	return filepath.Join(baseDir, "email-monitor"), nil
}

func main() {
	setupLogging()

	log.Printf("Application directory: %s", appDir)
	fmt.Printf("📁 Email Monitor\n")
	fmt.Printf("Application directory: %s\n\n", appDir)

	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}

	os.MkdirAll(historyDir, 0755)

	for i := range config.Accounts {
		config.Accounts[i].notifiedEmails = make(map[string]bool)
		config.Accounts[i].stopChan = make(chan bool)
		loadNotifiedEmails(&config.Accounts[i])
		cleanupOldNotifications(&config.Accounts[i])
	}

	log.Printf("Starting email monitor for %d accounts", len(config.Accounts))

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
		return fmt.Errorf("no accounts configured in config.json")
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
				Password:                "your-password",
				Protocol:                "imap",
				IncludeKeyword:          []string{"urgent", "invoice"},
				ExcludeKeyword:          []string{"promotion", "newsletter"},
				IncludeEmail:            []string{"boss@company.com"},
				ExcludeEmail:            []string{"spam@example.com"},
				CheckInterval:           120,
				CheckHistory:            1000,
				EnableNotificationSound: true,
				FolderMode:              "all",
				IncludeFolders:          []string{"INBOX", "Work"},
				ExcludeFolders:          []string{"Spam", "Trash"},
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
	
	// Show notification with config path
	beeep.Notify("Email Monitor - Setup Required", 
		fmt.Sprintf("Config file created at:\n%s\n\nPlease edit and restart.", configFile), "")
	
	os.Exit(0)
	return nil
}

func onReady() {
	systray.SetIcon(getIconData())
	systray.SetTitle("📧")
	systray.SetTooltip(fmt.Sprintf("Email Monitor - %d accounts", len(config.Accounts)))

	mStatus := systray.AddMenuItem("✅ Status: Running", "Current status")
	mStatus.Disable()

	systray.AddSeparator()

	accountMenuItems = make(map[string]*systray.MenuItem)
	for i := range config.Accounts {
		acc := &config.Accounts[i]
		menuText := fmt.Sprintf("📧 %s - Checking...", acc.Email)
		mAccount := systray.AddMenuItem(menuText, fmt.Sprintf("Account: %s", acc.Email))
		mAccount.Disable()
		accountMenuItems[acc.Email] = mAccount
	}

	systray.AddSeparator()

	mCheckAll := systray.AddMenuItem("🔍 Check All Now", "Check all accounts immediately")
	mClearHistory := systray.AddMenuItem("🗑️  Clear All History", "Clear notification history for all accounts")
	
	mFolders := systray.AddMenuItem("📁 Folder Management", "Manage email folders")
	mListFolders := mFolders.AddSubMenuItem("📋 List All Folders", "Get list of all folders from all accounts")
	mViewFolderList := mFolders.AddSubMenuItem("👁️  View Saved Folder List", "View previously saved folder list")
	
	mRestart := systray.AddMenuItem("🔄 Restart Monitor", "Restart email monitoring")
	mReloadConfig := systray.AddMenuItem("⚙️  Reload Config", "Reload configuration file")

	systray.AddSeparator()

	mViewFiles := systray.AddMenuItem("📂 Open App Directory", "Open application directory in file manager")
	mViewLogs := systray.AddMenuItem("📄 View File Locations", "Show file paths")
	mExit := systray.AddMenuItem("❌ Exit", "Exit the application")

	for i := range config.Accounts {
		go startMonitoring(&config.Accounts[i])
	}

	go func() {
		for {
			select {
			case <-mCheckAll.ClickedCh:
				log.Println("Manual check all triggered")
				checkAllAccounts()

			case <-mClearHistory.ClickedCh:
				log.Println("Clearing all notification history")
				clearAllHistory()

			case <-mListFolders.ClickedCh:
				log.Println("Listing all folders")
				go listAllFolders()

			case <-mViewFolderList.ClickedCh:
				log.Println("Viewing saved folder list")
				viewSavedFolderList()

			case <-mRestart.ClickedCh:
				log.Println("Restarting all monitors...")
				restartAllMonitors()

			case <-mReloadConfig.ClickedCh:
				log.Println("Reloading configuration...")
				reloadConfiguration()

			case <-mViewFiles.ClickedCh:
				openAppDirectory()

			case <-mViewLogs.ClickedCh:
				showFilePaths()

			case <-mExit.ClickedCh:
				log.Println("Exiting application...")
				systray.Quit()
				return
			}
		}
	}()
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

func startMonitoring(acc *AccountConfig) {
	log.Printf("Monitor started for: %s (interval: %ds, mode: %s)", acc.Email, acc.CheckInterval, acc.FolderMode)

	acc.ticker = time.NewTicker(time.Duration(acc.CheckInterval) * time.Second)
	defer acc.ticker.Stop()

	if err := checkNewEmails(acc); err != nil {
		log.Printf("[%s] Error: %v", acc.Email, err)
	}

	for {
		select {
		case <-acc.ticker.C:
			if err := checkNewEmails(acc); err != nil {
				log.Printf("[%s] Error: %v", acc.Email, err)
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
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer c.Logout()

	folders := getFoldersToCheck(acc, c)
	
	totalUnread := 0
	newNotifications := false

	for _, folder := range folders {
		mbox, err := c.Select(folder, false)
		if err != nil {
			log.Printf("[%s] Failed to select folder %s: %v", acc.Email, folder, err)
			continue
		}

		totalUnread += int(mbox.Unseen)

		if mbox.Messages == 0 {
			continue
		}

		criteria := imap.NewSearchCriteria()
		criteria.WithoutFlags = []string{imap.SeenFlag}
		ids, err := c.Search(criteria)
		if err != nil {
			log.Printf("[%s] Failed to search in %s: %v", acc.Email, folder, err)
			continue
		}

		if len(ids) == 0 {
			continue
		}

		seqset := new(imap.SeqSet)
		seqset.AddNum(ids...)

		messages := make(chan *imap.Message, len(ids))
		section := &imap.BodySectionName{
			BodyPartName: imap.BodyPartName{},
			Peek:         true,
		}

		items := []imap.FetchItem{
			imap.FetchEnvelope,
			imap.FetchUid,
			section.FetchItem(),
		}

		done := make(chan error, 1)
		go func() {
			done <- c.Fetch(seqset, items, messages)
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

		if err := <-done; err != nil {
			log.Printf("[%s] Failed to fetch from %s: %v", acc.Email, folder, err)
		}
	}

	acc.mu.Lock()
	acc.lastCheckTime = time.Now()
	acc.unreadCount = totalUnread
	acc.mu.Unlock()
	updateAccountMenuItem(acc)

	if newNotifications {
		saveNotifiedEmails(acc)
	}

	return nil
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

	if err := <-done; err != nil {
		log.Printf("Error listing folders: %v", err)
	}

	return folders
}

func listAllFolders() {
	type FolderInfo struct {
		Account string   `json:"account"`
		Folders []string `json:"folders"`
	}

	var allFolders []FolderInfo

	for i := range config.Accounts {
		acc := &config.Accounts[i]
		c, err := connectToIMAP(acc)
		if err != nil {
			log.Printf("[%s] Failed to connect: %v", acc.Email, err)
			continue
		}

		folders := listFolders(c)
		c.Logout()

		allFolders = append(allFolders, FolderInfo{
			Account: acc.Email,
			Folders: folders,
		})

		log.Printf("[%s] Found %d folders", acc.Email, len(folders))
	}

	data, err := json.MarshalIndent(allFolders, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal folder list: %v", err)
		beeep.Alert("Email Monitor", "Failed to save folder list", "")
		return
	}

	if err := os.WriteFile(foldersListFile, data, 0644); err != nil {
		log.Printf("Failed to write folder list: %v", err)
		beeep.Alert("Email Monitor", "Failed to save folder list", "")
		return
	}

	message := fmt.Sprintf("Folder list saved to:\n%s\n\nTotal accounts: %d", foldersListFile, len(allFolders))
	beeep.Notify("📁 Folder List Saved", message, "")
}

func viewSavedFolderList() {
	if _, err := os.Stat(foldersListFile); os.IsNotExist(err) {
		beeep.Alert("Email Monitor", "No saved folder list found.\nUse 'List All Folders' first.", "")
		return
	}

	data, err := os.ReadFile(foldersListFile)
	if err != nil {
		beeep.Alert("Email Monitor", fmt.Sprintf("Failed to read folder list: %v", err), "")
		return
	}

	type FolderInfo struct {
		Account string   `json:"account"`
		Folders []string `json:"folders"`
	}

	var allFolders []FolderInfo
	if err := json.Unmarshal(data, &allFolders); err != nil {
		beeep.Alert("Email Monitor", fmt.Sprintf("Failed to parse folder list: %v", err), "")
		return
	}

	message := fmt.Sprintf("Folder list location:\n%s\n\n", foldersListFile)
	
	for _, info := range allFolders {
		message += fmt.Sprintf("%s (%d folders):\n", info.Account, len(info.Folders))
		for i, folder := range info.Folders {
			if i >= 5 {
				message += fmt.Sprintf("  ... and %d more\n", len(info.Folders)-5)
				break
			}
			message += fmt.Sprintf("  • %s\n", folder)
		}
		message += "\n"
	}

	beeep.Notify("📁 Saved Folder List", message, "")
}

func showFilePaths() {
	message := fmt.Sprintf(
		"Application Directory:\n%s\n\nConfig: %s\n\nLog: %s\n\nFolder List: %s\n\nHistory: %s",
		appDir,
		configFile,
		logFile,
		foldersListFile,
		historyDir,
	)

	beeep.Notify("📂 File Locations", message, "")
}

func openAppDirectory() {
	// This will show a notification with the path
	// Users can manually open it from there
	message := fmt.Sprintf("Application directory:\n%s\n\nOpen this location in your file manager.", appDir)
	beeep.Notify("📂 Application Directory", message, "")
	log.Printf("App directory requested: %s", appDir)
}

func connectToIMAP(acc *AccountConfig) (*client.Client, error) {
	c, err := client.DialTLS(fmt.Sprintf("%s:%d", acc.Server, acc.Port), nil)
	if err != nil {
		return nil, err
	}

	if err := c.Login(acc.Username, acc.Password); err != nil {
		c.Logout()
		return nil, err
	}

	return c, nil
}

func applyFilters(acc *AccountConfig, env *imap.Envelope) bool {
	var senderEmail string
	if len(env.From) > 0 && env.From[0].MailboxName != "" && env.From[0].HostName != "" {
		senderEmail = env.From[0].MailboxName + "@" + env.From[0].HostName
	}

	subject := strings.ToLower(env.Subject)

	for _, excludeEmail := range acc.ExcludeEmail {
		if strings.EqualFold(senderEmail, excludeEmail) {
			log.Printf("[%s] Filtered out (exclude email): %s", acc.Email, senderEmail)
			return false
		}
	}

	for _, keyword := range acc.ExcludeKeyword {
		if strings.Contains(subject, strings.ToLower(keyword)) {
			log.Printf("[%s] Filtered out (exclude keyword '%s'): %s", acc.Email, keyword, subject)
			return false
		}
	}

	hasIncludeFilters := len(acc.IncludeEmail) > 0 || len(acc.IncludeKeyword) > 0

	if hasIncludeFilters {
		if len(acc.IncludeEmail) > 0 {
			emailMatch := false
			for _, includeEmail := range acc.IncludeEmail {
				if strings.EqualFold(senderEmail, includeEmail) {
					emailMatch = true
					break
				}
			}
			if emailMatch {
				return true
			}
		}

		if len(acc.IncludeKeyword) > 0 {
			for _, keyword := range acc.IncludeKeyword {
				if strings.Contains(subject, strings.ToLower(keyword)) {
					return true
				}
			}
		}

		log.Printf("[%s] Filtered out (no include match): %s - %s", acc.Email, senderEmail, subject)
		return false
	}

	return true
}

func updateAccountMenuItem(acc *AccountConfig) {
	mu.Lock()
	defer mu.Unlock()

	if item, exists := accountMenuItems[acc.Email]; exists {
		acc.mu.RLock()
		menuText := fmt.Sprintf("📧 %s - Unread: %d (Last: %s)",
			acc.Email,
			acc.unreadCount,
			acc.lastCheckTime.Format("15:04:05"))
		acc.mu.RUnlock()
		item.SetTitle(menuText)
	}
}

func checkAllAccounts() {
	var wg sync.WaitGroup
	for i := range config.Accounts {
		wg.Add(1)
		go func(acc *AccountConfig) {
			defer wg.Done()
			if err := checkNewEmails(acc); err != nil {
				log.Printf("[%s] Manual check error: %v", acc.Email, err)
			}
		}(&config.Accounts[i])
	}
	wg.Wait()
	beeep.Notify("Email Monitor", "Manual check completed for all accounts", "")
}

func clearAllHistory() {
	for i := range config.Accounts {
		config.Accounts[i].mu.Lock()
		config.Accounts[i].notifiedEmails = make(map[string]bool)
		config.Accounts[i].mu.Unlock()
		saveNotifiedEmails(&config.Accounts[i])
	}
	beeep.Notify("Email Monitor", "All notification history cleared", "")
}

func restartAllMonitors() {
	for i := range config.Accounts {
		config.Accounts[i].stopChan <- true
		time.Sleep(100 * time.Millisecond)
		config.Accounts[i].stopChan = make(chan bool)
		go startMonitoring(&config.Accounts[i])
	}
	beeep.Notify("Email Monitor", "All monitors restarted", "")
}

func reloadConfiguration() {
	if err := loadConfig(); err != nil {
		beeep.Alert("Email Monitor", fmt.Sprintf("Failed to reload config: %v", err), "")
		return
	}
	restartAllMonitors()
	beeep.Notify("Email Monitor", "Configuration reloaded", "")
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
		if !os.IsNotExist(err) {
			log.Printf("[%s] Warning: failed to load notified emails: %v", acc.Email, err)
		}
		return
	}

	var emails []string
	if err := json.Unmarshal(file, &emails); err != nil {
		log.Printf("[%s] Warning: failed to parse notified emails: %v", acc.Email, err)
		return
	}

	for _, email := range emails {
		acc.notifiedEmails[email] = true
	}
}

func cleanupOldNotifications(acc *AccountConfig) {
	if len(acc.notifiedEmails) > acc.CheckHistory {
		log.Printf("[%s] Cleaning up history (current: %d, max: %d)", acc.Email, len(acc.notifiedEmails), acc.CheckHistory)
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

func sanitizeFilename(s string) string {
	return strings.ReplaceAll(s, "@", "_at_")
}

func showNotification(acc *AccountConfig, folder string, env *imap.Envelope) {
	var sender string
	if len(env.From) > 0 {
		if env.From[0].MailboxName != "" && env.From[0].HostName != "" {
			sender = env.From[0].MailboxName + "@" + env.From[0].HostName
		} else if env.From[0].PersonalName != "" {
			sender = env.From[0].PersonalName
		} else {
			sender = "Unknown Sender"
		}
	} else {
		sender = "Unknown Sender"
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
		log.Printf("[%s] Failed to send notification: %v", acc.Email, err)
	}

	log.Printf("[%s][%s] NEW EMAIL - From: %s | Subject: %s", acc.Email, folder, sender, subject)
	fmt.Printf("\n[%s][%s] 📧 NEW EMAIL\n", acc.Email, folder)
	fmt.Printf("From: %s\nSubject: %s\nTime: %s\n\n", sender, subject, time.Now().Format("15:04:05"))
}

func getIconData() []byte {
	iconPath := filepath.Join(appDir, "icon.png")
	icon, err := os.ReadFile(iconPath)
	if err == nil {
		return icon
	}

	return []byte{
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
}89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00, 0x16,
		0x08, 0x06, 0x00, 0x00, 0x00, 0xC4, 0xB4, 0x6C, 0x3B, 0x00, 0x00, 0x00,
		0x09, 0x70, 0x48, 0x59, 0x73, 0x00, 0x00, 0x0B, 0x13, 0x00, 0x00, 0x0B,
		0x13, 0x01, 0x00, 0x9A, 0x9C, 0x18, 0x00, 0x00, 0x00, 0xDA, 0x49, 0x44,
		0x41, 0x54, 0x48, 0x4B, 0xED, 0x95, 0x4D, 0x0A, 0x82, 0x40, 0x10, 0x46,
		0x
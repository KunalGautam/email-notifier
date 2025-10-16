package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
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

const (
	configFile        = "config.json"
	notifiedEmailsDir = "notification_history"
	logFile           = "email-monitor.log"
)

var (
	config           Config
	accountMenuItems map[string]*systray.MenuItem
	mu               sync.RWMutex
	connectionPool   sync.Map // Connection pool for reusing IMAP connections
)

func main() {
	// Setup logging
	setupLogging()

	// Load or create config
	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}

	// Create notification history directory
	os.MkdirAll(notifiedEmailsDir, 0755)

	// Load notification history for all accounts
	for i := range config.Accounts {
		config.Accounts[i].notifiedEmails = make(map[string]bool)
		config.Accounts[i].stopChan = make(chan bool)
		loadNotifiedEmails(&config.Accounts[i])
		cleanupOldNotifications(&config.Accounts[i])
	}

	log.Printf("Starting email monitor for %d accounts", len(config.Accounts))

	// Start system tray
	systray.Run(onReady, onExit)
}

func setupLogging() {
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(f)
	}
}

func loadConfig() error {
	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Create sample config
		return createSampleConfig()
	}

	// Read config file
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse JSON
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}

	// Validate accounts
	if len(config.Accounts) == 0 {
		return fmt.Errorf("no accounts configured in config.json")
	}

	// Set defaults
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
	fmt.Printf("Sample config created: %s\nPlease edit it with your email settings and restart.\n", configFile)
	os.Exit(0)
	return nil
}

func onReady() {
	systray.SetIcon(getIconData())
	systray.SetTitle("📧")
	systray.SetTooltip(fmt.Sprintf("Email Monitor - %d accounts", len(config.Accounts)))

	// Status
	mStatus := systray.AddMenuItem("✅ Status: Running", "Current status")
	mStatus.Disable()

	systray.AddSeparator()

	// Account menu items
	accountMenuItems = make(map[string]*systray.MenuItem)
	for i := range config.Accounts {
		acc := &config.Accounts[i]
		menuText := fmt.Sprintf("📧 %s - Checking...", acc.Email)
		mAccount := systray.AddMenuItem(menuText, fmt.Sprintf("Account: %s", acc.Email))
		mAccount.Disable()
		accountMenuItems[acc.Email] = mAccount
	}

	systray.AddSeparator()

	// Actions
	mCheckAll := systray.AddMenuItem("🔍 Check All Now", "Check all accounts immediately")
	mClearHistory := systray.AddMenuItem("🗑️  Clear All History", "Clear notification history for all accounts")
	mRestart := systray.AddMenuItem("🔄 Restart Monitor", "Restart email monitoring")
	mReloadConfig := systray.AddMenuItem("⚙️  Reload Config", "Reload configuration file")

	systray.AddSeparator()

	mViewLogs := systray.AddMenuItem("📄 View Logs", "Open log file")
	mExit := systray.AddMenuItem("❌ Exit", "Exit the application")

	// Start monitoring all accounts
	for i := range config.Accounts {
		go startMonitoring(&config.Accounts[i])
	}

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mCheckAll.ClickedCh:
				log.Println("Manual check all triggered")
				checkAllAccounts()

			case <-mClearHistory.ClickedCh:
				log.Println("Clearing all notification history")
				clearAllHistory()

			case <-mRestart.ClickedCh:
				log.Println("Restarting all monitors...")
				restartAllMonitors()

			case <-mReloadConfig.ClickedCh:
				log.Println("Reloading configuration...")
				reloadConfiguration()

			case <-mViewLogs.ClickedCh:
				beeep.Notify("Email Monitor", fmt.Sprintf("Log file: %s", logFile), "")

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
	log.Printf("Monitor started for: %s (interval: %ds)", acc.Email, acc.CheckInterval)

	acc.ticker = time.NewTicker(time.Duration(acc.CheckInterval) * time.Second)
	defer acc.ticker.Stop()

	// Check immediately on start
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
	// Connect to IMAP server with timeout
	c, err := connectWithTimeout(acc)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer c.Logout()

	// Select INBOX
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		return fmt.Errorf("failed to select INBOX: %v", err)
	}

	// Update status
	acc.mu.Lock()
	acc.lastCheckTime = time.Now()
	acc.unreadCount = int(mbox.Unseen)
	acc.mu.Unlock()
	updateAccountMenuItem(acc)

	if mbox.Messages == 0 {
		return nil
	}

	// Fetch only unseen messages
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	ids, err := c.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search: %v", err)
	}

	if len(ids) == 0 {
		return nil
	}

	// Fetch headers
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

	// Process messages with filters
	newNotifications := false
	for msg := range messages {
		if msg.Envelope != nil && msg.Uid > 0 {
			emailID := generateEmailID(msg.Uid, msg.Envelope.MessageId)

			acc.mu.Lock()
			alreadyNotified := acc.notifiedEmails[emailID]
			acc.mu.Unlock()

			if !alreadyNotified && applyFilters(acc, msg.Envelope) {
				showNotification(acc, msg.Envelope)
				acc.mu.Lock()
				acc.notifiedEmails[emailID] = true
				acc.mu.Unlock()
				newNotifications = true
			}
		}
	}

	if err := <-done; err != nil {
		return fmt.Errorf("failed to fetch: %v", err)
	}

	if newNotifications {
		saveNotifiedEmails(acc)
	}

	return nil
}

func connectWithTimeout(acc *AccountConfig) (*client.Client, error) {
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
	// Get sender email
	var senderEmail string
	if len(env.From) > 0 && env.From[0].MailboxName != "" && env.From[0].HostName != "" {
		senderEmail = env.From[0].MailboxName + "@" + env.From[0].HostName
	}

	subject := strings.ToLower(env.Subject)

	// Check exclude email list first
	for _, excludeEmail := range acc.ExcludeEmail {
		if strings.EqualFold(senderEmail, excludeEmail) {
			log.Printf("[%s] Filtered out (exclude email): %s", acc.Email, senderEmail)
			return false
		}
	}

	// Check exclude keywords
	for _, keyword := range acc.ExcludeKeyword {
		if strings.Contains(subject, strings.ToLower(keyword)) {
			log.Printf("[%s] Filtered out (exclude keyword '%s'): %s", acc.Email, keyword, subject)
			return false
		}
	}

	// If include lists are not empty, check them
	hasIncludeFilters := len(acc.IncludeEmail) > 0 || len(acc.IncludeKeyword) > 0

	if hasIncludeFilters {
		// Check include email list
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

		// Check include keywords
		if len(acc.IncludeKeyword) > 0 {
			for _, keyword := range acc.IncludeKeyword {
				if strings.Contains(subject, strings.ToLower(keyword)) {
					return true
				}
			}
		}

		// If include filters exist but none matched, filter out
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

func generateEmailID(uid uint32, messageID string) string {
	if messageID != "" {
		return fmt.Sprintf("%d-%s", uid, messageID)
	}
	return fmt.Sprintf("%d", uid)
}

func loadNotifiedEmails(acc *AccountConfig) {
	filename := fmt.Sprintf("%s/%s.json", notifiedEmailsDir, sanitizeFilename(acc.Email))
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

	filename := fmt.Sprintf("%s/%s.json", notifiedEmailsDir, sanitizeFilename(acc.Email))
	return os.WriteFile(filename, data, 0644)
}

func sanitizeFilename(s string) string {
	return strings.ReplaceAll(s, "@", "_at_")
}

func showNotification(acc *AccountConfig, env *imap.Envelope) {
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

	// Truncate long subjects
	displaySubject := subject
	if len(displaySubject) > 50 {
		displaySubject = displaySubject[:47] + "..."
	}

	title := fmt.Sprintf("📧 %s", acc.Email)
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

	log.Printf("[%s] NEW EMAIL - From: %s | Subject: %s", acc.Email, sender, subject)
	fmt.Printf("\n[%s] 📧 NEW EMAIL\n", acc.Email)
	fmt.Printf("From: %s\nSubject: %s\nTime: %s\n\n", sender, subject, time.Now().Format("15:04:05"))
}

func getIconData() []byte {
	icon, err := os.ReadFile("icon.png")
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

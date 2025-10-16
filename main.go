package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
	"github.com/joho/godotenv"
)

type EmailConfig struct {
	Server            string
	Port              string
	Email             string
	Password          string
	CheckInterval     int
	MaxHistory        int
	NotificationSound bool
}

const notifiedEmailsFile = "notified_emails.json"

// Track notified emails using a map
var notifiedEmails = make(map[string]bool)
var config EmailConfig
var stopChan chan bool
var ticker *time.Ticker
var mUnreadCount *systray.MenuItem
var mLastCheck *systray.MenuItem
var lastCheckTime time.Time
var unreadCount int

func main() {
	// Setup logging
	logFile, err := os.OpenFile("email-monitor.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	// Load .env file
	err = godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Read configuration from environment variables with defaults
	checkInterval, _ := strconv.Atoi(os.Getenv("CHECK_INTERVAL"))
	if checkInterval == 0 {
		checkInterval = 30
	}

	maxHistory, _ := strconv.Atoi(os.Getenv("MAX_HISTORY"))
	if maxHistory == 0 {
		maxHistory = 1000
	}

	notificationSound := os.Getenv("NOTIFICATION_SOUND") != "false"

	config = EmailConfig{
		Server:            os.Getenv("IMAP_SERVER"),
		Port:              os.Getenv("IMAP_PORT"),
		Email:             os.Getenv("EMAIL"),
		Password:          os.Getenv("PASSWORD"),
		CheckInterval:     checkInterval,
		MaxHistory:        maxHistory,
		NotificationSound: notificationSound,
	}

	// Validate configuration
	if config.Server == "" || config.Port == "" || config.Email == "" || config.Password == "" {
		log.Fatal("Missing required configuration. Please check your .env file")
	}

	// Load and cleanup notified emails
	loadNotifiedEmails()
	cleanupOldNotifications()

	log.Printf("Starting email monitor for: %s (check interval: %ds)", config.Email, config.CheckInterval)

	// Start system tray
	systray.Run(onReady, onExit)
}

func onReady() {
	// Set icon
	systray.SetIcon(getIconData())
	systray.SetTitle("📧")
	systray.SetTooltip(fmt.Sprintf("Email Monitor - %s", config.Email))

	// Add menu items
	mStatus := systray.AddMenuItem("✅ Status: Running", "Current status")
	mStatus.Disable()

	mUnreadCount = systray.AddMenuItem("📬 Unread: Checking...", "Unread email count")
	mUnreadCount.Disable()

	mLastCheck = systray.AddMenuItem("🕐 Last Check: Never", "Last check time")
	mLastCheck.Disable()

	systray.AddSeparator()

	mEmail := systray.AddMenuItem(fmt.Sprintf("📧 %s", config.Email), "Monitored email")
	mEmail.Disable()

	mInterval := systray.AddMenuItem(fmt.Sprintf("⏱️  Interval: %ds", config.CheckInterval), "Check interval")
	mInterval.Disable()

	systray.AddSeparator()

	mCheckNow := systray.AddMenuItem("🔍 Check Now", "Check for new emails immediately")
	mClearHistory := systray.AddMenuItem("🗑️  Clear History", "Clear notification history")
	mRestart := systray.AddMenuItem("🔄 Restart Monitor", "Restart email monitoring")

	systray.AddSeparator()

	mViewLogs := systray.AddMenuItem("📄 View Logs", "Open log file")
	mExit := systray.AddMenuItem("❌ Exit", "Exit the application")

	// Initialize channel
	stopChan = make(chan bool)

	// Start monitoring
	go startMonitoring()

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mCheckNow.ClickedCh:
				log.Println("Manual check triggered")
				go func() {
					if err := checkNewEmails(config); err != nil {
						log.Printf("Error during manual check: %v", err)
						beeep.Alert("Email Monitor", fmt.Sprintf("Check failed: %v", err), "")
					} else {
						beeep.Notify("Email Monitor", "Manual check completed", "")
					}
				}()

			case <-mClearHistory.ClickedCh:
				log.Println("Clearing notification history")
				notifiedEmails = make(map[string]bool)
				if err := saveNotifiedEmails(); err != nil {
					log.Printf("Error clearing history: %v", err)
					beeep.Alert("Email Monitor", "Failed to clear history", "")
				} else {
					beeep.Notify("Email Monitor", "Notification history cleared", "")
				}

			case <-mRestart.ClickedCh:
				log.Println("Restarting monitor...")
				beeep.Notify("Email Monitor", "Restarting monitor...", "")
				stopChan <- true
				time.Sleep(1 * time.Second)
				go startMonitoring()

			case <-mViewLogs.ClickedCh:
				// Try to open log file with default text editor
				log.Println("Opening log file")
				// This is basic - you could use exec.Command to open with xdg-open/open
				beeep.Notify("Email Monitor", "Log file: email-monitor.log", "")

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
	if stopChan != nil {
		close(stopChan)
	}
	if ticker != nil {
		ticker.Stop()
	}
}

func startMonitoring() {
	log.Printf("Monitor started for: %s", config.Email)
	beeep.Notify("📧 Email Monitor", fmt.Sprintf("Monitoring: %s", config.Email), "")

	ticker = time.NewTicker(time.Duration(config.CheckInterval) * time.Second)
	defer ticker.Stop()

	// Check immediately on start
	if err := checkNewEmails(config); err != nil {
		log.Printf("Error checking emails: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := checkNewEmails(config); err != nil {
				log.Printf("Error checking emails: %v", err)
			}
		case <-stopChan:
			log.Println("Monitor stopped")
			return
		}
	}
}

func checkNewEmails(config EmailConfig) error {
	// Connect to IMAP server
	c, err := client.DialTLS(config.Server+":"+config.Port, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer c.Logout()

	// Login
	if err := c.Login(config.Email, config.Password); err != nil {
		return fmt.Errorf("failed to login: %v", err)
	}

	// Select INBOX
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		return fmt.Errorf("failed to select INBOX: %v", err)
	}

	// Update last check time and unread count
	lastCheckTime = time.Now()
	unreadCount = int(mbox.Unseen)
	updateMenuItems()

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

	// Fetch headers for unseen messages
	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	messages := make(chan *imap.Message, len(ids))
	section := &imap.BodySectionName{
		BodyPartName: imap.BodyPartName{},
		Peek:         true,
	}

	// Fetch envelope and UID
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchUid,
		section.FetchItem(),
	}

	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	// Process messages
	newNotifications := false
	for msg := range messages {
		if msg.Envelope != nil && msg.Uid > 0 {
			// Create unique identifier using UID and Message-ID
			emailID := generateEmailID(msg.Uid, msg.Envelope.MessageId)

			// Check if already notified
			if !notifiedEmails[emailID] {
				showNotification(msg.Envelope)
				notifiedEmails[emailID] = true
				newNotifications = true
			}
		}
	}

	if err := <-done; err != nil {
		return fmt.Errorf("failed to fetch: %v", err)
	}

	// Save to file if there were new notifications
	if newNotifications {
		if err := saveNotifiedEmails(); err != nil {
			log.Printf("Warning: failed to save notified emails: %v", err)
		}
	}

	return nil
}

func updateMenuItems() {
	if mUnreadCount != nil {
		mUnreadCount.SetTitle(fmt.Sprintf("📬 Unread: %d", unreadCount))
	}
	if mLastCheck != nil {
		mLastCheck.SetTitle(fmt.Sprintf("🕐 Last Check: %s", lastCheckTime.Format("15:04:05")))
	}
}

func generateEmailID(uid uint32, messageID string) string {
	if messageID != "" {
		return fmt.Sprintf("%d-%s", uid, messageID)
	}
	return fmt.Sprintf("%d", uid)
}

func loadNotifiedEmails() {
	file, err := os.ReadFile(notifiedEmailsFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No previous notification history found, starting fresh")
			return
		}
		log.Printf("Warning: failed to load notified emails: %v", err)
		return
	}

	var emails []string
	if err := json.Unmarshal(file, &emails); err != nil {
		log.Printf("Warning: failed to parse notified emails file: %v", err)
		return
	}

	for _, email := range emails {
		notifiedEmails[email] = true
	}
}

func cleanupOldNotifications() {
	if len(notifiedEmails) > config.MaxHistory {
		log.Printf("Cleaning up notification history (current: %d, max: %d)", len(notifiedEmails), config.MaxHistory)
		// Keep only the most recent entries (simple approach - clear half)
		count := 0
		for k := range notifiedEmails {
			if count > config.MaxHistory/2 {
				break
			}
			delete(notifiedEmails, k)
			count++
		}
		saveNotifiedEmails()
	}
}

func saveNotifiedEmails() error {
	emails := make([]string, 0, len(notifiedEmails))
	for email := range notifiedEmails {
		emails = append(emails, email)
	}

	data, err := json.MarshalIndent(emails, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal emails: %v", err)
	}

	if err := os.WriteFile(notifiedEmailsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	return nil
}

func showNotification(env *imap.Envelope) {
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

	// Truncate long subjects for notification
	if len(subject) > 50 {
		subject = subject[:47] + "..."
	}

	message := fmt.Sprintf("From: %s\nSubject: %s", sender, subject)

	// Send system notification
	var err error
	if config.NotificationSound {
		err = beeep.Notify("📧 New Email", message, "")
	} else {
		err = beeep.Alert("📧 New Email", message, "")
	}

	if err != nil {
		log.Printf("Failed to send notification: %v", err)
	}

	// Log to console and file
	log.Printf("NEW EMAIL - From: %s | Subject: %s", sender, subject)
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📧 NEW EMAIL NOTIFICATION")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("From: %s\n", sender)
	fmt.Printf("Subject: %s\n", env.Subject)
	fmt.Printf("Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

func getIconData() []byte {
	icon, err := os.ReadFile("icon.png")
	if err == nil {
		return icon
	}

	// Simple mail icon
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

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	mathrand "math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Client struct {
	conn     *websocket.Conn
	username string
	key      []byte // Each client gets their own encryption key
}

type Room struct {
	clients map[*Client]bool
	mutex   sync.Mutex
}

type Command struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CommandResponse struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// Add this struct for bot users
type Bot struct {
	name string
	room *Room
}

// Create a bot for financial commands
var financeBot = &Bot{name: "FinanceBot 🤖"}

func init() {
	mathrand.Seed(time.Now().UnixNano())
}

func generateKey() []byte {
	key := make([]byte, 32) // AES-256 requires 32 bytes
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		log.Fatal(err)
	}
	return key
}

func encrypt(text string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("invalid key size: must be 32 bytes")
	}

	plaintext := []byte(text)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt(encrypted string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("invalid key size: must be 32 bytes")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := 12
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func NewRoom() *Room {
	return &Room{
		clients: make(map[*Client]bool),
	}
}

func calculateSavings() string {
	monthlyAmount := 900 + mathrand.Intn(7101) // 8000 - 900 + 1 = 7101
	yearlyAmount := monthlyAmount * 12
	tenYearAmount := yearlyAmount * 10

	// Format numbers with thousand separators
	return fmt.Sprintf("💰 Financial Tip: If you save %s kr per month, you'll have %s kr in 10 years!",
		formatNumber(monthlyAmount),
		formatNumber(tenYearAmount))
}

func formatNumber(n int) string {
	// Convert to string and reverse for easier processing
	str := strconv.Itoa(n)
	var result []byte

	for i := len(str) - 1; i >= 0; i-- {
		if len(result) > 0 && (len(str)-i-1)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, str[i])
	}

	// Reverse back
	for i := 0; i < len(result)/2; i++ {
		result[i], result[len(result)-1-i] = result[len(result)-1-i], result[i]
	}

	return string(result)
}

func (b *Bot) SendMessage(message string) {
	if b.room != nil {
		botMessage := fmt.Sprintf("%s: %s", b.name, message)
		log.Printf("Bot sending message: %s", botMessage)
		// Send directly to all clients
		for client := range b.room.clients {
			err := client.conn.WriteMessage(websocket.TextMessage, []byte(botMessage))
			if err != nil {
				log.Printf("Error sending bot message: %v", err)
			}
		}
	} else {
		log.Printf("Error: Bot has no room assigned")
	}
}

func (room *Room) broadcast(message []byte, sender *Client) {
	room.mutex.Lock()
	defer room.mutex.Unlock()

	messageStr := string(message)
	log.Printf("Broadcasting message: %s", messageStr)

	// Assign room to bot if not set
	if financeBot.room == nil {
		log.Printf("Assigning room to bot")
		financeBot.room = room
	}

	// Check if message is a command
	if strings.HasPrefix(messageStr, "/") {
		log.Printf("Command detected")
		command := strings.TrimPrefix(messageStr, "/")
		command = strings.TrimSpace(command)
		log.Printf("Processing command: %s", command)

		switch command {
		case "saving":
			log.Printf("Processing saving command")
			financeBot.SendMessage(calculateSavings())
			return
		default:
			financeBot.SendMessage("Unknown command. Available commands: /saving")
			return
		}
	}

	// If the message starts with username:, it's a regular message
	if strings.Contains(messageStr, ": /") {
		parts := strings.SplitN(messageStr, ": /", 2)
		if len(parts) == 2 {
			command := strings.TrimSpace(parts[1])
			log.Printf("Processing command from chat: %s", command)

			switch command {
			case "saving":
				log.Printf("Processing saving command")
				financeBot.SendMessage(calculateSavings())
				return
			default:
				financeBot.SendMessage("Unknown command. Available commands: /saving")
				return
			}
		}
	}

	// Skip broadcasting if no sender (used for system/bot messages)
	if sender == nil {
		log.Printf("Broadcasting system/bot message: %s", messageStr)
		for client := range room.clients {
			err := client.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Printf("Error sending system message: %v", err)
			}
		}
		return
	}

	originalMsg := strings.TrimPrefix(messageStr, sender.username+": ")

	if strings.HasPrefix(originalMsg, "@") {
		parts := strings.SplitN(originalMsg[1:], " ", 2)
		if len(parts) == 2 {
			targetUsername := parts[0]
			privateMessage := parts[1]

			// Encrypt private message with sender's key
			encryptedMsg, err := encrypt(privateMessage, sender.key)
			if err != nil {
				log.Printf("Encryption error: %v", err)
				return
			}

			for client := range room.clients {
				if client.username == targetUsername {
					// Re-encrypt message with recipient's key
					reEncryptedMsg, err := encrypt(privateMessage, client.key)
					if err != nil {
						log.Printf("Re-encryption error: %v", err)
						return
					}

					client.conn.WriteMessage(websocket.TextMessage,
						[]byte(fmt.Sprintf("[Private from %s]: %s", sender.username, reEncryptedMsg)))
					sender.conn.WriteMessage(websocket.TextMessage,
						[]byte(fmt.Sprintf("[Private to %s]: %s", targetUsername, encryptedMsg)))
					return
				}
			}
			sender.conn.WriteMessage(websocket.TextMessage,
				[]byte(fmt.Sprintf("User %s not found", targetUsername)))
			return
		}
	}

	// For regular messages
	messageWithUsername := fmt.Sprintf("%s: %s", sender.username, originalMsg)

	// Encrypt and then decrypt for each client
	for client := range room.clients {
		encryptedMsg, err := encrypt(messageWithUsername, client.key)
		if err != nil {
			log.Printf("Encryption error: %v", err)
			continue
		}

		// Decrypt the message before sending
		decryptedMsg, err := decrypt(encryptedMsg, client.key)
		if err != nil {
			log.Printf("Decryption error: %v", err)
			continue
		}

		err = client.conn.WriteMessage(websocket.TextMessage, []byte(decryptedMsg))
		if err != nil {
			log.Printf("Write error: %v", err)
			client.conn.Close()
			delete(room.clients, client)
		}
	}
}

func handleConnections(room *Room, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		username = "Anonymous"
	}

	// Generate unique encryption key for this client
	clientKey := generateKey()

	client := &Client{
		conn:     conn,
		username: username,
		key:      clientKey,
	}

	room.mutex.Lock()
	room.clients[client] = true
	room.mutex.Unlock()

	// Send the client their encryption key
	keyBase64 := base64.StdEncoding.EncodeToString(clientKey)
	client.conn.WriteMessage(websocket.TextMessage,
		[]byte(fmt.Sprintf("ENCRYPTION_KEY:%s", keyBase64)))

	log.Printf("New client connected: %s", username)
	room.broadcast([]byte(fmt.Sprintf("%s joined the chat", username)), client)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			room.mutex.Lock()
			delete(room.clients, client)
			room.mutex.Unlock()
			conn.Close()
			break
		}

		message := fmt.Sprintf("%s: %s", username, string(msg))
		log.Printf("Message received: %s", message)
		room.broadcast([]byte(message), client)
	}
}

func main() {
	room := NewRoom()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleConnections(room, w, r)
	})

	http.Handle("/", http.FileServer(http.Dir(".")))

	fmt.Println("Server starting at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

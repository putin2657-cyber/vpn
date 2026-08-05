package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"vpn-server/pkg/db"
)

var (
	dbPath   = flag.String("db", "vpn.db", "path to SQLite database")
	action   = flag.String("action", "list", "action: list, add, delete, sessions")
	username = flag.String("user", "", "username for add/delete")
	email    = flag.String("email", "", "email for user")
	userID   = flag.Int("id", 0, "user ID for delete")
	limit    = flag.Int("limit", 20, "number of sessions to show")
)

func main() {
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	switch *action {
	case "add":
		if *username == "" {
			log.Fatal("username required for add action")
		}
		token := generateToken()
		user, err := database.AddUser(*username, token, *email)
		if err != nil {
			log.Fatalf("failed to add user: %v", err)
		}
		fmt.Printf("✅ User added:\n")
		fmt.Printf("  ID:       %d\n", user.ID)
		fmt.Printf("  Username: %s\n", user.Username)
		fmt.Printf("  Token:    %s\n", user.Token)
		fmt.Printf("  Email:    %s\n", user.Email)

	case "list":
		users, err := database.ListUsers()
		if err != nil {
			log.Fatalf("failed to list users: %v", err)
		}
		if len(users) == 0 {
			fmt.Println("No users found")
			return
		}
		fmt.Printf("\n%-5s %-15s %-40s %-20s\n", "ID", "Username", "Token", "Email")
		fmt.Println("─────────────────────────────────────────────────────────────────────────")
		for _, u := range users {
			fmt.Printf("%-5d %-15s %-40s %-20s\n", u.ID, u.Username, u.Token, u.Email)
		}

	case "sessions":
		sessions, err := database.ListSessions(*limit)
		if err != nil {
			log.Fatalf("failed to list sessions: %v", err)
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions recorded yet")
			return
		}
		fmt.Printf("\n%-5s %-15s %-12s %-12s %-20s %-20s\n",
			"ID", "Username", "IP", "Status", "Connected", "Disconnected")
		fmt.Println("──────────────────────────────────────────────────────────────────────────────────────────")
		for _, s := range sessions {
			disconnected := s.DisconnectedAt
			if disconnected == "" {
				disconnected = "—"
			}
			fmt.Printf("%-5d %-15s %-12s %-12s %-20s %-20s\n",
				s.ID, s.Username, s.IPAssigned, s.Action, s.ConnectedAt, disconnected)
		}

	case "delete":
		if *userID == 0 {
			log.Fatal("user ID required for delete action")
		}
		err := database.DeleteUser(*userID)
		if err != nil {
			log.Fatalf("failed to delete user: %v", err)
		}
		fmt.Printf("✅ User %d deleted\n", *userID)

	default:
		log.Fatalf("unknown action: %s", *action)
	}
}

// generateToken генерирует случайный 32-символьный токен
func generateToken() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", b)
}

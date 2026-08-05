package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"vpn-server/pkg/db"
)

var database *db.DB

type UserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type DeleteRequest struct {
	ID int `json:"id"`
}

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func main() {
	port := flag.String("port", "3000", "HTTP listen port")
	dbPath := flag.String("db", "vpn.db", "SQLite database path")
	flag.Parse()

	var err error
	database, err = db.Open(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// API Routes
	http.HandleFunc("/api/users", handleUsers)
	http.HandleFunc("/api/users/add", handleAddUser)
	http.HandleFunc("/api/users/delete", handleDeleteUser)
	http.HandleFunc("/health", handleHealth)

	// Serve static files (if any)
	http.Handle("/", http.FileServer(http.Dir("public")))

	addr := ":" + *port
	log.Printf("VPN Admin API listening on http://localhost%s", addr)
	log.Printf("📊 Endpoints:")
	log.Printf("  GET  http://localhost%s/api/users", addr)
	log.Printf("  POST http://localhost%s/api/users/add", addr)
	log.Printf("  POST http://localhost%s/api/users/delete", addr)

	if err := http.ListenAndServe(addr, enableCORS(http.DefaultServeMux)); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func enableCORS(next *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	users, err := database.ListUsers()
	if err != nil {
		writeError(w, "Failed to list users", err)
		return
	}

	writeJSON(w, Response{Success: true, Data: users})
}

func handleAddUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request", err)
		return
	}

	if req.Username == "" {
		writeJSON(w, Response{Success: false, Error: "Username is required"})
		return
	}

	token := generateToken()
	user, err := database.AddUser(req.Username, token, req.Email)
	if err != nil {
		writeError(w, "Failed to add user", err)
		return
	}

	writeJSON(w, Response{Success: true, Data: user})
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request", err)
		return
	}

	if err := database.DeleteUser(req.ID); err != nil {
		writeError(w, "Failed to delete user", err)
		return
	}

	writeJSON(w, Response{Success: true, Data: "User deleted"})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, Response{Success: true, Data: "API is running"})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	writeJSON(w, Response{
		Success: false,
		Error:   fmt.Sprintf("%s: %v", message, err),
	})
}

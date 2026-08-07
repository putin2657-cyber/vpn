// Command api exposes peer management over HTTP for the desktop admin panel.
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"strings"

	"vpn-server/pkg/awg"
	"vpn-server/pkg/db"
)

var (
	addr     = flag.String("addr", "127.0.0.1:3000", "listen address")
	dbPath   = flag.String("db", "vpn.db", "path to the SQLite database")
	apiToken = flag.String("token", "", "bearer token clients must present; generated when empty")
)

var database *db.DB

type peerView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Created string `json:"created"`
}

type response struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

func main() {
	flag.Parse()

	token := *apiToken
	if token == "" {
		token = randomToken()
		log.Printf("no -token given, generated one for this run: %s", token)
	}

	var err error
	database, err = db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /api/server", handleServer)
	mux.HandleFunc("GET /api/peers", handleListPeers)
	mux.HandleFunc("POST /api/peers", handleAddPeer)
	mux.HandleFunc("DELETE /api/peers/{name}", handleRemovePeer)
	mux.HandleFunc("GET /api/peers/{name}/config", handlePeerConfig)

	log.Printf("admin API listening on http://%s", *addr)
	// Binding to loopback by default: this API can mint and read peer keys.
	if err := http.ListenAndServe(*addr, withAuth(token, withCORS(mux))); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func randomToken() string {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		log.Fatalf("generate token: %v", err)
	}
	return hex.EncodeToString(raw)
}

// withAuth rejects anything without the bearer token. Health stays open so a
// supervisor can probe the process without holding a credential.
func withAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, response{Error: "unauthorised"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, response{Success: true, Data: "ok"})
}

func handleServer(w http.ResponseWriter, _ *http.Request) {
	server, err := database.Server()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, response{Success: true, Data: map[string]any{
		"endpoint":   server.Endpoint,
		"public_key": server.PublicKey.String(),
		"subnet":     server.Subnet.String(),
		"address":    server.Address.String(),
		"dns":        server.DNS,
		"mtu":        server.MTU,
	}})
}

func handleListPeers(w http.ResponseWriter, _ *http.Request) {
	peers, err := database.ListPeers()
	if err != nil {
		fail(w, err)
		return
	}

	views := make([]peerView, 0, len(peers))
	for _, peer := range peers {
		views = append(views, peerView{
			Name:    peer.Name,
			Address: peer.Address.String(),
			Created: peer.Created,
		})
	}
	writeJSON(w, response{Success: true, Data: views})
}

func handleAddPeer(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	tunnelMode, err := requestTunnelMode(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	peer, err := database.AddPeer(request.Name)
	if err != nil {
		fail(w, err)
		return
	}
	server, err := database.Server()
	if err != nil {
		fail(w, err)
		return
	}
	config, err := server.ClientConfig(awg.Peer{
		Name:         peer.Name,
		PrivateKey:   peer.PrivateKey,
		PublicKey:    peer.PublicKey,
		PresharedKey: peer.PresharedKey,
		Address:      peer.Address,
	}, tunnelMode)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, response{Success: true, Data: map[string]any{
		"name":    peer.Name,
		"address": peer.Address.String(),
		"mode":    tunnelMode,
		"config":  config,
	}})
}

func handleRemovePeer(w http.ResponseWriter, r *http.Request) {
	if err := database.RemovePeer(r.PathValue("name")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, response{Success: true, Data: "removed"})
}

func handlePeerConfig(w http.ResponseWriter, r *http.Request) {
	tunnelMode, err := requestTunnelMode(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	peer, err := database.GetPeer(r.PathValue("name"))
	if err != nil {
		fail(w, err)
		return
	}
	server, err := database.Server()
	if err != nil {
		fail(w, err)
		return
	}
	config, err := server.ClientConfig(awg.Peer{
		Name:         peer.Name,
		PrivateKey:   peer.PrivateKey,
		PublicKey:    peer.PublicKey,
		PresharedKey: peer.PresharedKey,
		Address:      peer.Address,
	}, tunnelMode)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, response{Success: true, Data: map[string]any{
		"mode":   tunnelMode,
		"config": config,
	}})
}

func requestTunnelMode(r *http.Request) (awg.TunnelMode, error) {
	return awg.ParseTunnelMode(r.URL.Query().Get("mode"))
}

func fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, db.ErrPeerNotFound):
		status = http.StatusNotFound
	case errors.Is(err, db.ErrNotInitialised):
		status = http.StatusConflict
	}
	w.WriteHeader(status)
	writeJSON(w, response{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, body response) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

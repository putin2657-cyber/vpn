// Command statusd exposes a tiny authenticated JSON status page for the Mac
// control app: VPN peers, memory, disk and awg0 health.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"vpn-server/pkg/awg"
	"vpn-server/pkg/db"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	dbPath := flag.String("db", envOr("BILLING_DB", "/opt/vpn/vpn.db"), "vpn.db path")
	token := flag.String("token", os.Getenv("STATUS_TOKEN"), "bearer token")
	iface := flag.String("iface", awg.DefaultInterface, "awg interface")
	flag.Parse()
	if *token == "" {
		log.Fatal("STATUS_TOKEN is required")
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got != *token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, collect(database, *iface))
	})

	log.Printf("statusd on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func collect(database *db.DB, iface string) map[string]any {
	peers, _ := database.ListPeers()
	live, liveErr := awg.ShowPeers(iface)
	type row struct {
		Name       string `json:"name"`
		Address    string `json:"address"`
		Enabled    bool   `json:"enabled"`
		Handshake  string `json:"handshake"`
		Rx         string `json:"rx"`
		Tx         string `json:"tx"`
		Endpoint   string `json:"endpoint"`
		Online     bool   `json:"online"`
	}
	outPeers := make([]row, 0, len(peers))
	online := 0
	for _, p := range peers {
		r := row{Name: p.Name, Address: p.Address.String(), Enabled: p.Enabled, Handshake: "—", Rx: "—", Tx: "—"}
		if live != nil {
			if st, ok := live[p.PublicKey.String()]; ok {
				r.Handshake = st.HandshakeHuman
				r.Rx = awg.FormatBytes(st.RxBytes)
				r.Tx = awg.FormatBytes(st.TxBytes)
				r.Endpoint = st.Endpoint
				if st.HandshakeHuman != "never" && time.Since(st.LastHandshake) < 3*time.Minute {
					r.Online = true
					online++
				}
			}
		}
		outPeers = append(outPeers, r)
	}
	awgActive := shellOK("systemctl", "is-active", "--quiet", "awg-quick@"+iface)
	return map[string]any{
		"time":           time.Now().UTC().Format(time.RFC3339),
		"awg_iface":      iface,
		"awg_active":     awgActive,
		"awg_error":      errString(liveErr),
		"peers_total":    len(peers),
		"peers_online":   online,
		"peers":          outPeers,
		"memory":         shellOut("free", "-h"),
		"disk":           shellOut("df", "-h", "/"),
		"uptime":         shellOut("uptime", "-p"),
		"loadavg":        strings.TrimSpace(readFile("/proc/loadavg")),
	}
}

func shellOK(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

func shellOut(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out) + " " + err.Error())
	}
	return strings.TrimSpace(string(out))
}

func readFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

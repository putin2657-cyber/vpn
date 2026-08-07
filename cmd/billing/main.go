// Command billing is the family payment portal: 120 RUB/month via YooKassa
// with saved-card auto-renewal and peer disable after a short grace period.
package main

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"vpn-server/pkg/billing"
	"vpn-server/pkg/db"
	"vpn-server/pkg/yookassa"
)

//go:embed web/*
var webFS embed.FS

const sessionCookie = "vpn_session"

func main() {
	loadEnvFile(envOr("BILLING_ENV_FILE", "/opt/vpn/billing.env"))

	action := flag.String("action", "serve", "serve | invite | list | maintenance")
	dbPath := flag.String("db", envOr("BILLING_DB", "vpn.db"), "path to vpn.db")
	name := flag.String("name", "", "display name for invite")
	peer := flag.String("peer", "", "peer name for invite")
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	cfg := billing.Config{
		DatabasePath: *dbPath,
		PublicBase:   strings.TrimRight(envOr("BILLING_PUBLIC_URL", "http://127.0.0.1:8088"), "/"),
		AdminToken:   os.Getenv("BILLING_ADMIN_TOKEN"),
		ShopID:       os.Getenv("YOOKASSA_SHOP_ID"),
		SecretKey:    os.Getenv("YOOKASSA_SECRET_KEY"),
		SyncCommand:  os.Getenv("BILLING_SYNC_CMD"),
		ListenAddr:   envOr("BILLING_ADDR", "127.0.0.1:8088"),
	}

	svc := &billing.Service{
		DB:     database,
		YK:     &yookassa.Client{ShopID: cfg.ShopID, SecretKey: cfg.SecretKey},
		Config: cfg,
	}

	switch *action {
	case "serve":
		if cfg.ShopID == "" || cfg.SecretKey == "" {
			log.Fatal("YOOKASSA_SHOP_ID and YOOKASSA_SECRET_KEY are required")
		}
		if cfg.AdminToken == "" {
			log.Fatal("BILLING_ADMIN_TOKEN is required")
		}
		go func() {
			t := time.NewTicker(time.Hour)
			defer t.Stop()
			for {
				if err := svc.RunMaintenance(); err != nil {
					log.Printf("maintenance: %v", err)
				}
				<-t.C
			}
		}()
		log.Printf("billing portal on http://%s (public URL %s)", cfg.ListenAddr, cfg.PublicBase)
		log.Fatal(http.ListenAndServe(cfg.ListenAddr, newMux(svc)))
	case "invite":
		if *name == "" || *peer == "" {
			log.Fatal("-name and -peer are required")
		}
		member, url, err := svc.Invite(*name, *peer)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Member %q → peer %s\nInvite: %s\n", member.DisplayName, member.PeerName, url)
	case "list":
		members, err := database.ListMembers()
		if err != nil {
			log.Fatal(err)
		}
		if len(members) == 0 {
			fmt.Println("No members yet.")
			return
		}
		fmt.Printf("%-4s %-18s %-16s %-10s %-22s\n", "ID", "Name", "Peer", "Status", "Paid until")
		for _, m := range members {
			fmt.Printf("%-4d %-18s %-16s %-10s %-22s\n", m.ID, m.DisplayName, m.PeerName, m.Status, m.PaidUntil)
		}
	case "maintenance":
		if err := svc.RunMaintenance(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Maintenance done.")
	default:
		log.Fatalf("unknown action %q", *action)
	}
}

func newMux(svc *billing.Service) http.Handler {
	static, err := fs.Sub(webFS, "web/static")
	if err != nil {
		log.Fatalf("embed static: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /", serveFile("web/home.html"))
	mux.HandleFunc("GET /register", serveFile("web/register.html"))
	mux.HandleFunc("GET /login", serveFile("web/login.html"))
	mux.HandleFunc("GET /cabinet", serveFile("web/cabinet.html"))
	mux.HandleFunc("GET /cabinet/done", serveFile("web/cabinet-done.html"))

	mux.HandleFunc("POST /api/register", handleRegister(svc))
	mux.HandleFunc("POST /api/login", handleLogin(svc))
	mux.HandleFunc("POST /api/logout", handleLogout(svc))
	mux.HandleFunc("GET /api/me", handleMe(svc))
	mux.HandleFunc("POST /api/me/pay", handleMePay(svc))
	mux.HandleFunc("GET /api/me/config", handleMeConfig(svc))

	// Legacy invite links still work.
	mux.HandleFunc("GET /i/{token}", serveFile("web/index.html"))
	mux.HandleFunc("GET /i/{token}/done", serveFile("web/done.html"))
	mux.HandleFunc("GET /api/i/{token}", handleMemberStatus(svc))
	mux.HandleFunc("POST /api/i/{token}/pay", handleStartPay(svc))
	mux.HandleFunc("POST /webhook/yookassa", handleWebhook(svc))
	mux.HandleFunc("POST /admin/invite", withAdmin(svc, handleAdminInvite(svc)))
	mux.HandleFunc("GET /admin/members", withAdmin(svc, handleAdminList(svc)))
	mux.HandleFunc("POST /admin/maintenance", withAdmin(svc, handleAdminMaintenance(svc)))
	return mux
}

func serveFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := webFS.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(raw)
	}
}

func handleRegister(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid json"))
			return
		}
		user, session, err := svc.Register(req.Email, req.Password, req.Name)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, db.ErrUserExists) {
				status = http.StatusConflict
			}
			writeErr(w, status, err)
			return
		}
		setSessionCookie(w, session.Token, r.TLS != nil || strings.HasPrefix(svc.Config.PublicBase, "https://"))
		writeJSON(w, map[string]any{
			"email":        user.Email,
			"display_name": user.DisplayName,
		})
	}
}

func handleLogin(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid json"))
			return
		}
		user, session, err := svc.Login(req.Email, req.Password)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err)
			return
		}
		setSessionCookie(w, session.Token, r.TLS != nil || strings.HasPrefix(svc.Config.PublicBase, "https://"))
		writeJSON(w, map[string]any{
			"email":        user.Email,
			"display_name": user.DisplayName,
		})
	}
}

func handleLogout(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			_ = svc.DB.DeleteSession(c.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleMe(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := currentUser(svc, r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err)
			return
		}
		member, err := svc.DB.GetMemberByUserID(user.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{
			"email":                user.Email,
			"display_name":         user.DisplayName,
			"peer_name":            member.PeerName,
			"status":               member.Status,
			"paid_until":           member.PaidUntil,
			"payment_method_saved": member.PaymentMethodID != "",
			"price":                billing.PriceRUB,
		})
	}
}

func handleMePay(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := currentUser(svc, r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err)
			return
		}
		url, err := svc.StartCheckoutForUser(user.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]any{"confirmation_url": url})
	}
}

func handleMeConfig(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := currentUser(svc, r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err)
			return
		}
		config, err := svc.ClientConfigForUser(user.ID)
		if err != nil {
			writeErr(w, http.StatusForbidden, err)
			return
		}
		member, _ := svc.DB.GetMemberByUserID(user.ID)
		name := "vpn.conf"
		if member != nil {
			name = member.PeerName + ".conf"
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		_, _ = io.WriteString(w, config)
	}
}

func currentUser(svc *billing.Service, r *http.Request) (*db.User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, db.ErrSessionNotFound
	}
	return svc.DB.UserBySession(c.Value)
}

func setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
}

func handleMemberStatus(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		member, err := svc.DB.GetMemberByToken(r.PathValue("token"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, map[string]any{
			"display_name":         member.DisplayName,
			"peer_name":            member.PeerName,
			"status":               member.Status,
			"paid_until":           member.PaidUntil,
			"payment_method_saved": member.PaymentMethodID != "",
			"price":                billing.PriceRUB,
		})
	}
}

func handleStartPay(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		url, err := svc.StartCheckout(r.PathValue("token"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]any{"confirmation_url": url})
	}
}

func handleWebhook(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		var note struct {
			Event  string `json:"event"`
			Object struct {
				ID string `json:"id"`
			} `json:"object"`
		}
		if err := json.Unmarshal(raw, &note); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if note.Object.ID == "" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := svc.HandleYooKassaNotification(note.Object.ID); err != nil {
			log.Printf("webhook: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func handleAdminInvite(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			Peer string `json:"peer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		member, url, err := svc.Invite(req.Name, req.Peer)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]any{
			"id":     member.ID,
			"name":   member.DisplayName,
			"peer":   member.PeerName,
			"invite": url,
		})
	}
}

func handleAdminList(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		members, err := svc.DB.ListMembers()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, members)
	}
}

func handleAdminMaintenance(svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if err := svc.RunMaintenance(); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func withAdmin(svc *billing.Service, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		want := svc.Config.AdminToken
		if len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
			next(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	writeJSON(w, map[string]string{"error": err.Error()})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadEnvFile applies KEY=VALUE lines without overriding variables already set.
func loadEnvFile(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, strings.TrimSpace(value))
	}
}

package config

import "time"

var (
	ServerAddr  = ":8443"
	CertFile    = "server.crt"
	KeyFile     = "server.key"
	Token       = "secret-token"
	IPRange     = "10.8.0.0/24"
	ReadTimeout = 30 * time.Second
)

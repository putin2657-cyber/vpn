package awg

// steamBypassCIDRs stay on the ISP path so Steam/Dota keep a native route while
// Discord and the rest of the internet go through the VPN. Keep this short:
// each hole shards AllowedIPs and huge lists break mobile AmneziaWG clients.
var steamBypassCIDRs = []string{
	"103.10.124.0/23",  // Steam Asia
	"146.66.152.0/21",  // Valve EU
	"155.133.224.0/19", // Valve / Steam Datagram Relay (Dota 2)
	"162.254.192.0/21", // Valve SDR
	"185.25.180.0/22",  // Valve EU
	"190.217.33.0/24",  // Steam LatAm edge
	"192.69.96.0/19",   // Valve
	"205.196.6.0/24",   // Steam content
	"208.64.200.0/22",  // Valve
	"208.78.164.0/22",  // Valve
}

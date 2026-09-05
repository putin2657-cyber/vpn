package awg

// smartExcludeCIDRs are IPv4 destinations kept on the ISP path in TunnelSmart.
// Everything else still goes through the VPN (0.0.0.0/0 minus this list).
//
// Intentionally coarse and short: each hole shards AllowedIPs, and mobile
// AmneziaWG/WireGuard clients truncate huge route tables — YouTube then
// randomly leaks to the ISP and fails "every other time".
var smartExcludeCIDRs = []string{
	// Yandex + Yandex Cloud
	"5.45.192.0/18",
	"5.255.192.0/18",
	"37.9.64.0/18",
	"37.140.128.0/18",
	"51.250.0.0/16",
	"77.88.0.0/18",
	"84.201.128.0/18",
	"87.250.224.0/19",
	"93.158.128.0/18",
	"95.108.128.0/17",
	"130.193.32.0/19",
	"141.8.128.0/18",
	"158.160.0.0/16",
	"178.154.128.0/17",
	"213.180.192.0/19",
	// VK / Mail.ru
	"5.2.32.0/19",
	"79.142.16.0/20",
	"87.240.128.0/18",
	"93.186.224.0/20",
	"94.100.176.0/20",
	"95.163.32.0/19",
	"185.30.176.0/22",
	"217.69.128.0/20",
	// Marketplaces (Ozon / WB / EdgeCenter / wbbasket)
	"37.220.160.0/20",
	"45.133.96.0/22",
	"85.198.76.0/22",    // WB static-basket
	"176.101.88.0/21",   // WB CDN
	"178.248.232.0/21",
	"185.13.132.0/22",
	"185.62.200.0/22",   // wildberries.ru / card.wb.ru
	"185.65.148.0/22",
	"185.73.192.0/22",   // Ozon
	"185.138.252.0/22",  // wbbasket.ru
	"185.178.208.0/22",
	"185.184.128.0/22",
	"213.184.152.0/21",  // WB static / geo CDN
	"194.1.214.0/24",    // nel.wb.ru / seller.wildberries.ru
	"91.230.107.0/24",   // nel.wb.ru
	// Telegram DCs — keep on ISP in RU (VPN path stalls "connecting…" after a few minutes)
	"91.108.4.0/22",
	"91.108.8.0/22",
	"91.108.12.0/22",
	"91.108.16.0/22",
	"91.108.56.0/22",
	"149.154.160.0/20",
	// Banks / fintech (coarse)
	"62.76.205.0/24",    // Sber securepayments
	"84.252.128.0/18",   // sberbank.ru / sber.ru / online.sberbank.ru
	"91.206.127.0/24",   // Megamarket (Sber)
	"185.157.96.0/22",
	"194.54.12.0/22",
	"213.165.192.0/19",
	// Gov / Gosuslugi
	"91.223.93.0/24",
	"185.206.164.0/22",
	"195.43.144.0/24",
	// Emias
	"82.202.188.0/22",
	// Mos.ru / Mosgortelecom
	"37.230.160.0/19",
	"94.79.0.0/18",
	"185.173.0.0/22",
	"194.54.148.0/22",
	"212.11.128.0/19",
	"213.79.88.0/24",
}

// smartForceVPN CIDRs are always placed first in AllowedIPs so mobile clients
// that truncate long route lists still send YouTube/Google through the tunnel.
var smartForceVPN = []string{
	"8.8.4.0/24",
	"8.8.8.0/24",
	"64.233.160.0/19",
	"66.102.0.0/20",
	"66.249.64.0/19",
	"72.14.192.0/18",
	"74.125.0.0/16",
	"108.177.0.0/17",
	"142.250.0.0/15",
	"172.217.0.0/16",
	"172.253.0.0/16",
	"173.194.0.0/16",
	"209.85.128.0/17",
	"216.58.192.0/19",
	"216.239.32.0/19",
}

// smartExcludeIPv6CIDRs keep major RU IPv6 ranges off the tunnel.
var smartExcludeIPv6CIDRs = []string{
	"2a02:6b8::/32",  // Yandex
	"2a0d:d6c0::/29", // Yandex Cloud
}

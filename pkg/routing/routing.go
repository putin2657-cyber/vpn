package routing

import (
	"net"
	"sync"
)

// Client — подключенная VPN-сессия с точки зрения маршрутизатора
type Client interface {
	WritePacket(pkt []byte) error
	Close() error
}

// RoutingTable управляет маршрутизацией пакетов между клиентами
type RoutingTable struct {
	mu      sync.RWMutex
	clients map[string]Client // client IP string -> сессия
}

// NewRoutingTable создает новую таблицу маршрутизации
func NewRoutingTable() *RoutingTable {
	return &RoutingTable{
		clients: make(map[string]Client),
	}
}

// RegisterClient регистрирует клиента с выданным ему IP адресом
func (rt *RoutingTable) RegisterClient(ip net.IP, c Client) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.clients[ip.String()] = c
}

// UnregisterClient удаляет клиента из таблицы
func (rt *RoutingTable) UnregisterClient(ip net.IP) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.clients, ip.String())
}

// Lookup возвращает клиента с указанным IP или nil, если маршрута нет
func (rt *RoutingTable) Lookup(destIP net.IP) Client {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.clients[destIP.String()]
}

// Count возвращает количество активных клиентов
func (rt *RoutingTable) Count() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.clients)
}

// CloseAll разрывает соединения со всеми клиентами (для остановки сервера)
func (rt *RoutingTable) CloseAll() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for ip, c := range rt.clients {
		_ = c.Close()
		delete(rt.clients, ip)
	}
}

package main

import (
	"net"
	"testing"
)

func newTestAllocator() *ipAllocator {
	return &ipAllocator{base: net.ParseIP("10.8.0.0"), next: ipPoolFirst}
}

// net.ParseIP returns the 16-byte form, where index 3 sits inside the
// IPv4-in-IPv6 prefix rather than on the last octet.
func TestAllocateReturnsUsableIPv4(t *testing.T) {
	a := newTestAllocator()

	ip := a.Allocate()
	if ip == nil {
		t.Fatal("Allocate returned nil on an empty pool")
	}
	if ip.To4() == nil {
		t.Fatalf("allocated %s is not a usable IPv4 address", ip)
	}
	if got := ip.String(); got != "10.8.0.2" {
		t.Fatalf("got %s, want 10.8.0.2", got)
	}

	if got := a.Allocate().String(); got != "10.8.0.3" {
		t.Fatalf("second allocation is %s, want 10.8.0.3", got)
	}
}

func TestAllocateHandsOutDistinctAddresses(t *testing.T) {
	a := newTestAllocator()
	seen := map[string]bool{}

	for i := 0; i < ipPoolLast-ipPoolFirst+1; i++ {
		ip := a.Allocate()
		if ip == nil {
			t.Fatalf("pool ran dry after %d allocations", i)
		}
		if seen[ip.String()] {
			t.Fatalf("%s handed out twice", ip)
		}
		seen[ip.String()] = true
	}
}

func TestAllocateReturnsNilWhenExhausted(t *testing.T) {
	a := newTestAllocator()
	for i := 0; i < ipPoolLast-ipPoolFirst+1; i++ {
		a.Allocate()
	}

	if ip := a.Allocate(); ip != nil {
		t.Fatalf("got %s from an exhausted pool, want nil", ip)
	}
}

func TestReleaseReturnsAddressToPool(t *testing.T) {
	a := newTestAllocator()
	first := a.Allocate()
	for i := 1; i < ipPoolLast-ipPoolFirst+1; i++ {
		a.Allocate()
	}

	a.Release(first)

	reused := a.Allocate()
	if reused == nil {
		t.Fatal("Allocate returned nil after a release")
	}
	if !reused.Equal(first) {
		t.Fatalf("got %s, want the released %s", reused, first)
	}
}

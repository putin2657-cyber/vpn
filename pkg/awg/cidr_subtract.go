package awg

import (
	"fmt"
	"net"
	"sort"
)

// subtractCIDRs returns universe minus every network in excludes.
// Only same-family networks are considered; others are ignored.
func subtractCIDRs(universe string, excludes []string) ([]string, error) {
	_, uNet, err := net.ParseCIDR(universe)
	if err != nil {
		return nil, fmt.Errorf("universe %q: %w", universe, err)
	}
	ones, bits := uNet.Mask.Size()
	if ones != 0 || (bits != 32 && bits != 128) {
		// We only need 0.0.0.0/0 and ::/0 in practice, but keep general.
	}

	remaining := []*net.IPNet{uNet}
	for _, raw := range excludes {
		_, ex, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, fmt.Errorf("exclude %q: %w", raw, err)
		}
		if len(ex.IP) != len(uNet.IP) {
			// IPv4 vs IPv6 — skip.
			continue
		}
		var next []*net.IPNet
		for _, cur := range remaining {
			next = append(next, addressExclude(cur, ex)...)
		}
		remaining = next
	}

	out := make([]string, 0, len(remaining))
	for _, n := range remaining {
		ones, bits := n.Mask.Size()
		out = append(out, fmt.Sprintf("%s/%d", n.IP.String(), ones))
		_ = bits
	}
	out = collapseCIDRStrings(out)
	sort.Strings(out)
	return out, nil
}

// collapseCIDRStrings merges adjacent/overlapping CIDRs into supernets.
func collapseCIDRStrings(cidrs []string) []string {
	if len(cidrs) < 2 {
		return cidrs
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, raw := range cidrs {
		_, n, err := net.ParseCIDR(raw)
		if err != nil {
			continue
		}
		nets = append(nets, n)
	}
	if len(nets) == 0 {
		return nil
	}
	sort.Slice(nets, func(i, j int) bool {
		if a, b := nets[i].IP.String(), nets[j].IP.String(); a != b {
			return a < b
		}
		onesI, _ := nets[i].Mask.Size()
		onesJ, _ := nets[j].Mask.Size()
		return onesI < onesJ
	})
	changed := true
	for changed {
		changed = false
		var next []*net.IPNet
		for i := 0; i < len(nets); {
			if i+1 >= len(nets) {
				next = append(next, nets[i])
				break
			}
			if merged := tryMerge(nets[i], nets[i+1]); merged != nil {
				next = append(next, merged)
				i += 2
				changed = true
				continue
			}
			next = append(next, nets[i])
			i++
		}
		nets = next
	}
	out := make([]string, 0, len(nets))
	for _, n := range nets {
		ones, _ := n.Mask.Size()
		out = append(out, fmt.Sprintf("%s/%d", n.IP.String(), ones))
	}
	return out
}

func tryMerge(a, b *net.IPNet) *net.IPNet {
	if len(a.IP) != len(b.IP) {
		return nil
	}
	aOnes, bits := a.Mask.Size()
	bOnes, _ := b.Mask.Size()
	if aOnes != bOnes || aOnes == 0 {
		// Allow covering: if one fully contains the other, keep the larger.
		if covers(a, b) {
			return a
		}
		if covers(b, a) {
			return b
		}
		return nil
	}
	parentOnes := aOnes - 1
	parentMask := net.CIDRMask(parentOnes, bits)
	aParent := &net.IPNet{IP: cloneIP(a.IP).Mask(parentMask), Mask: parentMask}
	bParent := &net.IPNet{IP: cloneIP(b.IP).Mask(parentMask), Mask: parentMask}
	if !aParent.IP.Equal(bParent.IP) {
		return nil
	}
	// Ensure a and b are the two children of parent (not duplicates).
	if a.IP.Equal(b.IP) {
		return a
	}
	return aParent
}

// addressExclude returns cur minus ex (non-overlapping parts of cur).
func addressExclude(cur, ex *net.IPNet) []*net.IPNet {
	if !cur.Contains(ex.IP) && !ex.Contains(cur.IP) {
		// No containment either way — check overlap via intersection emptiness.
		if !networksOverlap(cur, ex) {
			return []*net.IPNet{cur}
		}
	}
	if !networksOverlap(cur, ex) {
		return []*net.IPNet{cur}
	}
	// If exclude fully covers current, drop it.
	if covers(ex, cur) {
		return nil
	}
	// If current does not contain exclude start, but they overlap, clip via
	// recursive bipartition of cur until pieces don't overlap ex.
	return bipartitionExclude(cur, ex)
}

func networksOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

func covers(outer, inner *net.IPNet) bool {
	oOnes, oBits := outer.Mask.Size()
	iOnes, iBits := inner.Mask.Size()
	if oBits != iBits || oOnes > iOnes {
		return false
	}
	return outer.Contains(inner.IP)
}

func bipartitionExclude(cur, ex *net.IPNet) []*net.IPNet {
	ones, bits := cur.Mask.Size()
	if ones >= bits {
		if ex.Contains(cur.IP) {
			return nil
		}
		return []*net.IPNet{cur}
	}
	left, right := splitNetwork(cur)
	var out []*net.IPNet
	for _, part := range []*net.IPNet{left, right} {
		if !networksOverlap(part, ex) {
			out = append(out, part)
			continue
		}
		if covers(ex, part) {
			continue
		}
		out = append(out, bipartitionExclude(part, ex)...)
	}
	return out
}

func splitNetwork(n *net.IPNet) (left, right *net.IPNet) {
	ones, bits := n.Mask.Size()
	leftMask := net.CIDRMask(ones+1, bits)
	leftIP := cloneIP(n.IP).Mask(leftMask)
	left = &net.IPNet{IP: leftIP, Mask: leftMask}

	rightIP := cloneIP(leftIP)
	// Set the new host bit that distinguishes the upper half.
	byteIndex := ones / 8
	bit := 7 - (ones % 8)
	rightIP[byteIndex] |= 1 << uint(bit)
	right = &net.IPNet{IP: rightIP.Mask(leftMask), Mask: leftMask}
	return left, right
}

func cloneIP(ip net.IP) net.IP {
	out := make(net.IP, len(ip))
	copy(out, ip)
	return out
}

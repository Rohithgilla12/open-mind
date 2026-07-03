package enrich

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// extraBlockedRanges are CIDRs not covered by net.IP's built-in classifiers
// that must still never be fetched: CGNAT shared address space, IETF protocol
// assignments, the IPv4 limited broadcast address, and the IPv4-IPv6
// translation prefix.
var extraBlockedRanges = func() []*net.IPNet {
	cidrs := []string{
		"100.64.0.0/10",      // RFC 6598 CGNAT / shared address space
		"192.0.0.0/24",       // RFC 6890 IETF protocol assignments
		"255.255.255.255/32", // limited broadcast
		"64:ff9b::/96",       // RFC 6052 IPv4-IPv6 translation
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(fmt.Sprintf("invalid blocked CIDR %q: %v", c, err))
		}
		nets = append(nets, n)
	}
	return nets
}()

// BlockedIP reports whether the address must never be fetched: loopback,
// private, link-local, unspecified, multicast, or one of the extra ranges
// (CGNAT, protocol assignments, broadcast, IPv4-IPv6 translation).
func BlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, n := range extraBlockedRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// SafeHTTPClient returns a client for fetching user-supplied URLs. The dial
// Control hook runs after DNS resolution, so rebinding and redirects to
// internal addresses are rejected at every hop.
func SafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("parsing dial address %s: %w", address, err)
			}
			if BlockedIP(net.ParseIP(host)) {
				return fmt.Errorf("blocked address %s", host)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DialContext: dialer.DialContext},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects fetching %s", via[0].URL)
			}
			return nil
		},
	}
}

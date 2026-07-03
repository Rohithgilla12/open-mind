package enrich

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// BlockedIP reports whether the address must never be fetched: loopback,
// private, link-local, unspecified, or multicast ranges.
func BlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
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

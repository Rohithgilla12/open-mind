package enrich_test

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rohithgilla12/openmind/api/internal/enrich"
)

func TestBlockedIP(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"10.1.2.3", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"::1", true},
		{"fe80::1", true},
		{"fd00::1", true},
		{"0.0.0.0", true},
		{"224.0.0.1", true},
		{"100.64.0.1", true},
		{"192.0.0.1", true},
		{"255.255.255.255", true},
		{"64:ff9b::8.8.8.8", true},
		{"8.8.8.8", false},
		{"142.250.72.14", false},
		{"2607:f8b0::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := enrich.BlockedIP(net.ParseIP(tt.ip)); got != tt.blocked {
				t.Errorf("BlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestSafeClientRefusesLoopback(t *testing.T) {
	client := enrich.SafeHTTPClient(5 * time.Second)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://127.0.0.1:1/", nil)
	_, err := client.Do(req)
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected blocked-address error, got %v", err)
	}
}

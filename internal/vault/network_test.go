package vault

import (
	"errors"
	"net"
	"testing"
)

func TestValidateLANAddress(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
		errIs   error
	}{
		// Allowed: RFC1918 ranges
		{name: "class-C LAN", addr: "192.168.1.100:8000", wantErr: false},
		{name: "class-A LAN", addr: "10.0.0.1:8000", wantErr: false},
		{name: "class-B LAN low", addr: "172.16.0.1:8000", wantErr: false},
		{name: "class-B LAN high", addr: "172.31.255.255:8000", wantErr: false},
		{name: "class-C alt port", addr: "192.168.10.20:443", wantErr: false},

		// Refused: wildcard binds
		{name: "IPv4 wildcard", addr: "0.0.0.0:8000", wantErr: true, errIs: ErrPublicAddress},
		{name: "IPv6 wildcard", addr: "[::]:8000", wantErr: true, errIs: ErrPublicAddress},

		// Refused: loopback (vault must be LAN-reachable, not local-only)
		{name: "IPv4 loopback", addr: "127.0.0.1:8000", wantErr: true, errIs: ErrPublicAddress},
		{name: "IPv4 loopback alt", addr: "127.0.1.1:8000", wantErr: true, errIs: ErrPublicAddress},
		{name: "IPv6 loopback", addr: "[::1]:8000", wantErr: true, errIs: ErrPublicAddress},

		// Refused: public routable IPs
		{name: "Google DNS", addr: "8.8.8.8:8000", wantErr: true, errIs: ErrPublicAddress},
		{name: "Cloudflare DNS", addr: "1.1.1.1:8000", wantErr: true, errIs: ErrPublicAddress},
		{name: "TEST-NET-3 public", addr: "203.0.113.5:8000", wantErr: true, errIs: ErrPublicAddress},

		// Refused: link-local
		{name: "IPv4 link-local", addr: "169.254.1.1:8000", wantErr: true, errIs: ErrPublicAddress},

		// Refused: 172.32.x is NOT RFC1918 (172.16-31 is, 172.32 is not)
		{name: "non-RFC1918 172.32", addr: "172.32.0.1:8000", wantErr: true, errIs: ErrPublicAddress},

		// Refused: malformed / missing port
		{name: "bare IP no port", addr: "192.168.1.1", wantErr: true, errIs: ErrPublicAddress},
		{name: "public bare IP", addr: "10.0.0.1", wantErr: true, errIs: ErrPublicAddress},
		{name: "hostname no port", addr: "vault.local", wantErr: true, errIs: ErrPublicAddress},
		{name: "empty string", addr: "", wantErr: true, errIs: ErrPublicAddress},
		{name: "invalid IP octets", addr: "999.0.0.1:8000", wantErr: true, errIs: ErrPublicAddress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLANAddress(tt.addr)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateLANAddress(%q) = nil, want error", tt.addr)
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateLANAddress(%q) = %v, want nil", tt.addr, err)
				return
			}
			if tt.errIs != nil && err != nil && !errors.Is(err, tt.errIs) {
				t.Errorf("ValidateLANAddress(%q) error = %v, want errors.Is(%v)", tt.addr, err, tt.errIs)
			}
		})
	}
}

// TestPublicBindRefused is the security-invariant test from testing.md:
// the vault must refuse any public or wildcard bind address — no WAN surface.
func TestPublicBindRefused(t *testing.T) {
	mustRefuse := []string{
		"0.0.0.0:8000",     // wildcard
		"8.8.8.8:8000",     // public
		"1.1.1.1:8000",     // public
		"127.0.0.1:8000",   // loopback refused: vault must have LAN reachability
		"[::1]:8000",       // IPv6 loopback
		"203.0.113.1:8000", // public documentation range
	}

	for _, addr := range mustRefuse {
		err := ValidateLANAddress(addr)
		if err == nil {
			t.Errorf("ValidateLANAddress(%q) = nil — no-WAN guarantee violated; must return ErrPublicAddress", addr)
			continue
		}
		if !errors.Is(err, ErrPublicAddress) {
			t.Errorf("ValidateLANAddress(%q) = %v, want errors.Is(ErrPublicAddress)", addr, err)
		}
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"192.168.255.255", true},
		// Not private
		{"8.8.8.8", false},
		{"172.32.0.1", false},
		{"127.0.0.1", false},
		{"169.254.0.1", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip).To4()
		if ip == nil {
			t.Fatalf("failed to parse test IP %s", tt.ip)
		}
		got := IsPrivateIP(ip)
		if got != tt.want {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

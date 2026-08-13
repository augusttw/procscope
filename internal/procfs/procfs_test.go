package procfs

import (
	"net"
	"testing"
)

func TestParseStatHandlesSpacesInName(t *testing.T) {
	raw := "42 (my worker) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 4096 7"
	s, err := parseStat(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.name != "my worker" || s.state != "S" {
		t.Fatalf("resultado inesperado: %+v", s)
	}
	if s.userTicks != 11 || s.systemTicks != 12 || s.vmBytes != 4096 || s.rssPages != 7 {
		t.Fatalf("campos incorretos: %+v", s)
	}
}

func TestDecodeIPv4Address(t *testing.T) {
	got, err := decodeAddress("0100007F:1538", false)
	if err != nil {
		t.Fatal(err)
	}
	want := net.JoinHostPort("127.0.0.1", "5432")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDependency(t *testing.T) {
	for addr, want := range map[string]string{"127.0.0.1:5432": "postgresql", "[::1]:6379": "redis", "10.0.0.1:443": "http", "10.0.0.1:22": ""} {
		if got := dependency(addr); got != want {
			t.Errorf("dependency(%q)=%q want %q", addr, got, want)
		}
	}
}

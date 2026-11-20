package main

import (
	"net"
	"reflect"
	"testing"
)

func ipv6Loopback(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback: %v", err)
	}
	ln.Close()
}

func closeAll(lns []net.Listener) {
	for _, ln := range lns {
		ln.Close()
	}
}

func TestListenLocalhostBothStacks(t *testing.T) {
	ipv6Loopback(t)
	lns, err := listenAll("localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(lns)
	if len(lns) != 2 {
		t.Fatalf("got %d listeners, want 2", len(lns))
	}
	a, b := lns[0].Addr().(*net.TCPAddr), lns[1].Addr().(*net.TCPAddr)
	if !a.IP.Equal(net.IPv4(127, 0, 0, 1)) || !b.IP.Equal(net.IPv6loopback) {
		t.Errorf("listening on %s and %s, want 127.0.0.1 and ::1", a, b)
	}
	if a.Port != b.Port {
		t.Errorf("ports %d and %d differ", a.Port, b.Port)
	}
	for _, addr := range []*net.TCPAddr{a, b} {
		c, err := net.Dial("tcp", addr.String())
		if err != nil {
			t.Errorf("dial %s: %v", addr, err)
			continue
		}
		c.Close()
	}
}

func TestListenList(t *testing.T) {
	ipv6Loopback(t)
	lns, err := listenAll("[::1]:0,127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(lns)
	if len(lns) != 2 || !lns[0].Addr().(*net.TCPAddr).IP.Equal(net.IPv6loopback) {
		t.Fatalf("listeners %v, want [::1] then 127.0.0.1", lns)
	}
	got := pageURLs(lns)
	want := []string{
		"http://[::1]:" + port(lns[0]) + "/",
		"http://127.0.0.1:" + port(lns[1]) + "/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pageURLs = %v, want %v", got, want)
	}
}

func TestListenEveryAddress(t *testing.T) {
	lns, err := listenAll(":0")
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(lns)
	if got, want := pageURLs(lns), []string{"http://localhost:" + port(lns[0]) + "/"}; !reflect.DeepEqual(got, want) {
		t.Errorf("pageURLs = %v, want %v", got, want)
	}
}

func TestListenErrors(t *testing.T) {
	for _, spec := range []string{"::1:8034", "127.0.0.1", "127.0.0.1:0,", "[::/0]:0"} {
		if lns, err := listenAll(spec); err == nil {
			closeAll(lns)
			t.Errorf("listenAll(%q) succeeded", spec)
		}
	}
	// A failure part way through closes what was already opened.
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if _, err := listenAll("127.0.0.1:0," + busy.Addr().String()); err == nil {
		t.Error("listening on a busy port succeeded")
	}
}

func port(ln net.Listener) string {
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	return p
}

package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// listenAll listens on each of the comma-separated addresses in spec, e.g.
// "localhost:8034" or "127.0.0.1:8034,[::1]:8034". IPv6 addresses go in
// square brackets. A host of "localhost" stands for both loopback
// addresses, 127.0.0.1 and ::1; if one of them can't be listened on (such
// as ::1 with IPv6 disabled), the other is enough. An empty host, as in
// ":8034", listens on every address, IPv4 and IPv6.
func listenAll(spec string) ([]net.Listener, error) {
	var lns []net.Listener
	fail := func(err error) ([]net.Listener, error) {
		for _, ln := range lns {
			ln.Close()
		}
		return nil, err
	}
	for _, addr := range strings.Split(spec, ",") {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return fail(fmt.Errorf("-listen: %v", err))
		}
		if strings.Contains(host, "/") {
			return fail(fmt.Errorf("-listen: %q is a network, not an address; for every address use [::]:%s (or :%s), or name one address, e.g. [::1]:%s", host, port, port, port))
		}
		if host != "localhost" {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fail(err)
			}
			lns = append(lns, ln)
			continue
		}
		var errs []error
		listened := false
		for _, ip := range []string{"127.0.0.1", "::1"} {
			ln, err := net.Listen("tcp", net.JoinHostPort(ip, port))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			lns = append(lns, ln)
			listened = true
			// With port 0, use the port the first one got for the
			// other, so localhost means the same server on both.
			port = fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
		}
		if !listened {
			return fail(errors.Join(errs...))
		}
	}
	return lns, nil
}

// pageURLs lists the addresses to open the page at, one per listener, in
// order and without repeats. Listening on every address gives a
// localhost URL.
func pageURLs(lns []net.Listener) []string {
	var urls []string
	seen := map[string]bool{}
	for _, ln := range lns {
		a := ln.Addr().(*net.TCPAddr)
		host := a.IP.String()
		if a.IP.IsUnspecified() {
			host = "localhost"
		}
		u := fmt.Sprintf("http://%s/", net.JoinHostPort(host, fmt.Sprint(a.Port)))
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	return urls
}

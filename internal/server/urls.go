package server

import (
	"net"
)

// DisplayURL renders a browser-friendly URL from a listen address, turning a
// bare ":8080" or "0.0.0.0:8080" into a localhost URL.
func DisplayURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// No port. A bare IPv6 literal still needs brackets to form a URL.
		if ip := net.ParseIP(addr); ip != nil && ip.To4() == nil {
			return "http://[" + addr + "]"
		}
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// IsLoopback reports whether addr binds only to the local machine. Callers use
// it to decide whether running without a token is safe.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

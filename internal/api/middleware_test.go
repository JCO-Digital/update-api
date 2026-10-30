package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		remote, realIP, want string
	}{
		{"1.2.3.4:1234", "", "1.2.3.4"},
		{"127.0.0.1:5555", "5.6.7.8", "5.6.7.8"},     // behind nginx
		{"1.2.3.4:1234", "5.6.7.8", "1.2.3.4"},       // header from the internet is ignored
		{"[2001:db8::1]:443", "", "2001:db8::1"},     // IPv6
		{"[::1]:5555", "2001:db8::2", "2001:db8::2"}, // nginx over IPv6 loopback
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if got := clientIP(r); got != c.want {
			t.Errorf("clientIP(%s, X-Real-IP %q) = %s, want %s", c.remote, c.realIP, got, c.want)
		}
	}
}

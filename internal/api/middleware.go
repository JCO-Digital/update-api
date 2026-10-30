package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

func AdminAuthMiddleware(adminKeys []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				http.Error(w, "Unauthorized: Missing API Key", http.StatusUnauthorized)
				return
			}

			authorized := false
			for _, key := range adminKeys {
				if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) == 1 {
					authorized = true
				}
			}

			if !authorized {
				http.Error(w, "Unauthorized: Invalid API Key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIP returns the caller's IP. Behind the local nginx proxy every
// request comes from loopback, so the X-Real-IP header nginx sets is used
// instead; it is only trusted when the direct peer is loopback.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
	}
	return host
}

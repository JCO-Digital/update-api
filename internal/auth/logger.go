package auth

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func AnonymizeIP(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "unknown"
	}

	ipv4 := ip.To4()
	if ipv4 != nil {
		// Anonymize IPv4: 127.0.0.1 -> 127.0.x.x
		return fmt.Sprintf("%d.%d.x.x", ipv4[0], ipv4[1])
	}

	// Anonymize IPv6: 2001:db8:85a3:0000:0000:8a2e:0370:7334 -> 2001:db8:x:x:x:x:x:x
	parts := strings.Split(ip.String(), ":")
	if len(parts) >= 2 {
		return fmt.Sprintf("%s:%s:x:x:x:x:x:x", parts[0], parts[1])
	}

	return "anonymized"
}

func LogLicenseCreated(code, clientID string, expiresAt time.Time, source string) error {
	line := fmt.Sprintf("%s | Created: %s | Client: %s | Expires: %s | Source: %s\n",
		code,
		time.Now().Format(time.RFC3339),
		clientID,
		expiresAt.Format(time.RFC3339),
		source,
	)
	return appendToLog("created", line)
}

func LogLicenseValidated(code, ip string) error {
	line := fmt.Sprintf("%s | Validated: %s | IP: %s\n",
		code,
		time.Now().Format(time.RFC3339),
		ip,
	)
	return appendToLog("validated", line)
}

func appendToLog(filename, line string) error {
	dir := "licenses"
	path := filepath.Join(dir, filename)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(line)
	return err
}

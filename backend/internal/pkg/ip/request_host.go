package ip

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// ValidateRequestHost validates strict host[:port] syntax and rejects
// multi-value commas, userinfo (@), path/query/fragment delimiters, backslashes,
// whitespace, and control characters.
func ValidateRequestHost(raw string) (string, bool) {
	hostport := strings.TrimSpace(raw)
	if hostport == "" || len(hostport) > 255 {
		return "", false
	}
	for i := 0; i < len(hostport); i++ {
		ch := hostport[i]
		if ch <= 0x20 || ch == 0x7f {
			return "", false
		}
		switch ch {
		case ',', '@', '/', '\\', '?', '#':
			return "", false
		}
	}

	var host, port string
	if strings.HasPrefix(hostport, "[") {
		if strings.HasSuffix(hostport, "]") {
			host = hostport[1 : len(hostport)-1]
		} else {
			var err error
			host, port, err = net.SplitHostPort(hostport)
			if err != nil || port == "" {
				return "", false
			}
		}
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() || addr.Zone() != "" {
			return "", false
		}
	} else if strings.Contains(hostport, ":") {
		var err error
		host, port, err = net.SplitHostPort(hostport)
		if err != nil || port == "" {
			return "", false
		}
	} else {
		host = hostport
	}

	if port != "" {
		if !isValidPortString(port) {
			return "", false
		}
	}

	if !isValidHostNameOrIP(host) {
		return "", false
	}
	return hostport, true
}

func isValidPortString(port string) bool {
	if port == "" || len(port) > 5 || port[0] == '0' {
		return false
	}
	for i := 0; i < len(port); i++ {
		if port[i] < '0' || port[i] > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func isValidHostNameOrIP(host string) bool {
	if host == "" {
		return false
	}
	if addr, err := netip.ParseAddr(host); err == nil && addr.IsValid() && addr.Zone() == "" {
		return true
	}
	if len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			ch := label[i]
			isAlpha := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
			isDigit := ch >= '0' && ch <= '9'
			if !isAlpha && !isDigit && ch != '-' {
				return false
			}
		}
	}
	return true
}

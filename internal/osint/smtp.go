package osint

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// smtpVerify checks whether an email address likely exists by performing an
// SMTP RCPT TO probe without sending any mail.
// Returns true if the server accepted the address (250), false otherwise.
// Many servers accept-all (catch-all), so a true result isn't definitive —
// but a false/error result means the address is definitely invalid.
func smtpVerify(email string) (valid bool, catchAll bool) {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return false, false
	}
	domain := parts[1]

	// Look up MX records
	mxs, err := net.LookupMX(domain)
	if err != nil || len(mxs) == 0 {
		return false, false
	}

	host := strings.TrimSuffix(mxs[0].Host, ".")
	addr := net.JoinHostPort(host, "25")

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return false, false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return false, false
	}
	defer c.Quit() //nolint:errcheck

	if err := c.Hello("verify.example.com"); err != nil {
		return false, false
	}
	if err := c.Mail("probe@verify.example.com"); err != nil {
		return false, false
	}

	err = c.Rcpt(email)
	if err == nil {
		// Accepted — but might be catch-all; probe a random address to check
		dummy := fmt.Sprintf("zxqwerty99nonexistent@%s", domain)
		if err2 := c.Rcpt(dummy); err2 == nil {
			return true, true // catch-all domain
		}
		return true, false // real acceptance
	}
	return false, false
}

// VerifyPatterns takes pattern contacts and upgrades their confidence based on
// SMTP verification. Confirmed addresses get 85%, catch-all domains get 40%,
// rejected addresses are removed.
func VerifyPatterns(contacts []Contact) []Contact {
	if len(contacts) == 0 {
		return contacts
	}

	// Group by domain — one catch-all check covers all addresses at that domain
	type result struct{ valid, catchAll bool }
	domainChecked := map[string]bool{}
	catchAllDomains := map[string]bool{}

	// First check if the domain is catch-all using one address
	for _, c := range contacts {
		parts := strings.SplitN(c.Email, "@", 2)
		if len(parts) != 2 {
			continue
		}
		d := parts[1]
		if domainChecked[d] {
			continue
		}
		domainChecked[d] = true
		_, isCatchAll := smtpVerify(c.Email)
		if isCatchAll {
			catchAllDomains[d] = true
		}
	}

	var out []Contact
	for _, c := range contacts {
		if c.Email == "" {
			out = append(out, c)
			continue
		}
		parts := strings.SplitN(c.Email, "@", 2)
		d := parts[1]

		if catchAllDomains[d] {
			// Can't distinguish real vs fake on this domain
			c.Confidence = 40
			c.Notes = "domain is catch-all"
			out = append(out, c)
			continue
		}

		valid, _ := smtpVerify(c.Email)
		if valid {
			c.Confidence = 85
			c.Notes = "smtp verified"
			out = append(out, c)
		}
		// silently drop rejected addresses
	}
	return out
}

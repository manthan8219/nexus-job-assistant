package osint

import "strings"

// normalizeEmail trims surrounding whitespace and lowercases the domain part
// of an address so DNS lookups and dedup keys are stable. The local part is
// preserved as-is: RFC 5321 treats it as case-sensitive in theory, but every
// real mailbox is reached regardless of case.
func normalizeEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return email
	}
	return email[:at] + "@" + strings.ToLower(email[at+1:])
}

// validEmail reports whether email is structurally a plausible RFC 5321
// address. It is deliberately permissive (real mail systems accept a lot) but
// rejects addresses that could never reach a mailbox: empty local part,
// missing or doubled @, whitespace or control characters, over-long parts,
// and domains without at least one dot. No network activity is involved.
func validEmail(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > 254 {
		return false
	}
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	// The local part may not itself contain an @.
	if strings.Contains(email[:at], "@") {
		return false
	}
	local, domain := email[:at], email[at+1:]
	if len(local) > 64 || len(domain) > 253 {
		return false
	}
	if local == "" || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return false
	}
	for _, r := range local {
		if r == '.' { // dot-atom: allowed inside, checked above
			continue
		}
		if !isAText(r) {
			return false
		}
	}
	return validDomain(domain)
}

// isAText reports whether r is a valid RFC 5321 atext character for the local
// part of an address.
func isAText(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	switch r {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '/', '=', '?', '^', '_', '`', '{', '|', '}', '~':
		return true
	}
	return false
}

// validDomain reports whether domain is a plausible hostname for an email
// address: ASCII labels separated by dots, no empty or over-long labels, and
// at least one dot (single-label names such as "localhost" are not reachable
// from the outside world).
func validDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if domain == "" || len(domain) > 253 || !strings.Contains(domain, ".") {
		return false
	}
	if strings.HasPrefix(domain, "-") || strings.HasSuffix(domain, "-") || strings.Contains(domain, "..") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

// domainOf extracts the lowercased domain part of an address, or "" when the
// address has no usable @domain.
func domainOf(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(email[at+1:], "."))
}

// disposableDomains are throwaway/temporary mailbox providers. Addresses at
// these domains are technically deliverable but are never suitable for job
// outreach, so the verifier rejects them before touching the network.
var disposableDomains = map[string]bool{
	"mailinator.com": true, "mailinator.net": true, "mailinator.org": true,
	"mailinator2.com": true, "guerrillamail.com": true, "guerrillamail.de": true,
	"guerrillamail.net": true, "guerrillamail.org": true, "guerrillamail.biz": true,
	"guerrillamail.info": true, "sharklasers.com": true, "grr.la": true,
	"spam4.me": true, "yopmail.com": true, "yopmail.fr": true, "yopmail.net": true,
	"tempmail.com": true, "tempmailo.com": true, "temp-mail.org": true,
	"throwawaymail.com": true, "10minutemail.com": true, "10minutemail.net": true,
	"maildrop.cc": true, "getnada.com": true, "nada.email": true,
	"dispostable.com": true, "mailnesia.com": true, "mintemail.com": true,
	"spamgourmet.com": true, "mailcatch.com": true, "trashmail.com": true,
	"trashmail.de": true, "33mail.com": true, "dumpmail.com": true,
	"e4ward.com": true, "gishpuppy.com": true, "jetable.org": true,
	"mailnull.com": true, "moakt.com": true, "mytrashmail.com": true,
	"otr.to": true, "pookmail.com": true, "spamobox.com": true,
	"sogetthis.com": true, "tmail.ws": true, "mailtemp.net": true,
	"burnermail.io": true, "mailde.de": true, "spam.la": true,
	"mailforspam.com": true, "tempinbox.com": true, "mailexpire.com": true,
	"fakemail.net": true, "fakeinbox.com": true, "fake-mail.net": true,
	"instant-mail.de": true, "meltmail.com": true, "spamfree24.org": true,
	"discard.email": true, "spambox.us": true, "sendspamhere.com": true,
	"veryrealemail.com": true, "inboxbear.com": true, "boyfriendmail.com": true,
	"emailtemporario.com.br": true,
}

// isDisposableDomain reports whether the address's domain is a throwaway
// mailbox provider.
func isDisposableDomain(email string) bool {
	return disposableDomains[domainOf(email)]
}

// roleAddresses are shared inboxes, not individual people. They may be
// monitored, but outreach to them is far less personal than to a named
// person, so the verifier flags them in the verification detail.
var roleAddresses = map[string]bool{
	"info": true, "sales": true, "support": true, "admin": true,
	"webmaster": true, "postmaster": true, "abuse": true, "noreply": true,
	"no-reply": true, "donotreply": true, "contact": true, "help": true,
	"helpdesk": true, "marketing": true, "press": true, "media": true,
	"billing": true, "team": true, "office": true, "hello": true,
	"howdy": true, "inquiries": true, "newsletter": true, "notifications": true,
	"privacy": true, "root": true, "user": true, "username": true,
	"mail": true, "email": true, "careers": true, "jobs": true, "hr": true,
	"recruiting": true, "talent": true, "people": true, "hiring": true,
}

// isRoleAddress reports whether the local part is a shared inbox (info@,
// sales@, postmaster@, …) rather than an individual mailbox.
func isRoleAddress(email string) bool {
	return roleAddresses[strings.ToLower(strings.SplitN(email, "@", 2)[0])]
}

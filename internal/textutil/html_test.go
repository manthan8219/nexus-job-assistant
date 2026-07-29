package textutil

import (
	"strings"
	"testing"
)

func TestHTMLToPlain_DoubleEscaped(t *testing.T) {
	in := `&lt;h2&gt;Who we are&lt;/h2&gt;&lt;p&gt;About Stripe&amp;nbsp;team.&lt;/p&gt;&lt;ul&gt;&lt;li&gt;Build APIs&lt;/li&gt;&lt;li&gt;Ship payments&lt;/li&gt;&lt;/ul&gt;`
	out := HTMLToPlain(in)
	if strings.Contains(out, "&lt;") || strings.Contains(out, "<h2>") || strings.Contains(out, "&amp;") {
		t.Fatalf("still has markup/entities:\n%s", out)
	}
	if !strings.Contains(out, "Who we are") || !strings.Contains(out, "About Stripe") {
		t.Fatalf("missing text:\n%s", out)
	}
	if !strings.Contains(out, "Build APIs") {
		t.Fatalf("missing list item:\n%s", out)
	}
}

func TestHTMLToPlain_RawHTML(t *testing.T) {
	in := `<h2>Role</h2><p>Do things.</p><br/><p>More.</p>`
	out := HTMLToPlain(in)
	if strings.ContainsAny(out, "<>") {
		t.Fatalf("tags remain: %q", out)
	}
	if !strings.Contains(out, "Role") || !strings.Contains(out, "Do things.") {
		t.Fatalf("bad out: %q", out)
	}
}

package nexusdir

import (
	"path/filepath"
	"testing"
)

func TestHome_PrefersNexusHomeOverride(t *testing.T) {
	t.Setenv("NEXUS_HOME", "/opt/nexus-data")
	t.Setenv("HOME", "/home/ada")
	if got := Home(); got != "/opt/nexus-data" {
		t.Fatalf("Home() = %q; want NEXUS_HOME override %q", got, "/opt/nexus-data")
	}
}

func TestHome_UsesHomeWhenSet(t *testing.T) {
	t.Setenv("NEXUS_HOME", "")
	t.Setenv("HOME", "/home/ada")
	if want := filepath.Join("/home/ada", ".nexus"); Home() != want {
		t.Fatalf("Home() = %q; want %q", Home(), want)
	}
}

func TestHome_FallsBackWhenHomeUndefined(t *testing.T) {
	t.Setenv("NEXUS_HOME", "")
	t.Setenv("HOME", "")
	got := Home()
	if got == "" {
		t.Fatal("Home() returned empty with no HOME set")
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("Home() = %q; want an absolute fallback path", got)
	}
}

func TestUserHome_FallsBackWhenUndefined(t *testing.T) {
	t.Setenv("HOME", "")
	if got := UserHome(); got == "" {
		t.Fatal("UserHome() returned empty with no HOME set")
	}
}

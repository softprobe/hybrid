package version

import "testing"

func TestSemverTagAddsVPrefix(t *testing.T) {
	prev := Version
	t.Cleanup(func() { Version = prev })

	Version = "0.5.0"
	if got, want := SemverTag(), "v0.5.0"; got != want {
		t.Fatalf("SemverTag() = %q, want %q", got, want)
	}
}

func TestSemverTagPreservesLeadingV(t *testing.T) {
	prev := Version
	t.Cleanup(func() { Version = prev })

	Version = "v0.5.0"
	if got, want := SemverTag(), "v0.5.0"; got != want {
		t.Fatalf("SemverTag() = %q, want %q", got, want)
	}
}

func TestCLIDetailMatchesLdflagsExample(t *testing.T) {
	prev := Version
	t.Cleanup(func() { Version = prev })

	Version = "v0.5.0"
	got := CLIDetail("http-control-api@v1")
	want := "v0.5.0 (spec http-control-api@v1)"
	if got != want {
		t.Fatalf("CLIDetail(...) = %q, want %q", got, want)
	}
}

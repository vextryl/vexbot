package speaker

import "testing"

func TestNormalizeCollapsesWhitespaceAndControls(t *testing.T) {
	got := Normalize(" \tAlex\n\x00the\rBard  ")
	if want := "Alex the Bard"; got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
}

func TestResolveUsesNormalizedNameOrUserID(t *testing.T) {
	displayNames := map[string]string{
		"42": "  Alex\t",
		"99": " \n",
	}

	if got, want := Resolve("42", displayNames), "Alex"; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
	if got, want := Resolve("99", displayNames), "99"; got != want {
		t.Fatalf("Resolve() blank name = %q, want %q", got, want)
	}
	if got, want := Resolve("100", displayNames), "100"; got != want {
		t.Fatalf("Resolve() missing name = %q, want %q", got, want)
	}
}

package wallapop

import "testing"

func TestSignSchemesDiffer(t *testing.T) {
	pipe, err := Sign(SchemePipe, "put", "/api/v3/items/x/reactivate", 1757000000000)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := Sign(SchemeLegacy, "PUT", "/api/v3/items/x/reactivate", 1757000000000)
	if err != nil {
		t.Fatal(err)
	}
	if pipe == legacy {
		t.Fatal("both schemes produced the same signature")
	}
	if len(pipe) != 44 || len(legacy) != 44 {
		t.Fatalf("expected base64 of a 32-byte digest, got %d and %d chars", len(pipe), len(legacy))
	}
}

func TestMatchScheme(t *testing.T) {
	const method, path = "PUT", "/api/v3/items/x/reactivate"
	const ts = int64(1757000000000)

	for _, want := range Schemes() {
		sig, err := Sign(want, method, path, ts)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := MatchScheme(method, path, ts, sig)
		if !ok || got != want {
			t.Fatalf("signature of %s matched %q (ok=%t)", want, got, ok)
		}
	}

	if _, ok := MatchScheme(method, path, ts, "not-a-signature"); ok {
		t.Fatal("a bogus signature matched a scheme")
	}
}

func TestUnknownScheme(t *testing.T) {
	if _, err := Sign(SignScheme("nope"), "GET", "/", 0); err == nil {
		t.Fatal("expected an error for an unknown scheme")
	}
}

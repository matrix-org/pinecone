package types

import "testing"

func TestPartialKeyMatch(t *testing.T) {
	a := PublicKey{1, 2, 3, 3, 3}
	b := PublicKey{1, 2, 4, 4, 4}

	if !a.EqualMaskTo(b, PublicKey{0xFF, 0xFF, 0, 0, 0}) {
		t.Fatalf("Should have matched but didn't")
	}
	if a.EqualMaskTo(b, PublicKey{0xFF, 0xFF, 0xFF, 0, 0}) {
		t.Fatalf("Should not have matched but did")
	}
}

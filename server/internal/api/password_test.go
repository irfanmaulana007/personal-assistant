package api

import (
	"strings"
	"testing"
)

func TestGenerateRandomPassword(t *testing.T) {
	const n = 16
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		pw, err := generateRandomPassword(n)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pw) != n {
			t.Fatalf("expected length %d, got %d (%q)", n, len(pw), pw)
		}
		for _, c := range pw {
			if !strings.ContainsRune(passwordAlphabet, c) {
				t.Fatalf("password contains char %q outside the alphabet", c)
			}
		}
		if seen[pw] {
			t.Fatalf("generated a duplicate password %q — not random", pw)
		}
		seen[pw] = true
	}
}

func TestPasswordAlphabetOmitsAmbiguous(t *testing.T) {
	for _, c := range "0O1lI" {
		if strings.ContainsRune(passwordAlphabet, c) {
			t.Errorf("alphabet should omit ambiguous char %q", c)
		}
	}
}

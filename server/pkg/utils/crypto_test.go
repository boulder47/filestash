package utils

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := "0123456789abcdef" // 16 bytes -> AES-128
	cases := []string{"", "hello", `{"expiry":"2027-01-01T00:00:00Z","name":"QA"}`, strings.Repeat("x", 10000)}
	for _, plain := range cases {
		enc, err := EncryptString(key, plain)
		if err != nil {
			t.Fatalf("EncryptString(%q) error: %v", plain, err)
		}
		if plain != "" && enc == plain {
			t.Errorf("ciphertext equals plaintext for %q", plain)
		}
		dec, err := DecryptString(key, enc)
		if err != nil {
			t.Fatalf("DecryptString error: %v", err)
		}
		if dec != plain {
			t.Errorf("round-trip mismatch: got %q want %q", dec, plain)
		}
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	enc, err := EncryptString("0123456789abcdef", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptString("fedcba9876543210", enc); err == nil {
		t.Error("expected decryption with wrong key to fail, got nil")
	}
}

func TestHashDeterministicAndSized(t *testing.T) {
	a := Hash("hello", 10)
	b := Hash("hello", 10)
	if a != b {
		t.Errorf("Hash not deterministic: %q != %q", a, b)
	}
	if len(a) > 10 {
		t.Errorf("Hash length %d exceeds requested 10", len(a))
	}
	if Hash("hello", 10) == Hash("world", 10) {
		t.Error("distinct inputs produced identical hash")
	}
}

func TestRandomStringLengthAndCharset(t *testing.T) {
	s := RandomString(32)
	if len(s) != 32 {
		t.Fatalf("RandomString(32) len = %d", len(s))
	}
	for _, r := range s {
		if !strings.ContainsRune(string(Letters), r) {
			t.Errorf("RandomString produced out-of-alphabet rune %q", r)
		}
	}
}

// GenerateID must ignore volatile/sensitive keys (password, path, session,
// timestamp) so the telemetry/session id can never encode a password.
func TestGenerateIDIgnoresSensitiveKeys(t *testing.T) {
	base := map[string]string{"type": "samba", "host": "nas", "user": "alice"}
	withPassword := map[string]string{"type": "samba", "host": "nas", "user": "alice", "password": "s3cr3t", "path": "/x", "timestamp": "123"}
	if GenerateID(base) != GenerateID(withPassword) {
		t.Error("GenerateID changed when only password/path/timestamp differ")
	}
	other := map[string]string{"type": "samba", "host": "nas", "user": "bob"}
	if GenerateID(base) == GenerateID(other) {
		t.Error("GenerateID collided for different users")
	}
}

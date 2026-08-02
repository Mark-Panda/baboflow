package conf

import "testing"

func TestEncryptDecrypt(t *testing.T) {
	secret := "test-secret-32-bytes-padding-1234"
	plain := "sk-abc1234567890def"
	enc, err := Encrypt(secret, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plain || enc == "" {
		t.Fatalf("ciphertext should differ and be non-empty")
	}
	dec, err := Decrypt(secret, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != plain {
		t.Fatalf("roundtrip mismatch: got %q want %q", dec, plain)
	}
}

func TestDecryptWrongSecret(t *testing.T) {
	enc, _ := Encrypt("secret-a", "hello")
	if _, err := Decrypt("secret-b", enc); err == nil {
		t.Fatalf("expected decrypt error with wrong secret")
	}
}

func TestMask(t *testing.T) {
	cases := map[string]string{
		"sk-abcdefgh1234": "sk-****1234",
		"short":           "s****",
		"":                "",
	}
	for in, want := range cases {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q)=%q want %q", in, got, want)
		}
	}
}

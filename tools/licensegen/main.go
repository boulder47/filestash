// Command licensegen generates and verifies Filestash license keys for TESTING
// the plg_license code path against your own builds.
//
// It is self-contained (standard library only) and reimplements the exact
// wire format of server/pkg/utils (zlib -> AES-GCM -> base64url), so it builds
// and runs from a clean checkout with no `go generate` step and no extra
// modules:
//
//   generate: go run ./tools/licensegen -name "Acme QA" -expiry 8760h
//   custom:   go run ./tools/licensegen -name QA -expiry 8760h -secret mykey
//   verify:   go run ./tools/licensegen -verify <LICENSE_STRING> [-secret mykey]
//
// With the default secret the output is byte-compatible with the stock loader
// (server/plugin/plg_license). There is no vendor secret involved - the stock
// key ships in the public source as fmt.Sprintf("%-16s", "filestash").
package main

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

// Default matches the stock loader: fmt.Sprintf("%-16s", "filestash").
const defaultSecret = "filestash"

type license struct {
	Expiry time.Time `json:"expiry"`
	Name   string    `json:"name"`
}

// deriveKey mirrors the loader for short secrets (left-justify to a 16-byte
// AES-128 key) and also accepts full-length 24/32-byte AES-192/256 keys.
func deriveKey(secret string) (string, error) {
	switch {
	case len(secret) <= 16:
		return fmt.Sprintf("%-16s", secret), nil
	case len(secret) == 24 || len(secret) == 32:
		return secret, nil
	default:
		return "", fmt.Errorf("secret is %d bytes; must be <=16 (padded to AES-128) or exactly 24/32 bytes", len(secret))
	}
}

// --- wire format, identical to server/pkg/utils ---------------------------

func compress(data []byte) ([]byte, error) {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func encryptAESGCM(key, plaintext []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decryptAESGCM(key, ciphertext []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

func encryptString(secret, data string) (string, error) {
	d, err := compress([]byte(data))
	if err != nil {
		return "", err
	}
	d, err = encryptAESGCM([]byte(secret), d)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(d), nil
}

func decryptString(secret, data string) (string, error) {
	d, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	d, err = decryptAESGCM([]byte(secret), d)
	if err != nil {
		return "", err
	}
	d, err = decompress(d)
	if err != nil {
		return "", err
	}
	return string(d), nil
}

// --------------------------------------------------------------------------

func main() {
	var (
		name   = flag.String("name", "test", "value stored as the license `name`")
		expiry = flag.String("expiry", "8760h", "validity as a Go duration from now (e.g. 8760h), RFC3339 instant, or 'expired'/'forever'")
		secret = flag.String("secret", "", "encryption secret; overrides $LICENSE_SECRET; defaults to the stock loader key")
		verify = flag.String("verify", "", "decrypt and inspect an existing license string instead of generating")
	)
	flag.Parse()

	// Precedence: -secret flag > $LICENSE_SECRET env > stock default.
	sec := *secret
	if sec == "" {
		sec = os.Getenv("LICENSE_SECRET")
	}
	if sec == "" {
		sec = defaultSecret
	}
	key, err := deriveKey(sec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad -secret: %v\n", err)
		os.Exit(2)
	}

	if *verify != "" {
		doVerify(key, sec, *verify)
		return
	}

	exp, err := parseExpiry(*expiry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad -expiry: %v\n", err)
		os.Exit(2)
	}

	payload, err := json.Marshal(license{Expiry: exp, Name: *name})
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}

	out, err := encryptString(key, string(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "encrypt: %v\n", err)
		os.Exit(1)
	}

	// Self-check: round-trip so we never emit a dud.
	if back, err := decryptString(key, out); err != nil || back != string(payload) {
		fmt.Fprintf(os.Stderr, "self-check failed: err=%v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "secret=%q (keylen=%d) name=%q expiry=%s (valid=%t)\n", sec, len(key), *name, exp.Format(time.RFC3339), time.Now().Before(exp))
	fmt.Fprintf(os.Stderr, "set via env:  LICENSE='%s'\n", out)
	fmt.Fprintf(os.Stderr, "or config general.license:\n")
	fmt.Println(out) // the license string itself, on stdout
}

func parseExpiry(s string) (time.Time, error) {
	switch s {
	case "expired":
		return time.Now().Add(-24 * time.Hour), nil
	case "forever", "max":
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(d), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("want a duration (8760h), RFC3339 instant, or 'expired'/'forever'")
}

func doVerify(key, sec, s string) {
	data, err := decryptString(key, s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decrypt failed with secret %q (keylen=%d): %v\n", sec, len(key), err)
		os.Exit(1)
	}
	var lic license
	if err := json.Unmarshal([]byte(data), &lic); err != nil {
		fmt.Fprintf(os.Stderr, "decrypted but not valid license JSON: %v\nraw: %s\n", err, data)
		os.Exit(1)
	}
	fmt.Printf("name    : %s\n", lic.Name)
	fmt.Printf("expiry  : %s\n", lic.Expiry.Format(time.RFC3339))
	if time.Now().After(lic.Expiry) {
		fmt.Printf("status  : EXPIRED (loader would os.Exit(1))\n")
	} else {
		fmt.Printf("status  : valid for %s\n", time.Until(lic.Expiry).Round(time.Second))
	}
}

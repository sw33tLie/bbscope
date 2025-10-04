package otp

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GenerateTOTP creates a 6-digit TOTP code for the provided base32 secret at time t.
func GenerateTOTP(secret string, t time.Time) (string, error) {
	key, digits, err := parseTOTPSecret(secret)
	if err != nil {
		return "", err
	}
	if digits <= 0 {
		digits = 6
	}
	step := uint64(t.Unix() / 30)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], step)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0F
	code := (uint32(sum[offset])&0x7F)<<24 | (uint32(sum[offset+1])&0xFF)<<16 | (uint32(sum[offset+2])&0xFF)<<8 | (uint32(sum[offset+3]) & 0xFF)
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	code = code % mod
	format := fmt.Sprintf("%%0%dd", digits)
	return fmt.Sprintf(format, code), nil
}

// parseTOTPSecret supports multiple formats:
// - "<digits> <base32>"
// - raw base32 (std/hex, with or without padding)
// - otpauth:// URI
func parseTOTPSecret(s string) ([]byte, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, 0, fmt.Errorf("empty secret")
	}
	if strings.HasPrefix(strings.ToLower(s), "otpauth://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil, 0, err
		}
		q := u.Query()
		sec := q.Get("secret")
		digits := 6
		if d := q.Get("digits"); d != "" {
			if v, err := strconv.Atoi(d); err == nil {
				digits = v
			}
		}
		k, err := decodeBase32Flexible(sec)
		return k, digits, err
	}
	parts := strings.Fields(s)
	digits := 6
	if len(parts) >= 2 {
		if v, err := strconv.Atoi(parts[0]); err == nil {
			digits = v
			s = strings.Join(parts[1:], "")
		}
	}
	k, err := decodeBase32Flexible(s)
	if err != nil {
		return nil, 0, err
	}
	return k, digits, nil
}

func decodeBase32Flexible(sec string) ([]byte, error) {
	raw := strings.TrimSpace(sec)
	if raw == "" {
		return nil, fmt.Errorf("empty secret")
	}
	// Try standard base32 with padding
	if k, err := base32.StdEncoding.DecodeString(strings.ToUpper(raw)); err == nil && len(k) > 0 {
		return k, nil
	}
	// Try standard base32 without padding
	np := strings.TrimRight(strings.ToUpper(raw), "=")
	if k, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(np); err == nil && len(k) > 0 {
		return k, nil
	}
	// Try base32hex
	if k, err := base32.HexEncoding.DecodeString(strings.ToUpper(raw)); err == nil && len(k) > 0 {
		return k, nil
	}
	if k, err := base32.HexEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(np)); err == nil && len(k) > 0 {
		return k, nil
	}
	return nil, fmt.Errorf("unsupported TOTP secret format")
}

package nocpass

import (
	"crypto/rand"
	"math/big"
)

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomPassword returns a cryptographically random alphanumeric string of length n (phase 1: 15).
func RandomPassword(n int) (string, error) {
	if n <= 0 {
		return "", nil
	}
	b := make([]byte, n)
	max := big.NewInt(int64(len(passwordChars)))
	for i := range b {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = passwordChars[v.Int64()]
	}
	return string(b), nil
}

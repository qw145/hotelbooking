package helper

import (
	"crypto/rand"
	"fmt"
)

type RefGenerator interface {
	// NewReference returns a candidate booking reference.
	NewReference() (string, error)
}

const refAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// RefLength is the number of random characters in a reference.
const RefLength = 8

// RefPrefix marks a string as a booking reference at a glance.
const RefPrefix = "BK-"

type CryptoRefGenerator struct{}

// NewReference returns a reference of the form "BK-XXXXXXXX".
func (CryptoRefGenerator) NewReference() (string, error) {
	// Rejection sampling keeps the distribution uniform: 256 is not a multiple
	// of 32, so a plain modulo would bias the first few characters.
	const maxUnbiased = 256 - (256 % len(refAlphabet))

	buf := make([]byte, RefLength*2)
	out := make([]byte, 0, RefLength)

	for len(out) < RefLength {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate booking reference: %w", err)
		}
		for _, b := range buf {
			if int(b) >= maxUnbiased {
				continue
			}
			out = append(out, refAlphabet[int(b)%len(refAlphabet)])
			if len(out) == RefLength {
				break
			}
		}
	}
	return RefPrefix + string(out), nil
}

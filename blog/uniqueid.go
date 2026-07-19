package blog

import (
	"crypto/sha256"
	"fmt"
)

func hashID(input string) string {
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h)[:8]
}

func GenUniqueID(input string) string {
	return hashID(input)
}

func GenAuthorHash(email string) string {
	return hashID(email)
}

func GenDisplayName(name, email string) string {
	if name != "" {
		return name
	}
	return GenAuthorHash(email)
}

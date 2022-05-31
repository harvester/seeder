package util

import (
	"math/rand"
	"time"
)

const (
	charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	defaultLength = 16
)

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func GenerateRand() string {
	return GenerateRandCustomLength(defaultLength)
}

func GenerateRandCustomLength(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

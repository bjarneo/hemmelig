package util

import (
	"math/rand"
	"time"
)

// GenerateRandomNickname generates a random nickname from a predefined list.
func GenerateRandomNickname() string {
	rand.Seed(time.Now().UnixNano())
	names := []string{"Elliot", "Mr. Robot", "Darlene", "Angela", "Tyrell", "Whiterose", "Cisco", "Flippy"}
	return names[rand.Intn(len(names))]
}

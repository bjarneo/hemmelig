package util

import (
	"fmt"
	"math/rand"
	"time"
)

// GenerateRandomNickname generates a random nickname from a predefined list and appends a random tag.
func GenerateRandomNickname() string {
	rand.Seed(time.Now().UnixNano())
	names := []string{
		"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel", "India", "Juliett",
		"Kilo", "Lima", "Mike", "November", "Oscar", "Papa", "Quebec", "Romeo", "Sierra", "Tango",
		"Uniform", "Victor", "Whiskey", "X-ray", "Yankee", "Zulu", "Red", "Blue", "Green", "Gold",
		"Silver", "Bronze", "Ruby", "Sapphire", "Emerald", "Diamond", "Topaz", "Garnet", "Jade", "Opal",
		"Agent", "Rogue", "Cipher", "Specter", "Ghost", "Shadow", "Phantom", "Wraith", "Viper", "Cobra",
	}
	name := names[rand.Intn(len(names))]
	tag := rand.Intn(90000) + 10000 // Generate a 5-digit number
	return fmt.Sprintf("%s#%d", name, tag)
}

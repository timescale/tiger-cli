package util

import (
	"fmt"
	"math/rand"
)

// Matches front-end logic for generating a random service name
func GenerateServiceName() string {
	return fmt.Sprintf("db-%d", 10000+rand.Intn(90000))
}

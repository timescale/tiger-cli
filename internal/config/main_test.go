package config

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	// Replace the system keyring with an in-memory mock so that tests never
	// read, write, or delete real credentials or passwords.
	keyring.MockInit()

	os.Exit(m.Run())
}

package common

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	// Backstop: replace the system keyring with an in-memory mock so that even
	// a test that forgets to reset can never read, write, or delete real
	// credentials or passwords. Per-test isolation comes from the fresh
	// keyring.MockInit() in each test that uses the keyring.
	keyring.MockInit()

	os.Exit(m.Run())
}

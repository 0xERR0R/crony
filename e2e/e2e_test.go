//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"

	_ "github.com/testcontainers/testcontainers-go"
	_ "github.com/testcontainers/testcontainers-go/modules/mockserver"
	_ "github.com/testcontainers/testcontainers-go/network"
)

const defaultCronyImage = "crony-e2e:latest"

var cronyImage string

func TestMain(m *testing.M) {
	cronyImage = os.Getenv("CRONY_IMAGE")
	if cronyImage == "" {
		cronyImage = defaultCronyImage
	}
	fmt.Fprintf(os.Stderr, "e2e: using crony image %q\n", cronyImage)
	os.Exit(m.Run())
}

func TestE2EScaffolding(t *testing.T) {
	if cronyImage == "" {
		t.Fatal("cronyImage was not initialised in TestMain")
	}
}

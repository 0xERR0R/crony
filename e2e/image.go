//go:build e2e

package e2e

import (
	"fmt"
	"os"
)

const defaultCronyImage = "crony-e2e:latest"

var cronyImage = defaultCronyImage

func init() {
	if v := os.Getenv("CRONY_IMAGE"); v != "" {
		cronyImage = v
	}
	fmt.Fprintf(os.Stderr, "e2e: using crony image %q\n", cronyImage)
}

package testpb_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestTestpb bootstraps the Ginkgo suite for the fixture-runtime specs that
// live alongside the plain-Go TestXxx tests in this package.
func TestTestpb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testpb Suite")
}

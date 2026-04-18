package generator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These specs exercise lookupParent directly — hence the whitebox
// package so they can see unexported types (pattern, segment, registry,
// formatString/UUID4). They run inside the generator_test suite bootstrapped
// by suite_test.go: Ginkgo's global spec registry is shared across both
// packages in the same test binary.
var _ = Describe("lookupParent format consistency", func() {
	It("errors when a child segment's format disagrees with its parent", func() {
		parentPat := pattern{
			{Name: "organizations"},
			{Name: "organization", Var: true, Format: formatString},
		}
		childPat := pattern{
			{Name: "organizations"},
			{Name: "organization", Var: true, Format: formatUUID4},
			{Name: "items"},
			{Name: "item", Var: true, Format: formatUUID4},
		}

		reg := newRegistry()
		reg.insert(nil, &resource{
			Type:     resourceType{ServiceName: "example.com", TypeName: "Organization"},
			Patterns: []pattern{parentPat},
		})
		reg.insert(nil, &resource{
			Type:     resourceType{ServiceName: "example.com", TypeName: "Item"},
			Patterns: []pattern{childPat},
		})

		_, err := lookupParent(childPat, reg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`segment "organization"`))
		Expect(err.Error()).To(ContainSubstring(`parent "example.com/Organization"`))
	})

	It("returns a binding when parent and child agree on segment format", func() {
		parentPat := pattern{
			{Name: "organizations"},
			{Name: "organization", Var: true, Format: formatUUID4},
		}
		childPat := pattern{
			{Name: "organizations"},
			{Name: "organization", Var: true, Format: formatUUID4},
			{Name: "items"},
			{Name: "item", Var: true, Format: formatUUID4},
		}

		reg := newRegistry()
		reg.insert(nil, &resource{
			Type:     resourceType{ServiceName: "example.com", TypeName: "Organization"},
			Patterns: []pattern{parentPat},
		})
		reg.insert(nil, &resource{
			Type:     resourceType{ServiceName: "example.com", TypeName: "Item"},
			Patterns: []pattern{childPat},
		})

		got, err := lookupParent(childPat, reg)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).NotTo(BeNil())
	})
})

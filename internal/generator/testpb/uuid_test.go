package testpb_test

import (
	googleuuid "github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/protoc-contrib/protoc-gen-go-resource/internal/generator/testpb/uuid"
)

var _ = Describe("UUID-typed resource names", func() {
	It("parses a well-formed UUID4 segment into a typed struct", func() {
		id := googleuuid.MustParse("11111111-1111-4111-8111-111111111111")

		got, err := uuid.ParseCollectionName("collections/" + id.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(uuid.CollectionName{CollectionID: id}))
		Expect(got.String()).To(Equal("collections/" + id.String()))
	})

	It("wraps the underlying uuid.Parse error and names the segment index", func() {
		_, err := uuid.ParseCollectionName("collections/not-a-uuid")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`parse "collections/not-a-uuid": segment 1:`))
	})

	It("round-trips a UUID id through Format and Parse helpers", func() {
		id := googleuuid.MustParse("44444444-4444-4444-8444-444444444444")

		name := uuid.FormatCollectionName(id)
		Expect(name).To(Equal("collections/" + id.String()))

		got, err := uuid.ParseCollectionID(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(id))
	})

	It("surfaces a parse error from ParseCollectionID when the name is malformed", func() {
		_, err := uuid.ParseCollectionID("collections/not-a-uuid")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`parse "collections/not-a-uuid": segment 1:`))
	})

	It("flows typed parent ids through Parent() without string bridging", func() {
		orgID := googleuuid.MustParse("22222222-2222-4222-8222-222222222222")
		itemID := googleuuid.MustParse("33333333-3333-4333-8333-333333333333")

		child := uuid.ItemName{OrganizationID: orgID, ItemID: itemID}
		Expect(child.Parent()).To(Equal(uuid.OrganizationName{OrganizationID: orgID}))
		Expect(child.String()).To(Equal("organizations/" + orgID.String() + "/items/" + itemID.String()))
	})
})

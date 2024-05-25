// Copyright (c) 2024 David Ouellette.
//
// All rights reserved.
//
// This software and its documentation are proprietary information of David Ouellette.
// No part of this software or its documentation may be copied, transferred, reproduced,
// distributed, modified, or disclosed without the prior written permission of David Ouellette.
//
// Unauthorized use of this software is strictly prohibited and may be subject to civil and
// criminal penalties.
//
// By using this software, you agree to abide by the terms specified herein.

package tcp_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"intelligence/pkg/network/tcp"
)

var _ = Describe("tcp ", func() {
	When("the tcp listener host is an incorrectly formatted IP", func() {
		It("should return an error", func() {
			conn, err := tcp.ResolveAddr("300.300.300.300", 13579)
			Expect(conn).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to format the TCP address (invalid hostname '300.300.300.300')"))
		})
	})

	When("the tcp listener host is an incorrectly formatted hostname", func() {
		It("should return an error", func() {
			conn, err := tcp.ResolveAddr("doesnotexist.doesnotexist", 13579)
			Expect(conn).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no such host"))
		})
	})
})

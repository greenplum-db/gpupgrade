package integrations_test

import (
	. "github.com/onsi/ginkgo"
)

var _ = Describe("upgrade convert master", func() {
	/*
	 * We don't have any integration tests testing the actual behavior of convert
	 * master because that function just performs setup and then calls pg_upgrade,
	 * so the setup logic can be tested in unit tests and pg_upgrade behavior will
	 * be tested in end-to-end tests.
	 *
	 * TODO: Add end-to-end tests for convert master
	 */
})

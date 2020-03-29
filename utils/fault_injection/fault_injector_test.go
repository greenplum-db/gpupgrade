package fault_injector_test

import (
	"fmt"

	fault_injector "github.com/greenplum-db/gpupgrade/utils/fault_injection"
)

func ExampleInsert_second() {
	fault_injector.On()
	fmt.Println(fault_injector.Insert("neverCall", 0))
	// Output: false
}

func ExampleInsert_third() {
	fault_injector.On()
	fmt.Println(fault_injector.Insert("callOnce", 1))
	fmt.Println(fault_injector.Insert("callOnce", 1))
	// Output: true
	// false
}

func ExampleInsert_fourth() {
	fault_injector.On()
	if fault_injector.Insert("callMe", 10) {
		fmt.Println("callMe executing arbitrary code")
	}
	// Output: callMe executing arbitrary code
}

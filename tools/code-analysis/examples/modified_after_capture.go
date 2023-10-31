package main

import (
	"fmt"
)

func modified_after_capture() {
	// Scenario 1: Captured and Modified
	capturedAndModified := "captured"
	func() {
		fmt.Println("Scenario 1:", capturedAndModified)
	}()
	capturedAndModified = "modified"

	// Scenario 2: Captured but Not Modified
	capturedNotModified := "captured"
	func() {
		fmt.Println("Scenario 2:", capturedNotModified)
	}()

	// Scenario 3: Multiple variables
	var1 := "var1"
	var2 := "var2"
	func() {
		fmt.Println("Scenario 3:", var1, var2)
	}()
	var1 = "modified_var1"
	var2 = "modified_var2"

	// Scenario 4: Not captured
	func() {
		fmt.Println("Scenario 4: Not capturing anything.")
	}()
}

func nested_modified_after_capture() {
	// Outermost scope variables
	outerCapturedAndModified := "outer_captured"

	// Scenario 1: Nested Closures
	func() {
		// Middle scope variables
		middleCapturedAndModified := "middle_captured"

		func() {
			// Innermost scope
			fmt.Println("Nested Scenario 1:", outerCapturedAndModified, middleCapturedAndModified)
		}()
		middleCapturedAndModified = "middle_modified"
	}()
	outerCapturedAndModified = "outer_modified"

	// Scenario 2: Functions returning functions
	outerFuncVar := "outer_func_var"

	fn := func() func() {
		innerFuncVar := "inner_func_var"
		return func() {
			fmt.Println("Scenario 2:", outerFuncVar, innerFuncVar)
		}
	}()

	fn()
	outerFuncVar = "outer_func_var_modified"

	// Scenario 3: Combined nested and functions returning functions
	combinedVar := "combined_var"

	fn2 := func() func() {
		return func() {
			fmt.Println("Combined Scenario:", combinedVar)
		}
	}()

	func() func() {
		return func() {
			fmt.Println("Combined Scenario:", combinedVar)
		}
	}()

	fn2()
	combinedVar = "combined_var_modified"
}

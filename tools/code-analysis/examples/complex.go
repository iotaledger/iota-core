package main

import "fmt"

func complex() {
	// Captured variables
	captured1 := 1
	captured2 := 2

	// Non-captured variables
	nonCaptured1 := 10
	nonCaptured2 := 20

	// Nested function with captured variables
	func() {
		fmt.Println("Captured1:", captured1)
		func() {
			fmt.Println("Captured2:", captured2)
		}()
	}()

	// Nested function with argument, not captured
	func(x int, y int) {
		fmt.Println("Non-captured1:", x)
		fmt.Println("Non-captured2:", y)
	}(nonCaptured1, nonCaptured2)

	// Non-captured variables with shadowing
	func() {
		nonCaptured1 := 30
		nonCaptured2 := 40
		fmt.Println("Shadowed non-captured1:", nonCaptured1)
		fmt.Println("Shadowed non-captured2:", nonCaptured2)
	}()

	// Non-captured variable in a loop
	for i := 0; i < 3; i++ {
		go func(n int) {
			fmt.Println("Captured in loop:", n)
		}(i)
	}

	// Captured variable in a loop
	for i := 0; i < 3; i++ {
		go func() {
			fmt.Println("Captured in loop:", i)
		}()
	}

	// Captured by pointer
	func() {
		ptr := &captured1
		fmt.Println("Captured by pointer:", *ptr)
	}()
}

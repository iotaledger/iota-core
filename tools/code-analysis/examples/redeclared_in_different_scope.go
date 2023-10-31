package main

import "fmt"

func redaclered() {
	x := 42

	// Outer closure
	func() {
		fmt.Println(x)
	}()

	x = 2
}

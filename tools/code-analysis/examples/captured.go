package main

import "fmt"

func captured() {
	x := 42

	fn := func() {
		fmt.Println("Captured variable:", x)
	}

	fn()
}

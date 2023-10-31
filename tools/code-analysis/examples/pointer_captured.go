package main

import "fmt"

func pointer_captured() {
	x := 42
	ptr := &x

	fn := func() {
		fmt.Println("Captured pointer:", *ptr)
	}

	fn()
}

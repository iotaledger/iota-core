package main

import "fmt"

func not_captured() {
	fn := func() {
		y := 42
		fmt.Println("Local variable:", y)
	}

	fn()
}

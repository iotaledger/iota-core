package main

import "fmt"

func captured_as_argument() {
	x := 42

	fn := func(y int) {
		fmt.Println("Variable passed as argument:", y)
	}

	fn(x)
}

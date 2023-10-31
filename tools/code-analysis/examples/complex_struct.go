package main

import (
	"fmt"
)

type Animal interface {
	Speak() string
}

type Dog struct {
	Name string
}

func (d Dog) Speak() string {
	return "Woof!"
}

type ClosureHolder struct {
	fn func() string
}

func complex_struct() {
	// Captured variables
	captured := "I'm captured!"
	alsoCaptured := 42

	// Non-captured variables
	notCaptured := "I'm free!"
	alsoNotCaptured := 84

	// Struct with a captured field
	dog := Dog{Name: "Rex"}

	// Nested function with captured variable
	func() {
		fmt.Println(captured)
	}()

	// Interface holding struct with a non-captured field
	var a Animal = Dog{Name: notCaptured}

	// Function capturing a field of a struct
	func() {
		fmt.Println("Captured field: ", dog.Name)
	}()

	// Nested closures, second one capturing a variable from the first
	func() {
		nestedCapture := "I'm also captured!"
		func() {
			fmt.Println(nestedCapture)
			fmt.Println(alsoCaptured)
		}()
	}()

	// Struct capturing a local variable through a closure
	holder := ClosureHolder{
		fn: func() string {
			return captured
		},
	}

	// Using the captured variable through the struct's function field
	fmt.Println("Through struct: ", holder.fn())

	// Interface method call
	func(a Animal) {
		fmt.Println(a.Speak())
	}(a)

	// Non-captured variable passed as a function argument
	func(s string, n int) {
		fmt.Println("Non-captured string:", s)
		fmt.Println("Non-captured integer:", n)
	}(notCaptured, alsoNotCaptured)
}

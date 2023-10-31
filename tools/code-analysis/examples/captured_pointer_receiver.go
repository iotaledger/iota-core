package main

import (
	"fmt"
)

type MyStruct struct {
	field string
}

func (m *MyStruct) MethodWithClosure() {
	capturedInMethod := "captured in method"

	// Closure capturing a variable declared inside the method
	func() {
		fmt.Println(capturedInMethod)
	}()

	// Closure not capturing any variables
	func() {
		fmt.Println("Not capturing anything.")
	}()

	// Closure capturing the method's receiver (should not be reported)
	func() {
		fmt.Println(m.field)
	}()
}

func main() {
	// Variable declarations
	captured := "I am captured!"
	alsoCaptured := "Me too!"
	notCaptured := "I am free!"

	// Closure capturing a variable
	func() {
		fmt.Println(captured)
	}()

	// Closure not capturing any variable
	func() {
		fmt.Println("Nothing captured here.")
	}()

	// Closure capturing multiple variables
	func() {
		fmt.Println(captured)
		fmt.Println(alsoCaptured)
	}()

	// A method that has a closure which captures a variable
	m := &MyStruct{field: "I'm a field!"}
	m.MethodWithClosure()

	// Closure with variables passed as arguments (should not be reported)
	func(nc string) {
		fmt.Println(nc)
	}(notCaptured)
}

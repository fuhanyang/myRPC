package main

import (
	"fmt"
	"reflect"
)

func main() {
	var b int = 10
	var a *int = &b
	typeOfA := reflect.TypeOf(a)

	fmt.Println(typeOfA.NumIn(), "--", typeOfA.Kind(), typeOfA.String())
}

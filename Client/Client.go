package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
)

var target = flag.String("target", ":5080", "target")

var operations = make(map[string]func())

func main() {
	flag.Parse()
	fe := FrontEnd{}
	fe.Connect(*target)
	defer func() {
		fe.conn.Close()
	}()

	operations["q"] = func() {
		os.Exit(0)
	}
	operations["inc"] = func() {
		val := fe.Increment()
		fmt.Printf("New value is %d.\n", val)
	}

	fmt.Printf("Type \"inc\" to increment the value in the system.\n")
	fmt.Printf("Type \"q\" to exit the program.\n")
	reader := bufio.NewReader(os.Stdin)
	for {
		text, _ := reader.ReadString('\n')
		text = text[:len(text)-2] // Cut of newline character.

		if contains(operations, text) {
			operations[text]()
		} else {
			fmt.Println("Unknown command.")
		}
	}
}

func contains(m map[string]func(), key string) bool {
	for k, _ := range m {
		if k == key {
			return true
		}
	}
	return false
}

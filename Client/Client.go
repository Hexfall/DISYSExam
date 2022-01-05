package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var target = flag.String("target", ":5080", "target")

func main() {
	flag.Parse()
	fe := FrontEnd{}
	fe.Connect(*target)
	defer func() {
		fe.conn.Close()
	}()

	fmt.Printf("Type \"set x y\" to assign the value y, to the key x in the system.\n")
	fmt.Printf("Type \"get x\" to retrieve the value stored at x.\n")
	fmt.Printf("Type \"q\" to exit the program.\n")
	reader := bufio.NewReader(os.Stdin)
	for {
		text, _ := reader.ReadString('\n')
		text = text[:len(text)-2] // Cut of newline character.
		text = strings.ToLower(text)

		sep := strings.Split(text, " ")

		switch sep[0] {
		case "q":
			os.Exit(0)
		case "get":
			if len(sep) != 2 {
				fmt.Println("Incorrect number of arguments for command \"get\". Expected 1.")
				break
			}
			x, err := strconv.Atoi(sep[1])
			if err != nil {
				fmt.Println("x must be an integer.")
				break
			}
			fmt.Printf("%d\n", fe.Get(x))
		case "set":
			if len(sep) != 3 {
				fmt.Println("Incorrect number of arguments for command \"set\". Expected 2.")
				break
			}
			x, err1 := strconv.Atoi(sep[1])
			y, err2 := strconv.Atoi(sep[2])
			if err1 != nil || err2 != nil {
				fmt.Println("x and y must be integers.")
				break
			}
			if fe.Set(x, y) {
				fmt.Println("Value successfully stored.")
			} else {
				fmt.Println("Failed to store value.")
			}
		default:
			fmt.Println("Unknown command.")
		}
	}
}

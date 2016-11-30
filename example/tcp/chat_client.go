// +build ignore

package main

import (
	"fmt"
	"net"
	"os"
	"io"
	"gopkg.in/orivil/log.v0"
	"bufio"
)

func main() {

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s name ", os.Args[0])
		os.Exit(1)
	}
	name := os.Args[1]


	conn, err := net.Dial("tcp", "127.0.0.1:8081")
	if err != nil {
		log.Println("fatal error: " + err.Error())
		return
	}
	_, err = io.WriteString(conn, name)
	if err != nil {
		panic(err)
	}

	go func() {
		for {

			in := bufio.NewReader(os.Stdin)

			line, err := in.ReadString('\n')
			if err != nil {
				panic(err)
			}
			conn.Write([]byte(line[0:len(line)-1]))
		}
	}()

	for {
		data := make([]byte, 1024)

		n, err := conn.Read(data)
		if err != nil {
			panic(err)
		}

		data = data[0:n]
		fmt.Print(string(data))
	}
}

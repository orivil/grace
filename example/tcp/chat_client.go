// +build ignore

package main

import (
	"fmt"
	"net"
	"os"
	"io"
	"gopkg.in/orivil/log.v0"
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
		var msg string
		for {

			fmt.Scan(&msg)
			io.WriteString(conn, msg)
		}
	}()

	for {
		data := make([]byte, 1024)

		_, err = conn.Read(data)
		if err != nil {
			panic(err)
		}

		fmt.Print(string(data))
	}
}

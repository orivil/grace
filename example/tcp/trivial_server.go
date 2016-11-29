// +build ignore

package main

import (
	"gopkg.in/orivil/grace.v1"
	"net"
	"log"
	"fmt"
)

func main() {

	grace.ListenSignal()

	err := grace.ListenNetAndServe("tcp", ":8081", func(c net.Conn) {

		for {

			data := make([]byte, 1024)
			c.Read(data)
			fmt.Println(string(data))
		}
	})

	if err != nil {
		log.Fatal(err)
	}
}
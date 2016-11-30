// +build ignore

package main

import (
	"net/http"
	"io"
	"gopkg.in/orivil/grace.v1"
	"log"
)

func main() {

	grace.ListenSignal()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		io.WriteString(w, "hello world!")
	})

	err := grace.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
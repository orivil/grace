# Orivil Graceful Net Listener & Http Server

## Introduction

Package grace provides a graceful net listener and a http server base on the net listener.


## Platform

| OS | Status |
|----------|----------|
| ubuntu 14.04 | worked well |
| windows 7 | test pass(but not graceful) |

## Install

go get -v gopkg.in/orivil/grace.v1

## Example Graceful Tcp Server

```GO
package main

import (
    "gopkg.in/orivil/grace.v1"
    "net"
    "log"
    "fmt"
)

func main() {

    // ListenSignal listens system signals and watches the executable file events.
    // it will automatically restart the server when it got signal or file event.
    //
    // listen signal is an custom option, some times if we need to restart or stop
    // server manually, we can directly use the method Restart() or Stop().
    grace.ListenSignal()

    err := grace.ListenNetAndServe("tcp", ":8081", func(c net.Conn) {

        for {

            data := make([]byte, 1024)
            n, err := c.Read(data)
            if err != nil {
                log.Println(err)
                return
            }
            fmt.Println(string(data[0:n]))
        }
    })

    log.Fatal(err)
}
```

## Example Graceful Http Server

```GO
package main

import (
	"io"
	"log"
	"gopkg.in/orivil/grace.v1"
	"net/http"
)

func main() {

    grace.ListenSignal()
    
	http.HandleFunc("/", func (w http.ResponseWriter, req *http.Request) {
	
        io.WriteString(w, "hello, world!\n")
    })
    
	err := grace.ListenAndServe(":12345", nil)
	if err != nil {
		log.Fatal(err)
	}
}
```

## Graceful Restart With Command

> restart: `kill -HUP $pid`
>
> stop: `kill $pid`


## Automatic Graceful Restart

e.g.:

> open 1st terminal and build project: `go build server.go`
>
> open 2nd terminal and run server: `./server`
>
> open 1st terminal and rebuild build project: `go build server.go`

How does it work?

> the program will detected the executable file event, if the file was updated,
then will start a new child process with the new executable file, and after that 
the parent process will wait to exit until all opened connects closed.

## Contributors

https://github.com/orivil/grace/graphs/contributors

## License

Released under the [MIT License](https://github.com/orivil/grace/blob/master/LICENSE).
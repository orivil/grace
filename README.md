# Orivil Graceful Http Server

## Introduction

Package grace provide a graceful restart http server. From now on, it tested on Linux and Windows.

## Install

go get -v gopkg.in/orivil/grace.v0

## Example

`main.go`:

```GO
package main

import (
    "io"
    "gopkg.in/orivil/grace.v0"
    "gopkg.in/orivil/log.v0"
)

func HelloServer(w http.ResponseWriter, req *http.Request) {
    io.WriteString(w, "hello, world!\n")
}

func main() {

    http.HandleFunc("/hello", HelloServer)
    err := grace.ListenAndServe(":12345", nil)
    if err != nil {
        log.ErrEmergency(err)
    }
}
```

Open first terminal for building the project: `go build main.go`.

Open Second terminal for running the server: `main`(Linux) or `main.exe`(Windows).

After you updated your project, you need to open the first terminal and re-build the project: `go build main.go`.

You can see the graceful restart information in the second terminal.

Or you can open a third terminal to control it(not support on Windows):

> graceful restart: `kill -HUP $pid`
>
> stop server: `kill $pid`


## How does it work

The server has a file watcher, In this example it watches the executable file "main"(on Linux) or "main.exe"(on Windows),
if the file changed, the server will fork a new process and run a new server, if new server is ready,
the old process will be killed.

## Contributors

https://github.com/orivil/grace/graphs/contributors

## License

Released under the [MIT License](https://github.com/orivil/grace/blob/master/LICENSE).
// Copyright 2016 orivil Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package grace provides a graceful net listener, and a graceful http server.
package grace

import (
	"flag"
	"os"
	"os/exec"
	"net"
	"runtime"
	"os/signal"
	"syscall"
	"sync"
	"encoding/json"
	"gopkg.in/orivil/log.v0"
	"github.com/fsnotify/fsnotify"
	"time"
	"fmt"
	"path/filepath"
)

const graceTag = "graceful"

var (
	isChildProcess bool

	osSupportSocketFile bool

	socketFiles []*os.File

	listeners []net.Listener

	socketIndex = make(map[string]uintptr)

	waitGroup = sync.WaitGroup{}

	pid = os.Getpid()

	closeSig = struct {
		closed bool
		sync.RWMutex
	}{}
)

var (
	logf = func(format string, args... interface{}) {

		log.Printf(fmt.Sprintf("[process: %d] ", pid) + format, args...)
	}

	beforeCloseCalls []func()
	afterCloseCalls []func()
)

// BeforeCloseCall caches callbacks, they will be run before the process exited.
// unlike AfterCloseCall, the callbacks will be run before listeners closed.
// most of time, we can pass some notices to the client.
func BeforeCloseCall(callback func()) {

	beforeCloseCalls = append(beforeCloseCalls, callback)
}

// BeforeCloseCall caches callbacks, they will be run before the process exited.
// unlike BeforeCloseCall, the callbacks will be run after all listeners and
// connections closed. most of time, we can backup data here.
func AfterCloseCall(callback func()) {

	afterCloseCalls = append(afterCloseCalls, callback)
}

type supportSocketFile interface {
	File() (f *os.File, err error)
}

type netConn struct {
	net.Conn
}

func (n *netConn) Close() error {

	err := n.Conn.Close()
	waitGroup.Done()
	return err
}

type netListener struct {
	net.Listener
}

var waitForever = make(chan struct{})

func (n *netListener) Accept() (net.Conn, error) {
	closeSig.RLock()
	if closeSig.closed {

		// stop accept new connect.
		<-waitForever
	}
	closeSig.RUnlock()

	c, err := n.Listener.Accept()
	if err != nil {

		// if listener was closed, function "Accept()" will return an
		// error:"use of closed network connection", so cover the error here.
		closeSig.RLock()
		if closeSig.closed {

			<-waitForever
		}
		closeSig.RUnlock()

		return nil, err
	} else {

		waitGroup.Add(1)
		return &netConn{Conn: c}, nil
	}

}
// ListenAndServe listens on the given type network and address and then handle
// the incoming connections.
//
// A trivial example server is:
//
//	package main
//
//	import (
//		"gopkg.in/orivil/grace.v1"
//		"net"
//		"log"
//		"fmt"
//	)
//
//	func main() {
//
//		grace.ListenSignal()
//
//		err := grace.ListenNetAndServe("tcp", ":8081", func(c net.Conn) {
//
//			for {
//
//				data := make([]byte, 1024)
//				c.Read(data)
//				fmt.Println(string(data))
//			}
//		})
//
//		log.Fatal(err)
//	}
//
// ListenAndServe always returns a non-nil error.
func ListenNetAndServe(net, addr string, handler func(net.Conn)) error {

	listener, err := NewListener(net, addr)
	if err != nil {

		return err
	}

	for {

		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		go func() {
			defer conn.Close()
			handler(conn)
		}()
	}
}

func init() {
	flag.BoolVar(&isChildProcess, graceTag, false, "")
	if !flag.Parsed() {
		flag.Parse()
	}

	if isChildProcess {
		logf("initializing...\n")
	}

	switch runtime.GOOS {
	case "windows":
		osSupportSocketFile = false
	default:
		osSupportSocketFile = true
	}

	err := initSocketFiles()
	if err != nil {
		panic(err)
	}
}

func startNewProcess() error {

	logf("starting new process...\n")
	args := os.Args
	path, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	// replace first arg(like "./main") with "-graceful"
	if !isChildProcess {
		args[0] = "-" + graceTag
	} else {
		args = args[1:]
	}

	var pipeReader, pipeWriter *os.File

	if osSupportSocketFile {
		pipeReader, pipeWriter, err = os.Pipe()
		if err != nil {
			return err
		}
	}

	cmd := exec.Command(path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if osSupportSocketFile {
		cmd.ExtraFiles = append([]*os.File{pipeReader}, socketFiles...)
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	if osSupportSocketFile {
		return json.NewEncoder(pipeWriter).Encode(socketIndex)
	}
	return nil
}

func initSocketFiles() error {

	if osSupportSocketFile {

		// read socket files information from the first extra file.
		pipeReader := os.NewFile(3, "pipe-reader")
		defer pipeReader.Close()

		if isChildProcess {

			err := json.NewDecoder(pipeReader).Decode(&socketIndex)
			if err != nil {
				return err
			}

			// get all socket files from parent process.
			for name, idx := range socketIndex {
				f := os.NewFile(idx, name)
				socketFiles = append(socketFiles, f)
			}
		}
	}
	return nil
}

// NewListener returns a graceful net listener
func NewListener(netType, addr string) (l net.Listener, err error) {

	if osSupportSocketFile {

		// handle as child process
		for _, f := range socketFiles {
			if f.Name() == addr {
				l, err = net.FileListener(f)
				if err != nil {
					return nil, err
				}
				l = &netListener{Listener: l}
				listeners = append(listeners, l)
				return
			}
		}

		l, err = net.Listen(netType, addr)
		if err != nil {
			return nil, err
		}

		// handle as parent process
		if sf, ok := l.(supportSocketFile); ok {
			f, err := sf.File()
			if err != nil {
				return nil, err
			}

			// record socket file's "name" & "Fd".
			socketIndex[addr] = uintptr(len(socketFiles) + 4)

			// store socket files
			socketFiles = append(socketFiles, f)
		}

		l = &netListener{Listener: l}
		listeners = append(listeners, l)

		return l, err

	} else {

		return net.Listen(netType, addr)
	}
}

// Restart starts a new process with the same executable file, and wait to exit until
// all opened connects closed.
func Restart() {

	if osSupportSocketFile {

		err := startNewProcess()
		if err != nil {
			logf("start new process failed! %v\n", err)
			// if new process got any error, current process should continue to serve.
			// so prevent to stop the process.
			return
		}

		Stop()
	} else {

		// because must close all net listeners before the new process started. (or will
		// cause the addr already in use error) so if startNewProcess() returns any error,
		// it's too late to handle it.
		BeforeCloseCall(func() {
			err := startNewProcess()
			if err != nil {
				logf("start new process failed! %v\n", err)
			}
		})

		Stop()
	}
}

// Stop will exited the process after all opened connects closed.
func Stop() {

	// stop accept new connect.
	closeSig.Lock()
	closeSig.closed = true
	closeSig.Unlock()

	// run before callbacks
	for _, c := range beforeCloseCalls {

		c()
	}

	logf("wait for close...\n")

	// close all listeners.
	for _, l := range listeners {

		l.Close()
	}

	// wait until all connect closed.
	waitGroup.Wait()

	// run after callbacks
	for _, c := range afterCloseCalls {

		c()
	}

	logf("exited!\n")
	// exit current process.
	os.Exit(0)
}

var once = &sync.Once{}

// ListenSignal listens system signals and watches the executable file events.
// it will automatically restart the server when it got signal or file event.
//
// when process got signal "syscall.SIGHUP"(in linux use command: kill -HUP $pid),
// the old process will use the same executable file to start a new child process
// and wait to exit until all opened connects closed.
//
// when process got signal "syscall.SIGINT" or signal "syscall.SIGTERM", the process
// will wait to exit until all opened connects closed.
//
// when the executable file trigger event "fsnotify.Chmod"(e.g. when rebuild a project,
// this will generate a new executable file and trigger the event), the old process
// will use the new executable file to start a new child process, and wait to exit
// until all opened connects closed.
//
// listen signal is an custom option, some times if we need to restart or stop server
// manually, we can use the method Restart() or Stop() directly.
func ListenSignal() {

	once.Do(func() {

		// listen signals.
		signalChan := make(chan os.Signal)

		signal.Notify(
			signalChan,
			syscall.SIGTERM,
			syscall.SIGHUP,
			syscall.SIGINT,
		)

		go func() {
			sig := <-signalChan
			switch sig {
			case syscall.SIGHUP:
				Restart()
			case syscall.SIGTERM, syscall.SIGINT:
				Stop()
			}
		}()

		// listen file event.
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Printf("grace.ListenSignal(): %v\n", err)
			return
		}

		BeforeCloseCall(func() {

			watcher.Close()
		})

		timer := time.NewTimer(0)
		<-timer.C
		go func() {

			for {
				select {
				case evt := <-watcher.Events:

					switch evt.Op {
					case fsnotify.Chmod, fsnotify.Write:
						timer.Reset(time.Second)
					}
				case err := <-watcher.Errors:
					if err != nil {
						log.Printf("grace.ListenSignal(): %v\n", err)
					}
				}
			}
		}()

		go func() {
			<-timer.C
			Restart()
		}()

		err = watcher.Add(os.Args[0])
		if err != nil {
			log.Printf("grace.ListenSignal(): %v\n", err)
		}
	})
}

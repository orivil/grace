// Copyright 2016 orivil Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package grace provides a graceful restarting http server.
package grace

import (
	"net/http"
	"os"
	"syscall"
	"os/signal"
	"net"
	"fmt"
	"os/exec"
	"crypto/tls"
	"sync"
	"flag"
	"runtime"
	"time"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/orivil/log.v0"
)

const graceTag = "graceful"

var LogF = func(format string, args...interface{}) {

	as := make([]interface{}, len(args) + 1)
	as[0] = pid
	for idx, arg := range args {
		as[idx + 1] = arg
	}
	log.Printf("[process %d] " + format, as...)
}

var ErrF = func(format string, args...interface{}) {

	as := make([]interface{}, len(args) + 1)
	as[0] = pid
	for idx, arg := range args {
		as[idx + 1] = arg
	}
	log.ErrWarnF("[process %d] " + format, as...)
}

// TODO: from now on, only tested on windows and linux.
// IsSupportSocketFile can be overwritten.
var IsSupportSocketFile = func() bool {
	if runtime.GOOS == "windows" {
		return false
	} else {
		return true
	}
}

// IsSupportSignal can be overwritten.
var IsSupportSignal = func() bool {
	if runtime.GOOS == "windows" {
		return false
	} else {
		return true
	}
}

var pid = os.Getpid()

var isChild bool

func init() {
	flag.BoolVar(&isChild, graceTag, false, "")
	if !flag.Parsed() {
		flag.Parse()
	}
}

// ListenAndServe listens on the TCP network address addr
// and then calls Serve with handler to handle requests
// on incoming connections.
// Accepted connections are configured to enable TCP keep-alives.
// Handler is typically nil, in which case the DefaultServeMux is
// used.
//
// A trivial example server is:
//
//	package main
//
//	import (
//		"io"
//		"gopkg.in/orivil/grace.v0"
//		"gopkg.in/orivil/log.v0"
//	)
//
//	// hello world, the web server
//	func HelloServer(w http.ResponseWriter, req *http.Request) {
//		io.WriteString(w, "hello, world!\n")
//	}
//
//	func main() {
//		http.HandleFunc("/hello", HelloServer)
//		err := grace.ListenAndServe(":12345", nil)
//		if err != nil {
// 			log.ErrEmergency(err)
//		}
//	}
//
// If the server is graceful stopped, ListenAndServe will return a nil error.
func ListenAndServe(addr string, h http.Handler) error {

	httpServer := &http.Server{Addr: addr, Handler: h}
	return NewGraceServer(httpServer).ListenAndServe()
}

// ListenAndServeTLS acts identically to ListenAndServe, except that it
// expects HTTPS connections. Additionally, files containing a certificate and
// matching private key for the server must be provided. If the certificate
// is signed by a certificate authority, the certFile should be the concatenation
// of the server's certificate, any intermediates, and the CA's certificate.
//
// A trivial example server is:
//
//	import (
//		"log"
//		"gopkg.in/orivil/grace.v0"
//		"gopkg.in/orivil/log.v0"
//	)
//
//	func handler(w http.ResponseWriter, req *http.Request) {
//		w.Header().Set("Content-Type", "text/plain")
//		w.Write([]byte("This is an example server.\n"))
//	}
//
//	func main() {
//		http.HandleFunc("/", handler)
//		log.Printf("About to listen on 10443. Go to https://127.0.0.1:10443/")
//		err := http.ListenAndServeTLS(":10443", "cert.pem", "key.pem", nil)
//		if err != nil {
// 			log.ErrEmergency(err)
//		}
//	}
//
// One can use generate_cert.go in crypto/tls to generate cert.pem and key.pem.
//
// If the server is graceful stopped, ListenAndServeTLS will return a nil error.
func ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {

	httpServer := &http.Server{Addr: addr, Handler: h}
	return NewGraceServer(httpServer).ListenAndServeTLS(certFile, keyFile)
}

type GraceServer struct {
	*http.Server
	netListener net.Listener
	closeServer chan bool
	watcher *fsnotify.Watcher
}

func NewGraceServer(s *http.Server) *GraceServer {

	LogF("starting new server...\n")
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	return &GraceServer{
		Server: s,
		closeServer: make(chan bool),
		watcher: watcher,
	}
}

func (gs *GraceServer) newListener(addr string) (l net.Listener, err error) {
	if isChild {
		err := gs.killParentProcess()
		if err != nil {
			return nil, err
		} else {
			if IsSupportSocketFile() {
				f := os.NewFile(3, "")
				return net.FileListener(f)
			}
		}
	} else {
		gs.watch()
	}
	return net.Listen("tcp", addr)
}

// If the server is graceful stopped, ListenAndServeTLS will return a nil error.
func (gs *GraceServer) ListenAndServeTLS(certFile, keyFile string) error {
	addr := gs.Addr
	if addr == "" {
		addr = ":https"
	}

	l, err := gs.newListener(addr)
	if err != nil {
		return err
	}

	config := gs.TLSConfig
	if config == nil {
		config = &tls.Config{}
	}
	if !strSliceContains(config.NextProtos, "http/1.1") {
		config.NextProtos = append(config.NextProtos, "http/1.1")
	}

	c, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	config.Certificates = append(config.Certificates, c)
	gs.TLSConfig = config
	tlsListener := tls.NewListener(l, config)
	gs.netListener = tlsListener
	return gs.Serve()
}

// If the server is graceful stopped, ListenAndServe will return a nil error.
func (gs *GraceServer) ListenAndServe() error {
	addr := gs.Addr
	if addr == "" {
		addr = ":http"
	}

	l, err := gs.newListener(addr)
	if err != nil {
		return err
	}
	gs.netListener = l
	return gs.Serve()
}

func (gs *GraceServer) watch() {
	err := gs.watcher.Add(os.Args[0])
	if err != nil {
		ErrF(err.Error())
	}
}

func (gs *GraceServer) killParentProcess() error {

	ppid := os.Getppid()
	process, err := os.FindProcess(ppid)
	if err != nil {
		return err
	}

	LogF("killing parent process [ %d ]", ppid)
	if IsSupportSignal() {
		err = process.Signal(syscall.SIGINT)
	} else {
		err = process.Kill()
	}
	if err != nil {
		return err
	} else {
		wait := func(){
			// wait until parent process exited
			ticker := time.NewTicker(20 * time.Millisecond)
			for _ = range ticker.C {
				state, err := process.Wait()
				if err != nil || state.Exited() {
					gs.watch()
					return
				}
			}
		}

		if IsSupportSignal() {
			go wait()
		} else {
			wait()
		}
		return nil
	}
}

func (gs *GraceServer) Serve() error {
	errChan := make(chan error)
	go func() {
		gs.listenEvents()
		LogF("ready to serve http request...")
		errChan <- gs.Server.Serve(gs.netListener)
	}()

	select {
	case err := <-errChan:
		gs.watcher.Close()
		return err
	case <-gs.closeServer:
		gs.watcher.Close()
		gs.netListener.Close()
		return nil
	}
}

func (gs *GraceServer) listenEvents() {

	var sig os.Signal
	signalChan := make(chan os.Signal)

	signal.Notify(
		signalChan,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGINT,
	)

	go func() {
		for sig = range signalChan {
			switch sig {

			case syscall.SIGTERM, syscall.SIGINT:
				gs.Stop()
				return

			case syscall.SIGHUP:
				gs.Restart()
				return

			default:
				LogF("unknown signal %v", sig)
			}
		}
	}()

	timer := time.NewTimer(0)
	<- timer.C
	go func() {
		for {
			select {
			case evt := <-gs.watcher.Events:
				// only trigger the last event
				if evt.Op & fsnotify.Write == fsnotify.Write {
					timer.Reset(200 * time.Millisecond)
				}
			case err := <-gs.watcher.Errors:
				if err != nil {
					ErrF("grace.GraceServer.listenEvents(): %v", err)
				}
			}
		}
	}()

	go func() {
		for {
			<- timer.C
			gs.Restart()
		}
	}()
}

func (gs *GraceServer) Stop() {

	LogF("closing server...")
	gs.closeServer <- true
}

func (gs *GraceServer) Restart() {

	// un-watch the file, otherwise start new process will got error
	gs.watcher.Remove(os.Args[0])
	LogF("restarting http server...")
	err := gs.startNewProcess()
	if err != nil {

		// re-watch the file
		gs.watcher.Add(os.Args[0])
		ErrF("grace.GraceServer.restart(): %v", err)
		LogF("continue serving")
	}
}

func (gs *GraceServer) getGraceTagArgs() []string {
	res := make([]string, len(os.Args))
	for idx, arg := range os.Args {
		res[idx] = arg
	}
	if !isChild {
		// pass grace tag to the incoming process
		res = append(res, "-" + graceTag)
	}
	return res
}

func (gs *GraceServer) startNewProcess() error {
	if l, ok := gs.netListener.(CanGetFile); ok {
		args := gs.getGraceTagArgs()
		path := args[0]
		args = args[1:]
		cmd := exec.Command(path, args...)

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if IsSupportSocketFile() {
			f, err := l.File()
			if err != nil {
				ErrF("grace.GraceServer.startNewProcess(): %v", err)
			} else {
				cmd.ExtraFiles = []*os.File{f}
			}
		}
		err := cmd.Run()
		if err != nil {
			return err
		}
		return nil
	} else {
		return fmt.Errorf("unknown net listener")
	}
}

type CanGetFile interface {
	File() (f *os.File, err error)
}

// wait group for net listener and opened net connects
var waitGroup = sync.WaitGroup{}

type Listener struct {
	net.Listener
}

func (l *Listener) Accept() (net.Conn, error) {

	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	} else {
		waitGroup.Add(1)
		return NewGracefulConn(c), nil
	}
}

func (l *Listener) Close() error {
	// stop accept new connect
	err := l.Listener.Close()

	// wait until all opened connects closed
	waitGroup.Wait()
	return err
}

type GracefulConn struct {
	net.Conn
}

func NewGracefulConn(c net.Conn) *GracefulConn {
	return &GracefulConn{
		Conn: c,
	}
}

func (w GracefulConn) Close() (e error) {
	e = w.Conn.Close()
	waitGroup.Done()
	return
}

func strSliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
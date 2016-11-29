// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license.

// HTTP server. See RFC 2616.
package grace

import (
	"net/http"
	"net"
	"time"
	"crypto/tls"
)

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*netListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {

	tc, err := ln.netListener.Accept()
	if err != nil {
		return nil, err
	}

	tkc := tc.(*netConn).Conn.(*net.TCPConn)

	tkc.SetKeepAlive(true)
	tkc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}


type Server struct {

	*http.Server
}

// ListenAndServe listens on the TCP network address srv.Addr and then
// calls Serve to handle requests on incoming connections.
// Accepted connections are configured to enable TCP keep-alives.
// If srv.Addr is blank, ":http" is used.
// ListenAndServe always returns a non-nil error.
func (srv *Server) ListenAndServe() error {
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}

	ln, err := NewListener("tcp", srv.Addr)
	if err != nil {
		return err
	}

	return srv.Serve(tcpKeepAliveListener{netListener:ln.(*netListener)})
}

// ListenAndServeTLS listens on the TCP network address srv.Addr and
// then calls Serve to handle requests on incoming TLS connections.
// Accepted connections are configured to enable TCP keep-alives.
//
// Filenames containing a certificate and matching private key for the
// server must be provided if neither the Server's TLSConfig.Certificates
// nor TLSConfig.GetCertificate are populated. If the certificate is
// signed by a certificate authority, the certFile should be the
// concatenation of the server's certificate, any intermediates, and
// the CA's certificate.
//
// If srv.Addr is blank, ":https" is used.
//
// ListenAndServeTLS always returns a non-nil error.
func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":https"
	}

	// Setup HTTP/2 before srv.Serve, to initialize srv.TLSConfig
	// before we clone it and create the TLS Listener.

	// TODO:
	//if err := srv.setupHTTP2_ListenAndServeTLS(); err != nil {
	//	return err
	//}

	config := cloneTLSConfig(srv.TLSConfig)
	if !strSliceContains(config.NextProtos, "http/1.1") {
		config.NextProtos = append(config.NextProtos, "http/1.1")
	}

	configHasCert := len(config.Certificates) > 0 || config.GetCertificate != nil
	if !configHasCert || certFile != "" || keyFile != "" {
		var err error
		config.Certificates = make([]tls.Certificate, 1)
		config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return err
		}
	}

	ln, err := NewListener("tcp", srv.Addr)
	if err != nil {
		return err
	}

	tlsListener := tls.NewListener(tcpKeepAliveListener{netListener:ln.(*netListener)}, config)
	return srv.Serve(tlsListener)
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
// package main
//
// import (
// 	"net/http"
// 	"io"
// 	"gopkg.in/orivil/grace.v1"
// 	"log"
// )
//
// func main() {
//
//   grace.ListenSignal()
//
//	 http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
//
//		 io.WriteString(w, "hello world!")
//	 })
//
//	 err := grace.ListenAndServe(":8080", nil)
//	 log.Fatal(err)
// }
//
// ListenAndServe always returns a non-nil error.
func ListenAndServe(addr string, handler http.Handler) error {
	server := &Server{Server: &http.Server{Addr: addr, Handler: handler}}
	return server.ListenAndServe()
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
//		"net/http"
//		"gopkg.in/orivil/grace.v1"
//	)
//
//	func handler(w http.ResponseWriter, req *http.Request) {
//		w.Header().Set("Content-Type", "text/plain")
//		w.Write([]byte("This is an example server.\n"))
//	}
//
//	func main() {
//
//   	grace.ListenSignal()
//
//		http.HandleFunc("/", handler)
//
//		log.Printf("About to listen on 10443. Go to https://127.0.0.1:10443/")
//
//		err := grace.ListenAndServeTLS(":10443", "cert.pem", "key.pem", nil)
//
//		log.Fatal(err)
//	}
//
// One can use generate_cert.go in crypto/tls to generate cert.pem and key.pem.
//
// ListenAndServeTLS always returns a non-nil error.
func ListenAndServeTLS(addr, certFile, keyFile string, handler http.Handler) error {
	server := &Server{Server: &http.Server{Addr: addr, Handler: handler}}
	return server.ListenAndServeTLS(certFile, keyFile)
}

// cloneTLSConfig returns a shallow clone of the exported
// fields of cfg, ignoring the unexported sync.Once, which
// contains a mutex and must not be copied.
//
// The cfg must not be in active use by tls.Server, or else
// there can still be a race with tls.Server updating SessionTicketKey
// and our copying it, and also a race with the server setting
// SessionTicketsDisabled=false on failure to set the random
// ticket key.
//
// If cfg is nil, a new zero tls.Config is returned.
func cloneTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return &tls.Config{}
	}
	return &tls.Config{
		Rand:                        cfg.Rand,
		Time:                        cfg.Time,
		Certificates:                cfg.Certificates,
		NameToCertificate:           cfg.NameToCertificate,
		GetCertificate:              cfg.GetCertificate,
		RootCAs:                     cfg.RootCAs,
		NextProtos:                  cfg.NextProtos,
		ServerName:                  cfg.ServerName,
		ClientAuth:                  cfg.ClientAuth,
		ClientCAs:                   cfg.ClientCAs,
		InsecureSkipVerify:          cfg.InsecureSkipVerify,
		CipherSuites:                cfg.CipherSuites,
		PreferServerCipherSuites:    cfg.PreferServerCipherSuites,
		SessionTicketsDisabled:      cfg.SessionTicketsDisabled,
		SessionTicketKey:            cfg.SessionTicketKey,
		ClientSessionCache:          cfg.ClientSessionCache,
		MinVersion:                  cfg.MinVersion,
		MaxVersion:                  cfg.MaxVersion,
		CurvePreferences:            cfg.CurvePreferences,
		DynamicRecordSizingDisabled: cfg.DynamicRecordSizingDisabled,
		Renegotiation:               cfg.Renegotiation,
	}
}

func strSliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
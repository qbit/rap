package main

import (
	"flag"
	"log"
	"log/syslog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

var logger, err = syslog.NewLogger(syslog.LOG_INFO|syslog.LOG_DAEMON, 0)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		logger.Printf(
			"%s [%s] %s %d %d %v",
			r.RemoteAddr,
			r.Method,
			r.RequestURI,
			wrapped.statusCode,
			wrapped.size,
			duration,
		)
	})
}

func main() {
	if err != nil {
		log.Fatalln(err)
	}
	targetURL := flag.String("target", "http://localhost:3000", "Target URL to proxy to (e.g., http://localhost:8080)")
	domain := flag.String("domain", "openbsd.app", "Domain name for the certificate")
	certListen := flag.String("http", "127.0.0.1:8080", "Listen string for ACME http server")
	certListen6 := flag.String("http6", "[::1]:8080", "Listen string for ipv6 ACME http server")
	httpsListen := flag.String("https", "127.0.0.1:4343", "Listen string for https server")
	httpsListen6 := flag.String("https6", "[::1]:4343", "Listen string for ipv6 https server")
	flag.Parse()

	target, err := url.Parse(*targetURL)
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 5 * time.Second,
	}

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(*domain),
		Cache:      autocert.DirCache("certs"),
	}

	server := &http.Server{
		Addr:      *httpsListen,
		Handler:   Logger(proxy),
		TLSConfig: certManager.TLSConfig(),
	}
	server6 := &http.Server{
		Addr:      *httpsListen6,
		Handler:   Logger(proxy),
		TLSConfig: certManager.TLSConfig(),
	}

	go http.ListenAndServe(*certListen, certManager.HTTPHandler(nil))
	go http.ListenAndServe(*certListen6, certManager.HTTPHandler(nil))

	log.Printf("Starting reverse proxy for %s on %s\n", *targetURL, *domain)
	go log.Fatal(server.ListenAndServeTLS("", ""))
	log.Fatal(server6.ListenAndServeTLS("", ""))
}

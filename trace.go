// package trace provides convenient functionality to trace HTTP requests.
// Main functionality consists of prepending unique token to all logs related to request.
//
// Basic usage: wrap your top-level http.Handler with trace.Handler and use trace.Log* functions
// instead of log.Print*.
// Use trace.Token to retrieve unique token for request (for example to write it in response body/header).
//
// This library was created to help debugging services that handle multiple concurrent requests and
// be able to extract only relevant logs for it.
package trace

import (
	"bufio"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/context"
)

const requestTokenKey = "_token"

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(status int) {
	sr.status = status
	sr.ResponseWriter.WriteHeader(status)
}

func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := sr.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}

	return nil, nil, errors.New("Hijack not supported")
}

func (sr statusRecorder) getStatus() string {
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	return strconv.Itoa(sr.status) + " " + http.StatusText(sr.status)
}

// Handler wraps h, generating new token for it.
// It also logs request beginning and ending.
// gorilla/context.Clear is called after handler is done.
func Handler(h http.Handler) http.Handler {
	return context.ClearHandler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		token := fmt.Sprintf("%x", md5.Sum([]byte(r.URL.String()+r.RemoteAddr+time.Now().String())))
		context.Set(r, requestTokenKey, token)
		Logln(r, "new request", r.Method, r.URL)
		sr := &statusRecorder{ResponseWriter: rw}
		start := time.Now()
		h.ServeHTTP(sr, r)
		Logln(r, "done, status:", sr.getStatus(), "time:", time.Since(start))
	}))
}

// NoLogHandler is like Handler but it doesn't do any logging.
func NoLogHandler(h http.Handler) http.Handler {
	return context.ClearHandler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		token := fmt.Sprintf("%x", md5.Sum([]byte(r.URL.String()+r.RemoteAddr+time.Now().String())))
		context.Set(r, requestTokenKey, token)
		h.ServeHTTP(rw, r)
	}))
}

// NoClearHandler is like Handler but it doesn't clear gorilla/context.
func NoClearHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		token := fmt.Sprintf("%x", md5.Sum([]byte(r.URL.String()+r.RemoteAddr+time.Now().String())))
		context.Set(r, requestTokenKey, token)
		Logln(r, "new request", r.Method, r.URL)
		sr := &statusRecorder{ResponseWriter: rw}
		start := time.Now()
		h.ServeHTTP(sr, r)
		Logln(r, "done, status:", sr.getStatus(), "time:", time.Since(start))
	})
}

// NoLogClearHandler is like Handler but it doesn't do any logging and doesn't clear gorilla/context.
func NoLogClearHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		token := fmt.Sprintf("%x", md5.Sum([]byte(r.URL.String()+r.RemoteAddr+time.Now().String())))
		context.Set(r, requestTokenKey, token)
		h.ServeHTTP(rw, r)
	})
}

// KVPHandler is like Handler but logs the token as key-value pair.
// This means that instead of
//     [timestamp] [token] [message]
// you will see
//     [timestamp] request_id=[token] [message]
//
// This format is easier to deal with using log parsing systems, such as Splunk.
func KVPHandler(h http.Handler) http.Handler {
	return context.ClearHandler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		token := fmt.Sprintf("request_id=%x", md5.Sum([]byte(r.URL.String()+r.RemoteAddr+time.Now().String())))
		context.Set(r, requestTokenKey, token)
		Logln(r, "new request", r.Method, r.URL)
		sr := &statusRecorder{ResponseWriter: rw}
		start := time.Now()
		h.ServeHTTP(sr, r)
		Logln(r, "done, status:", sr.getStatus(), "time:", time.Since(start))
	}))
}

// Token returns generated token for request or empty string it's not present.
// The returned token is formatted as a key-value pair, e.g.
// "request_id=token". If you need just the token not in KVP form, use
// TokenPlain.
//
// The reason for this to prepend "request_id=" is to match our logging format
// and make log parsing easier.
func Token(r *http.Request) string {
	tok := context.Get(r, requestTokenKey)
	if toks, ok := tok.(string); ok {
		return toks
	}
	return ""
}

// TokenPlain returns generated token for request or empty string it's not present.
// In case token is not formatted correctly, TokenPlain panics.
func TokenPlain(r *http.Request) string {
	tok := context.Get(r, requestTokenKey)
	toks, ok := tok.(string)
	if !ok {
		return ""
	}
	parts := strings.Split(toks, "=")
	if len(parts) != 2 {
		panic("trace: malformed request token: " + toks)
	}
	return parts[1]
}

// Log forwards vals to log.Print and prepends request token
func Log(r *http.Request, vals ...interface{}) {
	tok := Token(r)
	log.Print(append([]interface{}{tok}, vals...)...)
}

// Logln forwards vals to log.Println and prepends request token
func Logln(r *http.Request, vals ...interface{}) {
	tok := Token(r)
	log.Println(append([]interface{}{tok}, vals...)...)
}

// Logf forwards f and vals to log.Printf and prepends request token
func Logf(r *http.Request, f string, vals ...interface{}) {
	tok := Token(r)
	f = "%s " + f
	log.Printf(f, append([]interface{}{tok}, vals...)...)
}

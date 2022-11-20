package main

import (
	"log"
	"net/http"

	"github.com/elazarl/goproxy"
)

func main() {
	log.Println("Starting HN proxy...")
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true

	proxy.OnRequest().HandleConnectFunc(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		// accept only requests to news.ycombinator.com
		if host == "news.ycombinator.com:443" {
			return goproxy.OkConnect, host
		} else {
			return goproxy.RejectConnect, host
		}
	})

	log.Fatal(http.ListenAndServe(":8081", proxy))
}

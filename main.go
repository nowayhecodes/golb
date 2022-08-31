package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	golb "github.com/nowayhecodes/golb/lb"
)

var serverPool golb.ServerPool

func main() {
	var serverList string
	var port int
	flag.StringVar(&serverList, "backends", "", "load balanced backends, use commas to separate")
	flag.IntVar(&port, "port", 3030, "port to serve")
	flag.Parse()

	if len(serverList) == 0 {
		log.Fatal("please provide one or more backends to load balance")
	}

	// parse servers
	tokens := strings.Split(serverList, ",")
	for _, tok := range tokens {
		serverUrl, err := url.Parse(tok)
		if err != nil {
			log.Fatal(err)
		}

		proxy := httputil.NewSingleHostReverseProxy(serverUrl)
		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, e error) {
			log.Printf("[%s] %s\n", serverUrl.Host, e.Error())
			retries := golb.GetRetryFromContext(request)
			if retries < 3 {
				select {
				case <-time.After(10 * time.Millisecond):
					ctx := context.WithValue(request.Context(), golb.Retry, retries+1)
					proxy.ServeHTTP(writer, request.WithContext(ctx))
				}
				return
			}

			// after 3 retries, mark this backend as down
			serverPool.ChangeBackendStatus(serverUrl, false)

			// if the same request routing for few attempts with different backends, increase the count
			attempts := golb.GetAttemptsFromContext(request)
			log.Printf("%s(%s) attempting retry %d\n", request.RemoteAddr, request.URL.Path, attempts)
			ctx := context.WithValue(request.Context(), golb.Attempts, attempts+1)
			golb.LB(writer, request.WithContext(ctx))
		}

		serverPool.AddBackend(&golb.Backend{
			URL:          serverUrl,
			Alive:        true,
			ReverseProxy: proxy,
		})
		log.Printf("configured server: %s\n", serverUrl)
	}

	// create http server
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(golb.LB),
	}

	// start health checking
	go golb.HealthChecker()

	log.Printf("GoLB started at :%d\n", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

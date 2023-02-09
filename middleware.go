package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/julienschmidt/httprouter"

	"github.com/johnwarden/httperror"

	"github.com/gorilla/schema"

	"github.com/NYTimes/gziphandler"
	cache "github.com/victorspringer/http-cache"
	"github.com/victorspringer/http-cache/adapter/memory"
)

var nonCanonicalDomains = map[string]bool{
	"social-protocols-news.fly.dev:443": true,
	"127.0.0.1:8080":                    true, // just for testing
}

const canonicalDomain = "news.social-protocols.org"

// middleware converts a handler of type httperror.XHandlerFunc[P] into an
// httprouter.Handle. We use the former type for our http handler functions:
// this is a clean function signature that accepts parameters as a struct and
// returns an error. But we need to pass an httprouter.Handle to our router.
// So we wrap our httperror.XHandlerFunc[P], parsing the URL parameters to
// produce the parameter struct, passing it to the inner handler, then
// handling any errors that are returned.
func middleware[P any](routeName string, logger leveledLogger, onPanic func(error), h httperror.XHandlerFunc[P]) httprouter.Handle {
	h = httperror.XPanicMiddleware[P](h)

	h = prometheusMiddleware[P](routeName, h)

	handleError := func(w http.ResponseWriter, err error) {
		if errors.Is(err, httperror.Panic) {
			// do this in a goroutine otherwise we get deadline if onPanic shutdowns the HTTP server
			// because the http server shutdown function will wait for all requests to terminate,
			// including this one!
			go onPanic(err)
		}
		httperror.DefaultErrorHandler(w, err)
	}

	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// Handle redirects from non-canonical domains (namely, our fly.dev instance) to
		// the canonical domain.
		if !strings.HasPrefix(r.Host, canonicalDomain) && r.Host != "localhost:8080" {
			logger.Info("Non-canonical domain", "host", r.Host, "uri", r.RequestURI)
			if _, found := nonCanonicalDomains[r.Host]; found {
				url := "https://" + canonicalDomain + r.RequestURI
				logger.Info("Redirecting to", "url", url)
				http.Redirect(w, r, url, 301)
				return
			}
			handleError(w, fmt.Errorf("Invalid request host: %s", r.Host))
			return
		}

		var params P
		err := unmarshalRouterRequest(r, ps, &params)
		if err != nil {
			err = httperror.Wrap(err, http.StatusBadRequest)
			logger.Error("unmarshalRouterRequest", err, "url", r.URL)
			handleError(w, err)
			return
		}

		err = h(w, r, params)
		if err != nil {
			if httperror.StatusCode(err) >= 500 {
				logger.Error("executing handler", err, "url", r.URL)
				requestErrorsTotal.Inc()
			}
			handleError(w, err)
		}
	}
}

var decoder = schema.NewDecoder()

// unmarshalRouterRequest is a generic request URL unmarshaler for use with
// httprouter. It unmarshals the request parameters parsed by httprouter, as
// well as any URL parameters, into a struct of any type, matching query
// names to struct field names.
func unmarshalRouterRequest(r *http.Request, ps httprouter.Params, params any) error {
	m := make(map[string][]string)

	// First convert the httprouter.Params into a map
	for _, p := range ps {
		key := p.Key
		if v, ok := m[key]; ok {
			m[key] = append(v, p.Value)
		} else {
			m[key] = []string{p.Value}
		}
	}

	// Then merge in the URL query parameters.
	for key, values := range r.URL.Query() {
		if v, ok := m[key]; ok {
			m[key] = append(v, values...)
		} else {
			m[key] = values
		}
	}

	// Then unmarshal.
	err := decoder.Decode(params, m)
	if err != nil {
		return errors.Wrap(err, "decode parameters")
	}

	return nil
}

// We could improve this middleware. Currently we cache before we
// compress, because the cache middleware we use here doesn't recognize the
// accept-encoding header, and if we compressed before we cache, cache
// entries would be randomly compressed or not, regardless of the
// accept-encoding header. Unfortunately by caching before we compress,
// requests are cached uncompressed. A compressed-cache middleware would be a
// nice improvement. Also our cache-control headers should be synced with the
// exact cache expiration time, which should be synced with the crawl. But
// what we have here is simple and probably good enough.

func (app app) cacheAndCompressMiddleware(handler http.Handler) http.Handler {
	if app.cacheSize == 0 {
		return handler
	}
	memorycached, err := memory.NewAdapter(
		memory.AdapterWithAlgorithm(memory.LRU),
		memory.AdapterWithCapacity(app.cacheSize),
	)
	if err != nil {
		LogFatal(app.logger, "memory.NewAdapater", err)
	}

	cacheClient, err := cache.NewClient(
		cache.ClientWithAdapter(memorycached),
		cache.ClientWithTTL(1*time.Minute),
		cache.ClientWithRefreshKey("opn"),
	)
	if err != nil {
		LogFatal(app.logger, "cache.NewClient", err)
	}

	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// since we update data only every minute, tell browsers to cache for one minute
		handler.ServeHTTP(w, r)
	})

	h = cacheClient.Middleware(h)

	return gziphandler.GzipHandler(h)
}

package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/julienschmidt/httprouter"

	"github.com/johnwarden/httperror"

	"github.com/gorilla/schema"

	cache "github.com/victorspringer/http-cache"
	"github.com/victorspringer/http-cache/adapter/memory"
)

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
		// since we update data only every minute, tell browsers to cache for one minute
		w.Header().Set("Cache-Control", "public, max-age=60")

		var params P
		err := unmarshalRouterRequest(r, ps, &params)
		if err != nil {
			err = httperror.Wrap(err, http.StatusBadRequest)
			logger.Err(err, "url", r.URL)
			handleError(w, err)
			return
		}

		err = h(w, r, params)
		if err != nil {
			if httperror.StatusCode(err) >= 500 {
				logger.Err(err, "url", r.URL)
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

// cache everything for one minute
func (app app) cacheMiddleware(handler http.Handler) http.Handler {
	if app.cacheSizeBytes == 0 {
		return handler
	}
	memorycached, err := memory.NewAdapter(
		memory.AdapterWithAlgorithm(memory.LRU),
		memory.AdapterWithCapacity(app.cacheSizeBytes),
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cacheClient, err := cache.NewClient(
		cache.ClientWithAdapter(memorycached),
		cache.ClientWithTTL(1*time.Minute),
		cache.ClientWithRefreshKey("opn"),
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return cacheClient.Middleware(handler)
}

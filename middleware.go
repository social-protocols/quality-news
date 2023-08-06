package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/julienschmidt/httprouter"

	"github.com/johnwarden/httperror"

	"github.com/gorilla/schema"

	"github.com/NYTimes/gziphandler"
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

func nullInt64Converter(value string) reflect.Value {
	var result sql.NullInt64
	if value != "" {
		v, _ := strconv.ParseInt(value, 10, 64)
		result = sql.NullInt64{Int64: v, Valid: true}
	}
	return reflect.ValueOf(result)
}

func nullFloat64Converter(value string) reflect.Value {
	var result sql.NullFloat64
	if value != "" {
		v, _ := strconv.ParseFloat(value, 64)
		result = sql.NullFloat64{Float64: v, Valid: true}
	}
	return reflect.ValueOf(result)
}

func init() {
	decoder.RegisterConverter(sql.NullInt64{}, nullInt64Converter)
	decoder.RegisterConverter(sql.NullFloat64{}, nullFloat64Converter)
}

// unmarshalRouterRequest is a generic request URL unmarshaler for use with
// httprouter. It unmarshals the request parameters parsed by httprouter, as
// well as any URL parameters, into a struct of any type, matching query
// names to struct field names.
func unmarshalRouterRequest(r *http.Request, ps httprouter.Params, params any) error {
	if r.Method == "POST" {
		err := json.NewDecoder(r.Body).Decode(params)
		if err != nil {
			return errors.Wrap(err, "decode json")
		}
		return nil
	}

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
		if !strings.HasPrefix(err.Error(), "schema: invalid path") {
			// ignore errors due to unrecognized parameters
			return errors.Wrap(err, "decode parameters")
		}
	}

	return nil
}

// preRouterMiddleware wraps the router itself. It is for middleware that does
// not need to know anything about the route (params, name, etc)
func (app app) preRouterMiddleware(handler http.Handler, writeTimeout time.Duration) http.Handler {
	handler = app.cacheAndCompressMiddleware(handler)
	handler = app.canonicalDomainMiddleware(handler)       // redirects must happen before caching!
	handler = app.timeoutMiddleware(handler, writeTimeout) // redirects must happen before caching!
	return handler
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
	// if app.cacheSize >  0 {

	// 	memorycached, err := memory.NewAdapter(
	// 		memory.AdapterWithAlgorithm(memory.LRU),
	// 		memory.AdapterWithCapacity(app.cacheSize),
	// 	)
	// 	if err != nil {
	// 		LogFatal(app.logger, "memory.NewAdapater", err)
	// 	}

	// 	cacheClient, err := cache.NewClient(
	// 		cache.ClientWithAdapter(memorycached),
	// 		cache.ClientWithTTL(1*time.Minute),
	// 		cache.ClientWithRefreshKey("opn"),
	// 	)
	// 	if err != nil {
	// 		LogFatal(app.logger, "cache.NewClient", err)
	// 	}

	// 	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 		// since we update data only every minute, tell browsers to cache for one minute
	// 		handler.ServeHTTP(w, r)
	// 	})

	// 	h = cacheClient.Middleware(h)
	// }
	h := handler

	return gziphandler.GzipHandler(h)
}

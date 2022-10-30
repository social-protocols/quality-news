package main

import (
	"net/http"

	"github.com/pkg/errors"

	"github.com/julienschmidt/httprouter"

	"github.com/johnwarden/httperror"
)

// middleware converts a handler of type httperror.XHandlerFunc[P] into an
// httprouter.Handle. We use the former type for our http handler functions:
// this is a clean function signature that accepts parameters as a struct and
// returns an error. But we need to pass an httprouter.Handle to our router.
// So we wrap our httperror.XHandlerFunc[P], parsing the URL parameters to
// produce the parameter struct, passing it to the inner handler, then
// handling any errors that are returned.
func middleware[P any](logger leveledLogger, h httperror.XHandlerFunc[P]) httprouter.Handle {

	h = httperror.XPanicMiddleware[P](h)

	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

		var params P
		err := unmarshalRouterRequest(r, ps, &params)
		if err != nil {
			err = httperror.Wrap(err, http.StatusBadRequest)
			logger.Err(err, "url", r.URL)
			httperror.DefaultErrorHandler(w, err)
			return
		}

		err = h(w, r, params)
		if err != nil {
			logger.Err(err)
			httperror.DefaultErrorHandler(w, err)
		}
	}
}

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

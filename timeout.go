package main

import (
	"context"
	"net/http"
	"time"
)

func (app app) timeoutMiddleware(handler http.Handler, timeoutSeconds time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeoutSeconds)
		defer cancel()
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

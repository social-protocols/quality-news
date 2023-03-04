package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/johnwarden/httperror"
)

var nonCanonicalDomains = map[string]string{
	"social-protocols-news.fly.dev": "news.social-protocols.org",
	"127.0.0.1:8080":                "localhost:8080", // just for testing
}

var canonicalDomains = getValues(nonCanonicalDomains)

func (app app) canonicalDomainMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.Debug("redirectNonCanonicalURLs", "host", r.Host)
		// Redirect any non-canonical domain to the corresponding canonical domain.
		for nonCanonicalDomain, canonicalDomain := range nonCanonicalDomains {
			if r.Host == nonCanonicalDomain {
				url := "https://" + canonicalDomain + r.RequestURI
				http.Redirect(w, r, url, http.StatusMovedPermanently)
				return
			}
		}
		isCanonical := false
		for _, canonicalDomain := range canonicalDomains {
			if strings.HasPrefix(r.Host, canonicalDomain) {
				isCanonical = true
				break
			}
		}
		if !isCanonical {
			httperror.DefaultErrorHandler(w, httperror.New(http.StatusForbidden, fmt.Sprintf("Invalid request host: %s", r.Host)))
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func (app app) testRedirectHandler() func(http.ResponseWriter, *http.Request, struct{}) error {
	return func(w http.ResponseWriter, r *http.Request, _ struct{}) error {
		_, _ = w.Write([]byte("Hello, World!\n"))
		return nil
	}
}

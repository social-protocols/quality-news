package main

import (
	"database/sql"
	"math/rand"
	"net/http"
	"strconv"

	"github.com/johnwarden/httperror"
	"github.com/pkg/errors"
)

func (app app) getUserID(r *http.Request) sql.NullInt64 {
	var id sql.NullInt64

	cookie, err := r.Cookie("userID")
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) {
			app.logger.Error("r.Cookie('UserID'", err)
		}
		return id
	}

	idInt, err := strconv.Atoi(cookie.Value)
	if err != nil {
		app.logger.Error("Parsing cookie", err)
	}

	id.Int64 = int64(idInt)
	id.Valid = true

	return id
}

type loginParams struct {
	UserID sql.NullInt64
}

func (app app) loginHandler() func(http.ResponseWriter, *http.Request, loginParams) error {
	return func(w http.ResponseWriter, r *http.Request, p loginParams) error {
		userID := p.UserID

		if !userID.Valid {
			loggedInUserID := app.getUserID(r)
			if loggedInUserID.Valid {
				http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
				return nil
			}

			// Assign a random user ID if none specified as parameter
			userID.Int64 = rand.Int63()
			userID.Valid = true
		}

		if userID.Int64 == 0 {
			return httperror.PublicErrorf(http.StatusUnauthorized, "Can't login as user 0")
		}

		setUserIDCookie(w, userID)

		http.Redirect(w, r, "/score", http.StatusTemporaryRedirect)

		return nil
	}
}

func (app app) logoutHandler() func(http.ResponseWriter, *http.Request, struct{}) error {
	return func(w http.ResponseWriter, r *http.Request, p struct{}) error {
		var userID sql.NullInt64
		setUserIDCookie(w, userID)

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)

		return nil
	}
}

func setUserIDCookie(w http.ResponseWriter, userID sql.NullInt64) {
	value := strconv.Itoa(int(userID.Int64))
	maxAge := 365 * 24 * 60 * 60
	if !userID.Valid {
		maxAge = -1
		value = ""
	}

	cookie := http.Cookie{
		Name:     "userID",
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}

	// Use the http.SetCookie() function to send the cookie to the client.
	// Behind the scenes this adds a `Set-Cookie` header to the response
	// containing the necessary cookie data.
	http.SetCookie(w, &cookie)
}

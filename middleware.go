package main

import (
	"net/http"

	"github.com/codegangsta/negroni"
)

func method(method string) negroni.Handler {
	handler := func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		if r.Method != method {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
	return negroni.HandlerFunc(handler)
}

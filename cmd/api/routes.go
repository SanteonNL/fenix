package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	// Initialize a new httprouter router instance.
	router := httprouter.New()

	// Convert the notFoundResponse() helper to a http.Handler using the
	// http.HandlerFunc() adapter, and then set it as the custom error handler for 404
	// Not Found responses.
	router.NotFound = http.HandlerFunc(app.notFoundResponse)

	// Likewise, convert the methodNotAllowedResponse() helper to a http.Handler and set
	// it as the custom error handler for 405 Method Not Allowed responses.
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	// Register the relevant methods, URL patterns and handler functions for our
	// endpoints using the HandlerFunc() method. Note that http.MethodGet and
	// http.MethodPost are constants which equate to the strings "GET" and "POST"
	// respectively.
	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)
	// TODO: Check if this is relevant; maybe for SIMonFHIR.ndjson input, waarbij je
	// ook json input moet vertalen naar go structs waarschijnlijk. Zie email Bram 8-2-2024
	// (plaatje input/output)
	router.HandlerFunc(http.MethodPost, "/v1/movies", app.createMovieHandler)
	router.HandlerFunc(http.MethodGet, "/v1/movies/:id", app.showMovieHandler)

	// TODO: Let op dat recoverPanic() alleen wordt gebruikt in routes.go en dat
	// dat panics in evt. toekomstige andere go routines niet worden afgevangen.
	// Zie p. 75 van Let's go further.
	// Return the httprouter instance.
	// Wrap the router with the panic recovery middleware.
	return app.recoverPanic(router)
}

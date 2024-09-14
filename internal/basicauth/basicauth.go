// Package basicauth provides an HTTP middleware handler enforcing
// Basic Authentication.
//
// Reference: https://www.alexedwards.net/blog/basic-authentication-in-go
package basicauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"sync"

	"github.com/jakewan/sudsy/internal/common"
)

type handler struct {
	next                 http.Handler
	expectedUsernameHash [32]byte
	expectedPasswordHash [32]byte
	realm                string
}

// AfterShutdown implements common.MiddlewareHandler.
func (h *handler) AfterShutdown() {}

// BeforeStart implements common.MiddlewareHandler.
func (h *handler) BeforeStart(*sync.WaitGroup) {}

// ServeHTTP implements http.Handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// CORS preflight requests exclude credentials.
	if req.Method == "OPTIONS" {
		h.next.ServeHTTP(w, req)
	}
	username, password, ok := req.BasicAuth()
	if ok {
		usernameHash := sha256.Sum256([]byte(username))
		passwordHash := sha256.Sum256([]byte(password))

		// Use the subtle.ConstantTimeCompare() function to check if
		// the provided username and password hashes equal the
		// expected username and password hashes. ConstantTimeCompare
		// will return 1 if the values are equal, or 0 otherwise.
		// Importantly, we should to do the work to evaluate both the
		// username and password before checking the return values to
		// avoid leaking information.
		usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], h.expectedUsernameHash[:]) == 1)
		passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], h.expectedPasswordHash[:]) == 1)

		if usernameMatch && passwordMatch {
			h.next.ServeHTTP(w, req)
			return
		}
	}
	w.Header().Set(
		"www-authenticate",
		fmt.Sprintf(`Basic realm="%s", charset="UTF-8"`, h.realm),
	)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

func NewMiddlewareHandler(
	next http.Handler,
	username string,
	password string,
	realm string,
) common.MiddlewareHandler {
	result := handler{
		next:                 next,
		expectedUsernameHash: sha256.Sum256([]byte(username)),
		expectedPasswordHash: sha256.Sum256([]byte(password)),
		realm:                realm,
	}
	return &result
}

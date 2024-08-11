package common

import (
	"net/http"
	"sync"
)

type MiddlewareHandler interface {
	http.Handler
	AfterShutdown()
	BeforeStart(*sync.WaitGroup)
}

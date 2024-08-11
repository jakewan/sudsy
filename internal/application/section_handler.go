package application

import (
	"net/http"
	"slices"
	"sync"

	"github.com/jakewan/sudsy/internal/common"
	"github.com/jakewan/sudsy/internal/urlpathpatternhandler"
)

type sectionHandlerDependencies struct {
	StatusNotFoundHandlerFunc http.HandlerFunc
}

type sectionHandler struct {
	deps                   sectionHandlerDependencies
	urlPathPatternHandlers []urlpathpatternhandler.Handler
}

// AfterShutdown implements MiddlewareHandler.
func (s *sectionHandler) AfterShutdown() {}

// BeforeStart implements MiddlewareHandler.
func (s *sectionHandler) BeforeStart(*sync.WaitGroup) {}

// ServeHTTP implements http.Handler.
func (s *sectionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger.Debug("", "Inside sectionHandler.ServeHTTP: %s", r.URL.Path)
	if idx, found := slices.BinarySearchFunc(
		s.urlPathPatternHandlers,
		r.URL.Path,
		urlpathpatternhandler.ComparePatternHandlerToPath,
	); found {
		logger.Debug("", "Found handler at index %d", idx)
		s.urlPathPatternHandlers[idx].ServeHTTP(w, r)
	} else {
		logger.Debug("", "Handler not found")
		if s.deps.StatusNotFoundHandlerFunc != nil {
			s.deps.StatusNotFoundHandlerFunc(w, r)
		} else {
			w.WriteHeader(http.StatusNotFound)
			if _, err := w.Write([]byte("Not found")); err != nil {
				logger.Debug("", "Error writing response: %s", err)
			}
		}
	}
}

func newSectionHandler(deps sectionHandlerDependencies, handlers []urlpathpatternhandler.Handler) common.MiddlewareHandler {
	return &sectionHandler{
		deps:                   deps,
		urlPathPatternHandlers: handlers,
	}
}

package application

import (
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/jakewan/sudsy/internal/basicauth"
	"github.com/jakewan/sudsy/internal/common"
	"github.com/jakewan/sudsy/internal/ratelimiting"
	"github.com/jakewan/sudsy/internal/urlpathpatternhandler"
)

type HandlerFuncWithError func(http.ResponseWriter, *http.Request, error)

type Section interface {
	AddPathPatternHandler(pattern string, handler http.Handler, contextKey any)
	AddRateLimitingSessionConfig(maxRequests int64, sessionDuration, banDuration time.Duration)
	AfterShutdown()
	BeforeStart(*sync.WaitGroup)
	NewHandler() http.Handler
	Root() string
	SetBasicAuthPassword(string)
	SetBasicAuthRealm(string)
	SetBasicAuthUsername(string)
	SetRateLimitingHostCacheEntryIdleDuration(time.Duration)
	SetStatusBadRequestHandlerFunc(HandlerFuncWithError)
	SetStatusNotFoundHandlerFunc(http.HandlerFunc)
	SetStatusTooManyRequestsHandlerFunc(http.HandlerFunc)
}

type SectionDependencies interface {
	Now() time.Time
}

type sectionRateLimitingConfig struct {
	maxRequests     int64
	sessionDuration time.Duration
	banDuration     time.Duration
}

type section struct {
	deps SectionDependencies

	statusBadRequestHandlerFunc HandlerFuncWithError

	statusNotFoundHandlerFunc http.HandlerFunc

	statusTooManyRequestsHandlerFunc http.HandlerFunc

	urlPathPatternHandlers []urlpathpatternhandler.Handler

	rateLimitingHostCacheEntryIdleDuration time.Duration

	activeMiddlewareHandlers []common.MiddlewareHandler

	rateLimitingConfigs []sectionRateLimitingConfig

	root string

	basicAuthUsername string

	basicAuthPassword string

	basicAuthRealm string
}

// AddPathPatternHandler implements Section.
func (s *section) AddPathPatternHandler(
	pattern string,
	handler http.Handler,
	contextKey any,
) {
	patternHandler := urlpathpatternhandler.NewHandler(pattern, handler, contextKey)
	s.urlPathPatternHandlers = append(s.urlPathPatternHandlers, patternHandler)
	if err := urlpathpatternhandler.ValidateResponders(
		s.urlPathPatternHandlers,
	); err != nil {
		panic(err)
	}
	slices.SortFunc(
		s.urlPathPatternHandlers,
		urlpathpatternhandler.ComparePatternHandlers,
	)
}

// AddRateLimitingSessionConfig implements Section.
func (s *section) AddRateLimitingSessionConfig(maxRequests int64, sessionDuration time.Duration, banDuration time.Duration) {
	s.rateLimitingConfigs = append(s.rateLimitingConfigs, sectionRateLimitingConfig{
		maxRequests:     maxRequests,
		sessionDuration: sessionDuration,
		banDuration:     banDuration,
	})
}

// AfterShutdown implements Section.
func (s *section) AfterShutdown() {
	for _, h := range s.activeMiddlewareHandlers {
		h.AfterShutdown()
	}
}

// BeforeStart implements Section.
func (s *section) BeforeStart(wg *sync.WaitGroup) {
	for i := len(s.activeMiddlewareHandlers) - 1; i >= 0; i-- {
		s.activeMiddlewareHandlers[i].BeforeStart(wg)
	}
}

// Root implements Section.
func (s *section) Root() string {
	return s.root
}

// SetBasicAuthPassword implements Section.
func (s *section) SetBasicAuthPassword(password string) {
	s.basicAuthPassword = password
}

// SetBasicAuthRealm implements Section.
func (s *section) SetBasicAuthRealm(realm string) {
	s.basicAuthRealm = realm
}

// SetBasicAuthUsername implements Section.
func (s *section) SetBasicAuthUsername(username string) {
	s.basicAuthUsername = username
}

// SetRateLimitingHostCacheEntryIdleDuration implements Section.
func (s *section) SetRateLimitingHostCacheEntryIdleDuration(d time.Duration) {
	s.rateLimitingHostCacheEntryIdleDuration = d
}

// SetStatusBadRequestHandlerFunc implements Section.
func (s *section) SetStatusBadRequestHandlerFunc(h HandlerFuncWithError) {
	s.statusBadRequestHandlerFunc = h
}

// SetStatusNotFoundHandlerFunc implements Section.
func (s *section) SetStatusNotFoundHandlerFunc(h http.HandlerFunc) {
	s.statusNotFoundHandlerFunc = h
}

// SetStatusTooManyRequestsHandlerFunc implements Section.
func (s *section) SetStatusTooManyRequestsHandlerFunc(h http.HandlerFunc) {
	s.statusTooManyRequestsHandlerFunc = h
}

func (s *section) NewHandler() http.Handler {
	logger.Debug("", "Creating HTTP handler for %+v", s)
	var outermost common.MiddlewareHandler
	outermost = newSectionHandler(
		s.newSectionHandlerDependencies(),
		s.urlPathPatternHandlers,
	)
	s.activeMiddlewareHandlers = append(s.activeMiddlewareHandlers, outermost)
	if s.basicAuthUsername != "" && s.basicAuthPassword != "" && s.basicAuthRealm != "" {
		outermost = basicauth.NewMiddlewareHandler(
			outermost,
			s.basicAuthUsername,
			s.basicAuthPassword,
			s.basicAuthRealm,
		)
		s.activeMiddlewareHandlers = append(s.activeMiddlewareHandlers, outermost)
	} else {
		logger.Debug("", "Basic auth not configured")
	}
	if len(s.rateLimitingConfigs) > 0 {
		outermost = func() common.MiddlewareHandler {
			h := ratelimiting.NewMiddlewareHandler(
				s.newRateLimitingDependencies(),
				outermost,
			)
			for _, c := range s.rateLimitingConfigs {
				h.AddSessionConfig(c.maxRequests, c.sessionDuration, c.banDuration)
			}
			if s.rateLimitingHostCacheEntryIdleDuration > 0 {
				h.SetHostCacheEntryIdleDuration(s.rateLimitingHostCacheEntryIdleDuration)
			}
			return h
		}()
		s.activeMiddlewareHandlers = append(s.activeMiddlewareHandlers, outermost)
	} else {
		logger.Debug("", "Rate limiting not configured")
	}
	return outermost
}

func (s *section) newRateLimitingDependencies() ratelimiting.Dependencies {
	return &rateLimitingDependencies{
		statusBadRequestHandlerFunc:      s.statusBadRequestHandlerFunc,
		statusTooManyRequestsHandlerFunc: s.statusTooManyRequestsHandlerFunc,
		now:                              s.deps.Now,
	}
}

func (s *section) newSectionHandlerDependencies() sectionHandlerDependencies {
	return sectionHandlerDependencies{
		StatusNotFoundHandlerFunc: s.statusNotFoundHandlerFunc,
	}
}

func NewSection(deps SectionDependencies, root string) Section {
	return &section{
		deps: deps,
		root: root,
	}
}

type rateLimitingDependencies struct {
	statusBadRequestHandlerFunc      HandlerFuncWithError
	statusTooManyRequestsHandlerFunc http.HandlerFunc
	now                              func() time.Time
}

// HandleStatusBadRequest implements ratelimiting.Dependencies.
func (r *rateLimitingDependencies) HandleStatusBadRequest(w http.ResponseWriter, req *http.Request, err error) {
	if r.statusBadRequestHandlerFunc != nil {
		r.statusBadRequestHandlerFunc(w, req, err)
	} else {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("Bad Request")); err != nil {
			logger.Debug("", "Error writing response: %s", err)
		}
	}
}

// HandleStatusTooManyRequests implements ratelimiting.Dependencies.
func (r *rateLimitingDependencies) HandleStatusTooManyRequests(w http.ResponseWriter, req *http.Request) {
	if r.statusTooManyRequestsHandlerFunc != nil {
		r.statusTooManyRequestsHandlerFunc(w, req)
	} else {
		w.WriteHeader(http.StatusTooManyRequests)
		if _, err := w.Write([]byte("Too Many Requests")); err != nil {
			logger.Debug("", "Error writing response: %s", err)
		}
	}
}

// Now implements ratelimiting.Dependencies.
func (r *rateLimitingDependencies) Now() time.Time {
	return r.now()
}

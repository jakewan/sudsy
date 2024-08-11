package sudsy

import (
	"net/http"
	"time"

	"github.com/jakewan/sudsy/internal/application"
)

type Application interface {
	AddApplicationSection(section application.Section) error
	ListenAndServe()
}

type applicationSectionOpt func(application.Section)

func NewApplicationSection(
	root string,
	opts ...applicationSectionOpt,
) application.Section {
	s := application.NewSection(
		newApplicationSectionDependencies(),
		root,
	)
	for _, o := range opts {
		o(s)
	}
	return s
}

func WithBasicAuth(username, password, realm string) applicationSectionOpt {
	return func(s application.Section) {
		s.SetBasicAuthUsername(username)
		s.SetBasicAuthPassword(password)
		s.SetBasicAuthRealm(realm)
	}
}

func WithPathPatternHandler(
	pattern string,
	handler http.Handler,
	contextKey any,
) applicationSectionOpt {
	return func(s application.Section) {
		s.AddPathPatternHandler(pattern, handler, contextKey)
	}
}

func WithRateLimitingHostCacheEntryIdleDuration(d time.Duration) applicationSectionOpt {
	return func(s application.Section) {
		s.SetRateLimitingHostCacheEntryIdleDuration(d)
	}
}

func WithRateLimitingSessionConfig(
	maxRequests int64,
	sessionDuration time.Duration,
	banDuration time.Duration,
) applicationSectionOpt {
	return func(s application.Section) {
		s.AddRateLimitingSessionConfig(maxRequests, sessionDuration, banDuration)
	}
}

func WithStatusBadRequestHandlerFunc(h application.HandlerFuncWithError) applicationSectionOpt {
	return func(s application.Section) {
		s.SetStatusBadRequestHandlerFunc(h)
	}
}

func WithStatusNotFoundHandlerFunc(h http.HandlerFunc) applicationSectionOpt {
	return func(s application.Section) {
		s.SetStatusNotFoundHandlerFunc(h)
	}
}

func WithStatusTooManyRequestsHandlerFunc(h http.HandlerFunc) applicationSectionOpt {
	return func(s application.Section) {
		s.SetStatusTooManyRequestsHandlerFunc(h)
	}
}

type applicationWrapper struct {
	application application.Application
}

// AddApplicationSection implements Application.
func (a *applicationWrapper) AddApplicationSection(section application.Section) error {
	return a.application.AddSection(section)
}

// ListenAndServe implements Application.
func (a *applicationWrapper) ListenAndServe() {
	a.application.ListenAndServe()
}

func NewApplication() Application {
	return &applicationWrapper{
		application: application.NewApplication(),
	}
}

type applicationSectionDependencies struct{}

// Now implements application.SectionDependencies.
func (a *applicationSectionDependencies) Now() time.Time {
	return time.Now()
}

func newApplicationSectionDependencies() application.SectionDependencies {
	return &applicationSectionDependencies{}
}

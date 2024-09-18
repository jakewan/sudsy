package application

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/jakewan/sudsy/internal/common"
	"github.com/vardius/shutdown"
)

var (
	logger = common.NewLogger("application")
)

type Application interface {
	AddAfterShutdownFunc(f func())
	AddBeforeShutdownFunc(f func())
	AddSection(Section) error
	ListenAndServe()
	SetServerListenPort(int)
}

type application struct {
	afterShutDownFuncs  []func()
	beforeShutDownFuncs []func()
	sections            []Section
	serverListenPort    int
}

// AddAfterShutdownFunc implements Application.
func (a *application) AddAfterShutdownFunc(f func()) {
	a.afterShutDownFuncs = append(a.afterShutDownFuncs, f)
}

// AddBeforeShutdownFunc implements Application.
func (a *application) AddBeforeShutdownFunc(f func()) {
	a.beforeShutDownFuncs = append(a.beforeShutDownFuncs, f)
}

// SetServerListenPort implements Application.
func (a *application) SetServerListenPort(port int) {
	a.serverListenPort = port
}

func (a *application) AddSection(s Section) error {
	rootsObserved := []string{}
	for _, s := range a.sections {
		rootsObserved = append(rootsObserved, s.Root())
	}
	if slices.Contains(rootsObserved, s.Root()) {
		return fmt.Errorf("duplicate section found for root %s", s.Root())
	}
	a.sections = append(a.sections, s)
	return nil
}

func (a *application) ListenAndServe() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	for _, s := range a.sections {
		mux.Handle(s.Root(), s.NewHandler())
	}

	httpServer := &http.Server{
		Addr:        fmt.Sprintf(":%d", a.serverListenPort),
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	stop := func() {
		// Process anything the caller would like to do before shutting down.
		for _, f := range a.beforeShutDownFuncs {
			f()
		}

		gracefulCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(gracefulCtx); err != nil {
			logger.Debug("", "shutdown error: %v", err)
		} else {
			logger.Debug("", "gracefully stopped")
		}

		// Process anything the caller would like to do after shutting down.
		for _, f := range a.afterShutDownFuncs {
			f()
		}
	}

	// Run server.
	go func() {
		// Start async processes.
		var wg sync.WaitGroup
		for _, s := range a.sections {
			s.BeforeStart(&wg)
		}

		// Start the HTTP server.
		err := httpServer.ListenAndServe()
		var exitCode int
		if err != http.ErrServerClosed {
			logger.Debug("", "ListenAndServe responded with unexpected error: %s", err)
			exitCode = 1
		}

		// Stop async processess and wait for them to complete.
		for _, s := range a.sections {
			s.AfterShutdown()
		}
		wg.Wait()

		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	startedAt := time.Now()
	logger.Debug("", "Server started at %s", startedAt.Format(time.RFC3339))

	// Block until the shutdown signal is received.
	shutdown.GracefulStop(stop)
}

func NewApplication() Application {
	return &application{
		afterShutDownFuncs:  []func(){},
		beforeShutDownFuncs: []func(){},
		sections:            []Section{},
		serverListenPort:    8080,
	}
}

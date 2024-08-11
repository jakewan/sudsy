package ratelimiting

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/jakewan/sudsy/internal/common"
)

var logger = common.NewLogger("ratelimiting")

func NewMiddlewareHandler(deps Dependencies, next http.Handler) MiddlewareHandler {
	result := handler{
		deps:                       deps,
		next:                       next,
		remoteHosts:                map[string]clientEntry{},
		hostCacheLocker:            &sync.Mutex{},
		sessionConfigs:             []sessionConfig{},
		hostCacheEntryIdleDuration: 20 * time.Minute,
	}
	return &result
}

type Dependencies interface {
	Now() time.Time
	HandleStatusBadRequest(http.ResponseWriter, *http.Request, error)
	HandleStatusTooManyRequests(http.ResponseWriter, *http.Request)
}

type MiddlewareHandler interface {
	common.MiddlewareHandler
	AddSessionConfig(maxRequests int64, sessionDuration, banDuration time.Duration)
	SetHostCacheEntryIdleDuration(d time.Duration)
}

type sessionConfig struct {
	banDuration     time.Duration
	sessionDuration time.Duration
	maxRequests     int64
}

type handler struct {
	deps Dependencies

	next http.Handler

	// remoteHosts maps hosts (usually remote IP addresses) to client entries.
	remoteHosts map[string]clientEntry

	hostCacheLocker sync.Locker

	quitHostCacheGrooming chan bool

	hostCacheGroomingTicker *time.Ticker

	sessionConfigs []sessionConfig

	// hostCacheEntryIdleDuration is how long a cache entry can go without an
	// update before being eligible for eviction.
	hostCacheEntryIdleDuration time.Duration
}

// AddSessionConfig implements MiddlewareHandler.
func (h *handler) AddSessionConfig(maxRequests int64, sessionDuration time.Duration, banDuration time.Duration) {
	h.sessionConfigs = append(h.sessionConfigs, sessionConfig{
		sessionDuration: sessionDuration,
		maxRequests:     maxRequests,
		banDuration:     banDuration,
	})
}

// AfterShutdown implements MiddlewareHandler.
func (h *handler) AfterShutdown() {
	h.stopHostCacheGroomingLoop(h.quitHostCacheGrooming)
}

// BeforeStart implements MiddlewareHandler.
func (h *handler) BeforeStart(wg *sync.WaitGroup) {
	h.hostCacheGroomingTicker = time.NewTicker(10 * time.Second)
	h.quitHostCacheGrooming = make(chan bool)
	wg.Add(1)
	go h.startHostCacheGroomingLoop(wg, h.quitHostCacheGrooming)
}

// SetHostCacheEntryIdleDuration implements MiddlewareHandler.
func (h *handler) SetHostCacheEntryIdleDuration(d time.Duration) {
	h.hostCacheEntryIdleDuration = d
}

func (h *handler) startHostCacheGroomingLoop(wg *sync.WaitGroup, quit <-chan bool) {
	defer logger.Debug("startHostCacheGroomingLoop", "exited")
	defer wg.Done()
	for {
		select {
		case <-quit:
			return
		case t := <-h.hostCacheGroomingTicker.C:
			h.onHostCacheGroomingTick(t)
		}
	}
}

func (h *handler) stopHostCacheGroomingLoop(quit chan<- bool) {
	h.hostCacheGroomingTicker.Stop()
	quit <- true
}

func (h *handler) onHostCacheGroomingTick(t time.Time) {
	h.hostCacheLocker.Lock()
	defer h.hostCacheLocker.Unlock()
	beforeCount := len(h.remoteHosts)
	maps.DeleteFunc(
		h.remoteHosts,
		func(host string, entry clientEntry) bool {
			idleDuration := t.Sub(entry.lastUpdatedAt)
			if idleDuration > h.hostCacheEntryIdleDuration {
				logger.Debug("onHostCacheGroomingTick", "Removing client cache entry for host %s", host)
				return true
			} else {
				willRemoveIn := h.hostCacheEntryIdleDuration - idleDuration
				logger.Debug("onHostCacheGroomingTick", "client cache entry for host %s can be removed in %s", host, willRemoveIn)
				return false
			}
		})
	afterCount := len(h.remoteHosts)
	if afterCount != beforeCount {
		logger.Debug("onHostCacheGroomingTick",
			"Removed %d entries (current length %d)",
			beforeCount-afterCount,
			afterCount,
		)
	}
}

func getApplicableHost(r *http.Request) (string, error) {
	if ip := r.Header.Get("fastly-client-ip"); ip != "" {
		return ip, nil
	}
	forwardedForIPs := r.Header.Values("x-forwarded-for")
	if len(forwardedForIPs) > 0 {
		return forwardedForIPs[len(forwardedForIPs)-1], nil
	}
	logger.Debug("getApplicableHost", "Remote address: %s", r.RemoteAddr)
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err != nil {
		return "", err
	} else if host != "" {
		return host, nil
	}
	return "", errors.New("no applicable host")
}

// ServeHTTP implements http.Handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.hostCacheLocker.Lock()
	defer h.hostCacheLocker.Unlock()
	if host, err := getApplicableHost(r); err != nil {
		logger.Debug("ServeHTTP", "Error determining applicable host: %s", err)
		h.deps.HandleStatusBadRequest(w, r, fmt.Errorf("determining host: %w", err))
	} else {
		logger.Debug("ServeHTTP", "Processing host: %s", host)
		if value, found := h.remoteHosts[host]; found {
			h.remoteHosts[host] = newUpdatedEntry(
				value,
				h.deps.Now(),
			)
		} else {
			h.remoteHosts[host] = newClientEntry(
				h.deps.Now(),
				h.sessionConfigs,
			)
		}
		if h.remoteHosts[host].isBanned() {
			logger.Debug("ServeHTTP", "Host %s is banned", host)
			h.deps.HandleStatusTooManyRequests(w, r)
		} else {
			h.next.ServeHTTP(w, r)
		}
	}
}

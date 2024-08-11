package urlpathpatternhandler

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jakewan/sudsy/internal/common"
)

var (
	ErrAmbiguousCaptureVariableNames = errors.New("ambiguous capture variable names")

	logger = common.NewLogger("urlpathpatternhandler")
)

type Handler interface {
	http.Handler
	Pattern() string
}

func NewHandler(pattern string, handler http.Handler, contextKey any) Handler {
	return &urlPatternHandler{
		contextKey:  contextKey,
		pattern:     pattern,
		httpHandler: handler,
	}
}

type urlPatternHandler struct {
	contextKey  any
	pattern     string
	httpHandler http.Handler
}

// ServeHTTP implements Handler.
func (r *urlPatternHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger.Debug("", "Inside urlPatternHandler.ServeHTTP")
	pathParts := splitParts(req.URL.Path)
	patternParts := splitParts(r.pattern)
	pathPartsLen := len(pathParts)
	if pathPartsLen != len(patternParts) {
		panic("unimplemented")
	} else {
		contextVal := make(map[string]string)
		for i := 0; i < pathPartsLen; i++ {
			patternToken := patternParts[i]
			if strings.HasPrefix(patternToken, ":") {
				contextVal[patternToken] = pathParts[i]
			}
		}
		if len(contextVal) > 0 {
			req = req.WithContext(
				context.WithValue(
					req.Context(),
					r.contextKey,
					contextVal,
				),
			)
		}
		r.httpHandler.ServeHTTP(w, req)
	}
}

// Pattern implements Responder.
func (r *urlPatternHandler) Pattern() string {
	return r.pattern
}

// ComparePatternHandlers compares two PatternResponder objects without respect to the names of any capture variable names.
func ComparePatternHandlers(l, r Handler) int {
	lparts := splitParts(l.Pattern())
	rparts := splitParts(r.Pattern())
	return compareParts(lparts, rparts)
}

func ComparePatternHandlerToPath(h Handler, requestPath string) int {
	lparts := splitParts(h.Pattern())
	rparts := splitParts(requestPath)
	return compareParts(lparts, rparts)
}

// ValidateResponders should be called on a set of handlers to ensure there
// are no ambiguous patterns found.
func ValidateResponders(handlers []Handler) error {
	staticPatterns := make([][]string, 0, len(handlers))
	for _, h := range handlers {
		staticPattern := []string{}
		for _, part := range splitParts(h.Pattern()) {
			if strings.HasPrefix(part, ":") {
				staticPattern = append(staticPattern, "")
			} else {
				staticPattern = append(staticPattern, part)
			}
		}
		if len(staticPatterns) < 1 {
			staticPatterns = append(staticPatterns, staticPattern)
		} else {
			alreadySaved := true
			staticPatternCount := len(staticPattern)
			for _, savedStaticPattern := range staticPatterns {
				if staticPatternCount != len(savedStaticPattern) {
					alreadySaved = false
					break
				}
				for i := 0; i < staticPatternCount; i++ {
					if staticPattern[i] != savedStaticPattern[i] {
						alreadySaved = false
						break
					}
				}
			}
			if alreadySaved {
				return ErrAmbiguousCaptureVariableNames
			}
			staticPatterns = append(staticPatterns, staticPattern)
		}
	}
	return nil
}

// compareParts is used internally to compare patterns.
//
// lparts should always be derived from a pattern specification (i.e. from
// calling PatternResponder.Pattern()). Any tokens with a leading ":" character
// are ignored during comparisons.
func compareParts(lparts []string, rparts []string) int {
	llen := len(lparts)
	rlen := len(rparts)
	if llen < rlen {
		return -1
	} else if llen > rlen {
		return 1
	} else {
		for i := 0; i < llen; i++ {
			if !strings.HasPrefix(lparts[i], ":") {
				switch c := cmp.Compare(lparts[i], rparts[i]); {
				case c < 0:
					return -1
				case c > 0:
					return 1
				}
			}
		}
		return 0
	}
}

func splitParts(s string) []string {
	return strings.Split(strings.TrimPrefix(s, "/"), "/")
}

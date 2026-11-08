package server

// Every request a device makes to reader sync is logged and the last ones kept
// in memory, so a push that did not show up can be told apart from one that
// never arrived: a wrong password, a wrong server address, a different file.

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

const kosyncContactsKept = 100

// kosyncContact is one request from a device.
type kosyncContact struct {
	At       time.Time `json:"at"`
	Method   string    `json:"method"`
	Path     string    `json:"path"`
	Status   int       `json:"status"`
	Username string    `json:"username"`
	Client   string    `json:"client"` // the device's User-Agent
}

type kosyncContacts struct {
	mu   sync.Mutex
	list []kosyncContact // oldest first
}

func (c *kosyncContacts) add(x kosyncContact) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list = append(c.list, x)
	if len(c.list) > kosyncContactsKept {
		c.list = c.list[len(c.list)-kosyncContactsKept:]
	}
}

// matching returns the contacts keep accepts, newest first.
func (c *kosyncContacts) matching(keep func(kosyncContact) bool) []kosyncContact {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []kosyncContact{}
	for i := len(c.list) - 1; i >= 0; i-- {
		if keep(c.list[i]) {
			out = append(out, c.list[i])
		}
	}
	return out
}

// kosyncRecord notes each request after it is answered.
func (s *Server) kosyncRecord(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		user := r.Header.Get("X-Auth-User")
		if user == "" {
			user, _, _ = r.BasicAuth()
		}
		x := kosyncContact{At: time.Now(), Method: r.Method, Path: clip(r.URL.Path, 200), Status: ww.Status(),
			Username: clip(strings.TrimSpace(user), 64), Client: clip(r.UserAgent(), 160)}
		s.kosyncSeen.add(x)
		s.log.Info("kosync request", "method", x.Method, "path", x.Path, "status", x.Status, "user", x.Username, "client", x.Client, "ip", clientIP(r))
	})
}

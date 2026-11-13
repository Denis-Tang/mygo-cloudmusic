// Package control serves a small HTTP API on 127.0.0.1, for driving the app
// without a mouse or a screen: the state of what plays, the elements of the
// view clicked by their label, text typed, and the window drawn off screen.
// It is off unless MYGO_CONTROL is set, and answers only to the token it
// writes beside the app's data.
package control

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"netease-music-mygo/internal/diag"
)

// A Driver does what the API asks, on the app's main thread.
type Driver interface {
	State() (any, error)
	Click(label string) error
	Type(text string) error
	Key(name string) error
	Seek(seconds float64) error
	Open(page string) error
	Play(index int) error
	Search(query string) error
	Screenshot(width, height int) ([]byte, error)
	Quit()
}

// Server is the control API, listening on the loopback.
type Server struct {
	srv   *http.Server
	Token string
	Port  int
	Path  string
}

// Start serves d at a port of the system's choosing, and writes where it is
// and the token to reach it as control.json in dir.
func Start(dir string, d Driver) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		ln.Close()
		return nil, err
	}
	s := &Server{Token: token, Port: ln.Addr().(*net.TCPAddr).Port}
	mux := http.NewServeMux()
	mux.HandleFunc("/state", s.auth(func(w http.ResponseWriter, r *http.Request) {
		st, err := d.State()
		reply(w, st, err)
	}))
	mux.HandleFunc("/errors", s.auth(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		reply(w, map[string]any{"errors": diag.Recent(n), "log": diag.Path()}, nil)
	}))
	mux.HandleFunc("/screenshot.png", s.auth(func(w http.ResponseWriter, r *http.Request) {
		wq, _ := strconv.Atoi(r.URL.Query().Get("w"))
		hq, _ := strconv.Atoi(r.URL.Query().Get("h"))
		b, err := d.Screenshot(wq, hq)
		if err != nil {
			reply(w, nil, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(b)
	}))
	mux.HandleFunc("/click", s.auth(func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, d.Click(r.URL.Query().Get("label")))
	}))
	mux.HandleFunc("/type", s.auth(func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, d.Type(r.URL.Query().Get("text")))
	}))
	mux.HandleFunc("/key", s.auth(func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, d.Key(r.URL.Query().Get("key")))
	}))
	mux.HandleFunc("/seek", s.auth(func(w http.ResponseWriter, r *http.Request) {
		sec, err := strconv.ParseFloat(r.URL.Query().Get("sec"), 64)
		if err != nil {
			reply(w, nil, fmt.Errorf("sec: %w", err))
			return
		}
		reply(w, nil, d.Seek(sec))
	}))
	mux.HandleFunc("/open", s.auth(func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, d.Open(r.URL.Query().Get("page")))
	}))
	mux.HandleFunc("/play", s.auth(func(w http.ResponseWriter, r *http.Request) {
		i, err := strconv.Atoi(r.URL.Query().Get("index"))
		if err != nil {
			reply(w, nil, fmt.Errorf("index: %w", err))
			return
		}
		reply(w, nil, d.Play(i))
	}))
	mux.HandleFunc("/search", s.auth(func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, d.Search(r.URL.Query().Get("q")))
	}))
	mux.HandleFunc("/quit", s.auth(func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]bool{"quitting": true}, nil)
		go func() {
			time.Sleep(100 * time.Millisecond)
			d.Quit()
		}()
	}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"ok": true, "pid": os.Getpid()}, nil)
	})
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			diag.Errorf("control: %v", err)
		}
	}()
	s.Path = filepath.Join(dir, "control.json")
	body, _ := json.MarshalIndent(map[string]any{"port": s.Port, "token": s.Token, "pid": os.Getpid()}, "", "  ")
	if err := os.WriteFile(s.Path, body, 0o600); err != nil {
		log.Printf("control: %v", err)
	}
	log.Printf("control on http://127.0.0.1:%d, token in %s", s.Port, s.Path)
	return s, nil
}

// URL is where the API listens.
func (s *Server) URL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.Port) }

// Stop closes the server.
func (s *Server) Stop() {
	if s != nil && s.srv != nil {
		s.srv.Close()
	}
}

// auth lets through the requests that carry the token, by header or query.
func (s *Server) auth(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Control-Token")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token != s.Token {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
			return
		}
		fn(w, r)
	}
}

// reply writes v as JSON, or err.
func reply(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(v)
}

// newToken makes the secret of one run.
func newToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToLower(hex.EncodeToString(b[:])), nil
}

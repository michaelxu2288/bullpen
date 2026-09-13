package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
	handlers   *Handlers
	cancelBase context.CancelFunc
}

func NewServer(addr string, h *Handlers) *Server {
	if addr == "" {
		addr = ":7070"
	}
	mux := http.NewServeMux()
	h.Register(mux)

	// Every request context descends from this one. http.Server.Shutdown waits
	// for handlers to return but never cancels them, so a long-lived SSE stream
	// would hold shutdown open until its deadline. Cancelling the base context
	// is what lets the stream handlers notice and exit.
	base, cancel := context.WithCancel(context.Background())

	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: mux,
			BaseContext: func(net.Listener) context.Context {
				return base
			},
		},
		handlers:   h,
		cancelBase: cancel,
	}
}

func (s *Server) Start() error {
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = c
	}
	// Stop accepting first, then cancel in-flight contexts so the streams
	// drain, then wait for the handlers to actually return.
	s.httpServer.SetKeepAlivesEnabled(false)
	s.cancelBase()
	return s.httpServer.Shutdown(ctx)
}

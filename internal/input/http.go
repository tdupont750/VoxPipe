package input

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const httpShutdownTimeout = 2 * time.Second

// HTTP accepts commands as POST /command/{c} on localhost.
type HTTP struct {
	port int
	log  Logger
}

func NewHTTP(port int, log Logger) *HTTP { return &HTTP{port: port, log: log} }

// Run serves until ctx is cancelled, then shuts down gracefully.
func (h *HTTP) Run(ctx context.Context, out chan<- rune) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", h.port))
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
	}
	h.log.Info(fmt.Sprintf("HTTP command server listening: curl -s -X POST http://localhost:%d/command/{cmd}", h.port))

	srv := &http.Server{Handler: Handler(ctx, out), ReadHeaderTimeout: 5 * time.Second}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		h.log.Warn("HTTP server did not shut down in time; continuing with shutdown.")
		_ = srv.Close()
	}
	return nil
}

// Handler returns the command endpoint: 200 on a valid command, 400 otherwise.
func Handler(ctx context.Context, out chan<- rune) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method != http.MethodPost || len(parts) != 2 || parts[0] != "command" || utf8.RuneCountInString(parts[1]) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		cmd, _ := utf8.DecodeRuneInString(parts[1])
		select {
		case out <- cmd:
			w.WriteHeader(http.StatusOK)
		case <-ctx.Done():
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
}

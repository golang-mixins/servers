// Package server provides an implementation of interfaces servers.
package server

import (
	"context"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"
	"io"
	Log "log"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// Config delivers a set of settings for server implementation.
type Config struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	StopTimeout       time.Duration
	MaxHeaderBytes    int
	ErrorsOutput      io.Writer
	Router            http.Handler
	KeepAliveEnabled  bool
}

// Validate validates Config according to predefined rules.
func (c Config) Validate() error {
	if c.Router == nil {
		return xerrors.New("Router can't be nil")
	}

	if c.StopTimeout == 0 {
		return xerrors.New("StopTimeout can't be empty")
	}

	addrRegExp := regexp.MustCompile(`^:[0-9]+$`)
	if ok := addrRegExp.MatchString(c.Addr); !ok {
		return xerrors.New("RegExp: Addr must be in a valid format")
	}

	if c.ErrorsOutput == nil {
		return xerrors.New("ErrorsOutput can't be nil")
	}
	return nil
}

// Server predetermines the consistency of the implementation servers.Launcher.
// Using the methods of the structure, without being initialized by the New() constructor, will lead to panic.
type Server struct {
	stopTimeout time.Duration
	mutex       *sync.RWMutex
	shutdown    bool
	http        *http.Server
}

// Serve serving the server.
func (s *Server) Serve() error {
	err := s.http.ListenAndServe()
	if err != nil {
		err = xerrors.New(err.Error())
		s.http.ErrorLog.Printf("error ListenAndServe: %s", err.Error())
	} else {
		s.http.ErrorLog.Println("unexpected exit ListenAndServe")
	}

	return err
}

// Stop stops the server.
func (s *Server) Stop(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "http server stop")
	defer span.End()

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.shutdown {
		return nil
	}

	s.http.ErrorLog.Println("starting shutdown http server")
	s.shutdown = true

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), s.stopTimeout)
	defer cancel()

	err := s.http.Shutdown(ctx)
	if err == nil {
		s.http.ErrorLog.Println("shutdown successful")
		return nil
	} else {
		s.http.ErrorLog.Printf("shutdown error: %s", err.Error())
	}

	closing := make(chan error)

	timer := time.NewTimer(s.stopTimeout)
	defer timer.Stop()

	go func() {
		err = s.http.Close()
		if err != nil {
			err = xerrors.Errorf("error closing: %w", err)
		}
		s.http.SetKeepAlivesEnabled(false)
		closing <- err
		close(closing)
	}()

	select {
	case err := <-closing:
		if err != nil {
			err = xerrors.Errorf("can't close http server: %w", err)
			s.http.ErrorLog.Printf("closing error: %s", err.Error())
		} else {
			s.http.ErrorLog.Println("closing successful")
		}
		return err
	case <-timer.C:
		err := xerrors.New("can't close http server, timeout exceeded")
		s.http.ErrorLog.Printf("closing timeout exceeded error: %s", err.Error())
		return err
	}
}

// New - constructor Server.
func New(cfg Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	server := &Server{
		stopTimeout: cfg.StopTimeout,
		mutex:       new(sync.RWMutex),
	}

	server.http = &http.Server{
		Addr:    cfg.Addr,
		Handler: cfg.Router,
	}

	server.http.ErrorLog = Log.New(cfg.ErrorsOutput, "Golang HTTP standard server: ",
		Log.LstdFlags|Log.Lmicroseconds|Log.Llongfile|Log.Lshortfile)

	if cfg.ReadTimeout != 0 {
		server.http.ReadTimeout = cfg.ReadTimeout
	}
	if cfg.ReadHeaderTimeout != 0 {
		server.http.ReadHeaderTimeout = cfg.ReadHeaderTimeout
	}
	if cfg.WriteTimeout != 0 {
		server.http.WriteTimeout = cfg.WriteTimeout
	}
	if cfg.IdleTimeout != 0 {
		server.http.IdleTimeout = cfg.IdleTimeout
	}
	if cfg.MaxHeaderBytes != 0 {
		server.http.MaxHeaderBytes = cfg.MaxHeaderBytes
	}

	server.http.SetKeepAlivesEnabled(cfg.KeepAliveEnabled)

	return server, nil
}

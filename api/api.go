package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"net/netip"
	"strconv"
	"strings"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

type Options struct {
	Listen string `yaml:"listen"`
	Secret string `yaml:"secret"`
	Debug  bool   `yaml:"debug"`
}

type APIServer struct {
	ctx    context.Context
	core   adapter.Core
	logger log.Logger

	listen string
	secret string
	debug  bool

	listener net.Listener
}

func NewAPIServer(ctx context.Context, core adapter.Core, logger log.Logger, options Options) (*APIServer, error) {
	s := &APIServer{
		ctx:    ctx,
		core:   core,
		logger: logger,
		secret: options.Secret,
		debug:  options.Debug,
	}
	listen, err := parseListen(options.Listen, 8080)
	if err != nil {
		return nil, fmt.Errorf("failed to parse listen: %w", err)
	}
	s.listen = listen
	return s, nil
}

func parseListen(listen string, defaultPort uint16) (string, error) {
	addr, err := netip.ParseAddrPort(listen)
	if err == nil {
		return addr.String(), nil
	}
	_listen := listen
	_listen = strings.Trim(_listen, "[]")
	ip, err := netip.ParseAddr(_listen)
	if err == nil {
		return netip.AddrPortFrom(ip, defaultPort).String(), nil
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("invalid listen: %s, error: %s", listen, err)
	}
	if host == "" {
		host = "::"
	}
	ip, err = netip.ParseAddr(host)
	if err != nil {
		return "", fmt.Errorf("invalid listen: %s, error: %s", listen, err)
	}
	portUint16, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return "", fmt.Errorf("invalid listen: %s, error: %s", listen, err)
	}
	if portUint16 == 0 {
		return "", fmt.Errorf("invalid listen: %s, error: invalid port", listen)
	}
	return net.JoinHostPort(ip.String(), strconv.FormatUint(portUint16, 10)), nil
}

func (s *APIServer) debugHTTPHandler() http.Handler {
	router := chi.NewRouter()
	router.HandleFunc("/pprof", pprof.Index)
	router.HandleFunc("/pprof/*", pprof.Index)
	router.HandleFunc("/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/pprof/profile", pprof.Profile)
	router.HandleFunc("/pprof/symbol", pprof.Symbol)
	router.HandleFunc("/pprof/trace", pprof.Trace)
	return router
}

func (s *APIServer) logMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			apiLogContext := adapter.NewAPILogContext()
			ctx = adapter.SaveLogContext(ctx, apiLogContext)
			s.logger.DebugfContext(ctx, "request: %s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (s *APIServer) panicMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				err := recover()
				if err != nil {
					s.logger.ErrorfContext(r.Context(), "panic: %v", err)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func (s *APIServer) authHTTPHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			bearer, token, found := strings.Cut(header, " ")

			hasInvalidHeader := bearer != "Bearer"
			hasInvalidSecret := !found || token != s.secret
			if hasInvalidHeader || hasInvalidSecret {
				w.WriteHeader(http.StatusUnauthorized)
				s.logger.ErrorfContext(r.Context(), "unauthorized: %s", r.RemoteAddr)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *APIServer) initHTTPRouter() http.Handler {
	router := chi.NewRouter()
	router.Use(s.logMiddleware(), s.panicMiddleware())
	cors := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         300,
	})
	router.Use(cors.Handler)
	if s.debug {
		router.Mount("/debug", s.debugHTTPHandler())
	}
	router.Group(func(r chi.Router) {
		if s.secret != "" {
			r.Use(s.authHTTPHandler())
		}
		upstreamRouter := chi.NewRouter()
		upstreams := s.core.GetUpstreams()
		for _, u := range upstreams {
			upstreamRouter.Get("/"+u.Tag(), func(u adapter.Upstream) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					data := map[string]any{
						"tag":  u.Tag(),
						"type": u.Type(),
						"data": u.StatisticalData(),
					}
					raw, err := json.Marshal(data)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						w.WriteHeader(http.StatusOK)
						w.Header().Set("Content-Type", "application/json")
						w.Write(raw)
					}
				}
			}(u))
		}
		upstreamRouter.Get("/", func(w http.ResponseWriter, r *http.Request) {
			data := make([]map[string]any, 0, len(upstreams))
			for _, u := range upstreams {
				data = append(data, map[string]any{
					"tag":  u.Tag(),
					"type": u.Type(),
					"data": u.StatisticalData(),
				})
			}
			raw, err := json.Marshal(map[string]any{"data": data})
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Content-Type", "application/json")
				w.Write(raw)
			}
		})
		r.Mount("/upstream", upstreamRouter)
		pluginMatcherRouter := chi.NewRouter()
		pluginMatchers := s.core.GetPluginMatchers()
		pluginMatcherHandlerExist := make(map[string]bool, len(pluginMatchers))
		for _, pm := range pluginMatchers {
			apiHandler, isAPIHandler := pm.(adapter.APIHandler)
			if isAPIHandler && apiHandler != nil {
				httpHandler := apiHandler.APIHandler()
				if httpHandler != nil {
					pluginMatcherHandlerExist[pm.Tag()] = true
					pluginMatcherRouter.Mount("/"+pm.Tag(), httpHandler)
				}
			}
		}
		pluginMatcherRouter.Get("/", func(w http.ResponseWriter, r *http.Request) {
			data := make([]map[string]any, 0, len(pluginMatchers))
			for _, pm := range pluginMatchers {
				if pluginMatcherHandlerExist[pm.Tag()] {
					data = append(data, map[string]any{
						"tag":  pm.Tag(),
						"type": pm.Type(),
					})
				}
			}
			raw, err := json.Marshal(map[string]any{"data": data})
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Content-Type", "application/json")
				w.Write(raw)
			}
		})
		r.Mount("/plugin/matcher", pluginMatcherRouter)
		pluginExecutorRouter := chi.NewRouter()
		pluginExecutors := s.core.GetPluginExecutors()
		pluginExecutorHandlerExist := make(map[string]bool, len(pluginExecutors))
		for _, pe := range pluginExecutors {
			apiHandler, isAPIHandler := pe.(adapter.APIHandler)
			if isAPIHandler && apiHandler != nil {
				httpHandler := apiHandler.APIHandler()
				if httpHandler != nil {
					pluginExecutorHandlerExist[pe.Tag()] = true
					pluginExecutorRouter.Mount("/"+pe.Tag(), httpHandler)
				}
			}
		}
		pluginExecutorRouter.Get("/", func(w http.ResponseWriter, r *http.Request) {
			data := make([]map[string]any, 0, len(pluginExecutors))
			for _, pm := range pluginExecutors {
				if pluginExecutorHandlerExist[pm.Tag()] {
					data = append(data, map[string]any{
						"tag":  pm.Tag(),
						"type": pm.Type(),
					})
				}
			}
			raw, err := json.Marshal(map[string]any{"data": data})
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Content-Type", "application/json")
				w.Write(raw)
			}
		})
		r.Mount("/plugin/executor", pluginExecutorRouter)
	})
	return router
}

func (s *APIServer) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", s.listen)
	if err != nil {
		err = fmt.Errorf("failed to listen: %w", err)
		return err
	}
	httpServer := &http.Server{
		Handler: s.initHTTPRouter(),
	}
	go httpServer.Serve(s.listener)
	s.logger.Infof("api server started: %s", s.listen)
	return nil
}

func (s *APIServer) Close() error {
	s.listener.Close()
	return nil
}

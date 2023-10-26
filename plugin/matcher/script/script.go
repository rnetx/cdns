package script

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"

	"github.com/go-chi/chi/v5"
)

const Type = "script"

func init() {
	plugin.RegisterPluginMatcher(Type, NewScript)
}

const DefaultInterval = 5 * time.Minute

type Args struct {
	Command  string                 `json:"command"`
	Args     utils.Listable[string] `json:"args"`
	Interval utils.Duration         `json:"interval"`
}

var (
	_ adapter.PluginMatcher = (*Script)(nil)
	_ adapter.Starter       = (*Script)(nil)
	_ adapter.Closer        = (*Script)(nil)
)

type Script struct {
	ctx    context.Context
	tag    string
	logger log.Logger

	command  string
	args     []string
	interval time.Duration

	result  bool
	runLock sync.Mutex

	commandCtx    context.Context
	commandCancel context.CancelFunc
	loopCtx       context.Context
	loopCancel    context.CancelFunc
	closeDone     chan struct{}
}

func NewScript(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginMatcher, error) {
	s := &Script{
		ctx:    ctx,
		tag:    tag,
		logger: logger,
	}
	var a Args
	err := utils.JsonDecode(args, &a)
	if err != nil {
		return nil, fmt.Errorf("parse args failed: %w", err)
	}
	if a.Command == "" {
		return nil, fmt.Errorf("missing command")
	}
	s.command = a.Command
	s.args = a.Args
	if a.Interval > 0 {
		s.interval = time.Duration(a.Interval)
	} else {
		s.interval = DefaultInterval
	}
	return s, nil
}

func (s *Script) Tag() string {
	return s.tag
}

func (s *Script) Type() string {
	return Type
}

func (s *Script) Start() error {
	s.commandCtx, s.commandCancel = context.WithCancel(s.ctx)
	s.runScript(s.commandCtx)
	s.loopCtx, s.loopCancel = context.WithCancel(s.ctx)
	s.closeDone = make(chan struct{}, 1)
	go s.loopHandle()
	return nil
}

func (s *Script) Close() error {
	s.commandCancel()
	s.loopCancel()
	<-s.closeDone
	close(s.closeDone)
	return nil
}

func (s *Script) runScript(ctx context.Context) {
	s.logger.Debug("run script...")
	defer s.logger.Debug("run script done")
	var cmd *exec.Cmd
	if len(s.args) > 0 {
		cmd = exec.CommandContext(ctx, s.command, s.args...)
	} else {
		cmd = exec.CommandContext(ctx, s.command)
	}
	buffer := bytes.NewBuffer(nil)
	cmd.Stdout = buffer
	err := cmd.Run()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.logger.Errorf("run script failed: %s", err)
		}
		return
	}
	op := buffer.String()
	op = strings.TrimSpace(op)
	s.result, _ = strconv.ParseBool(op)
}

func (s *Script) loopHandle() {
	defer func() {
		select {
		case s.closeDone <- struct{}{}:
		default:
		}
	}()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.loopCtx.Done():
			return
		case <-ticker.C:
			s.runLock.Lock()
			s.runScript(s.commandCtx)
			s.runLock.Unlock()
		}
	}
}

func (s *Script) LoadRunningArgs(_ context.Context, _ any) (uint16, error) {
	return 0, nil
}

func (s *Script) Match(ctx context.Context, dnsCtx *adapter.DNSContext, _ uint16) (bool, error) {
	result := s.result
	s.logger.DebugfContext(ctx, "script match result: %t", result)
	return result, nil
}

func (s *Script) runScriptHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.runLock.TryLock() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer s.runLock.Unlock()
		s.runScript(s.commandCtx)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Script) APIHandler() chi.Router {
	builder := utils.NewChiRouterBuilder()
	builder.Add(&utils.ChiRouterBuilderItem{
		Path:        "/run",
		Methods:     []string{http.MethodGet},
		Description: "run script",
		Handler:     s.runScriptHandler(),
	})
	return builder.Build()
}

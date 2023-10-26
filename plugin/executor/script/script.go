package script

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

const Type = "script"

func init() {
	plugin.RegisterPluginExecutor(Type, NewScript)
}

type Args struct {
	Command string                 `json:"command"`
	Args    utils.Listable[string] `json:"args"`
}

var (
	_ adapter.PluginExecutor = (*Script)(nil)
	_ adapter.Starter        = (*Script)(nil)
	_ adapter.Closer         = (*Script)(nil)
)

type Script struct {
	ctx    context.Context
	tag    string
	logger log.Logger

	command string
	args    []string

	commandCtx    context.Context
	commandCancel context.CancelFunc
}

func NewScript(ctx context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginExecutor, error) {
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
	return nil
}

func (s *Script) Close() error {
	s.commandCancel()
	return nil
}

func (s *Script) buildArgs(dnsCtx *adapter.DNSContext) map[string]string {
	m := make(map[string]string)
	m["CDNS_ID"] = strconv.Itoa(int(dnsCtx.ID()))
	m["CDNS_INIT_TIME"] = strconv.Itoa(int(dnsCtx.InitTime().UnixNano()))
	m["CDNS_LISTENER"] = dnsCtx.Listener()
	m["CDNS_CLIENT_IP"] = dnsCtx.ClientIP().String()
	reqMsg := dnsCtx.ReqMsg()
	if reqMsg != nil {
		if len(reqMsg.Question) > 0 {
			q := reqMsg.Question[0]
			m["CDNS_REQ_QNAME"] = q.Name
			m["CDNS_REQ_QTYPE"] = dns.TypeToString[q.Qtype]
			m["CDNS_REQ_QCLASS"] = dns.ClassToString[q.Qclass]
		}
	}
	respMsg := dnsCtx.RespMsg()
	if respMsg != nil {
		ips := make([]netip.Addr, 0, len(respMsg.Answer))
		for _, ans := range respMsg.Answer {
			switch rr := ans.(type) {
			case *dns.A:
				ip, ok := netip.AddrFromSlice(rr.A)
				if ok {
					ips = append(ips, ip)
				}
			case *dns.AAAA:
				ip, ok := netip.AddrFromSlice(rr.AAAA)
				if ok {
					ips = append(ips, ip)
				}
			}
		}
		if len(ips) > 0 {
			m["CDNS_RESP_IP_LEN"] = strconv.Itoa(len(ips))
			for i, ip := range ips {
				m[fmt.Sprintf("CDNS_RESP_IP_%d", i+1)] = ip.String()
			}
		}
		respUpstreamTag := dnsCtx.RespUpstreamTag()
		if respUpstreamTag != "" {
			m["CDNS_RESP_UPSTREAM_TAG"] = respUpstreamTag
		}
	}
	mark := dnsCtx.Mark()
	if mark != 0 {
		m["CDNS_MARK"] = strconv.Itoa(int(mark))
	}
	metadata := dnsCtx.Metadata()
	if len(metadata) > 0 {
		for k, v := range metadata {
			m[fmt.Sprintf("CDNS_METADATA_%s", strings.ToUpper(k))] = v
		}
	}
	return m
}

func (s *Script) runScript(m map[string]string) {
	args := s.args
	if len(args) > 0 {
		for i := range args {
			arg := args[i]
			for k, v := range m {
				if strings.Contains(arg, "{"+k+"}") {
					arg = strings.ReplaceAll(arg, "{"+k+"}", v)
				}
			}
			args[i] = arg
		}
	}
	var cmd *exec.Cmd
	if len(args) > 0 {
		cmd = exec.CommandContext(s.commandCtx, s.command, args...)
	} else {
		cmd = exec.CommandContext(s.commandCtx, s.command)
	}
	err := cmd.Run()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.logger.Errorf("run script failed: %s, error: %s", cmd.String(), err)
		}
	}
}

func (s *Script) LoadRunningArgs(_ context.Context, _ any) (uint16, error) {
	return 0, nil
}

func (s *Script) Exec(ctx context.Context, dnsCtx *adapter.DNSContext, _ uint16) (adapter.ReturnMode, error) {
	m := s.buildArgs(dnsCtx)
	go s.runScript(m)
	s.logger.DebugContext(ctx, "run script")
	return adapter.ReturnModeContinue, nil
}

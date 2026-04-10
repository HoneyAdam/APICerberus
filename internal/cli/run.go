package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/mcp"
	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
	"github.com/APICerberus/APICerebrus/internal/portal"
	"github.com/APICerberus/APICerebrus/internal/raft"
	"github.com/APICerberus/APICerebrus/internal/store"
	"github.com/APICerberus/APICerebrus/internal/version"
)

const defaultPIDFile = "apicerberus.pid"

// Run dispatches CLI commands.
func Run(args []string) error {
	if len(args) == 0 {
		return runStart(nil)
	}

	switch args[0] {
	case "start":
		return runStart(args[1:])
	case "stop":
		return runStop(args[1:])
	case "version":
		return runVersion()
	case "config":
		return runConfig(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "user":
		return runUser(args[1:])
	case "credit":
		return runCredit(args[1:])
	case "audit":
		return runAudit(args[1:])
	case "analytics":
		return runAnalytics(args[1:])
	case "service":
		return runService(args[1:])
	case "route":
		return runRoute(args[1:])
	case "upstream":
		return runUpstream(args[1:])
	case "db":
		return runDB(args[1:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	cfgPath := fs.String("config", "apicerberus.yaml", "path to gateway config file")
	pidFile := fs.String("pid-file", defaultPIDFile, "path to PID file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	netutil.SetTrustedProxies(cfg.Gateway.TrustedProxies)
	gw, err := gateway.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize gateway: %w", err)
	}

	adminSrv, err := admin.NewServer(cfg, gw)
	if err != nil {
		return fmt.Errorf("initialize admin server: %w", err)
	}

	adminHTTP := &http.Server{
		Addr:           cfg.Admin.Addr,
		Handler:        adminSrv,
		ReadTimeout:    cfg.Gateway.ReadTimeout,
		WriteTimeout:   cfg.Gateway.WriteTimeout,
		IdleTimeout:    cfg.Gateway.IdleTimeout,
		MaxHeaderBytes: cfg.Gateway.MaxHeaderBytes,
	}

	// Raft cluster initialization
	var (
		raftNode   *raft.Node
		clusterMgr *raft.ClusterManager
	)
	if cfg.Cluster.Enabled {
		raftCfg := &raft.Config{
			NodeID:             cfg.Cluster.NodeID,
			BindAddress:        cfg.Cluster.BindAddress,
			ElectionTimeoutMin: cfg.Cluster.ElectionTimeoutMin,
			ElectionTimeoutMax: cfg.Cluster.ElectionTimeoutMax,
			HeartbeatInterval:  cfg.Cluster.HeartbeatInterval,
		}

		gatewayFSM := raft.NewGatewayFSM()
		transport := raft.NewHTTPTransport(cfg.Cluster.BindAddress, cfg.Cluster.NodeID)

		// Set RPC secret for inter-node authentication
		if cfg.Cluster.RPCSecret != "" {
			transport.SetRPCSecret(cfg.Cluster.RPCSecret)
		}

		var raftErr error
		raftNode, raftErr = raft.NewNode(raftCfg, gatewayFSM, transport)
		if raftErr != nil {
			return fmt.Errorf("initialize raft node: %w", raftErr)
		}

		for _, peer := range cfg.Cluster.Peers {
			raftNode.AddPeer(peer.ID, peer.Address)
			transport.SetPeer(peer.ID, peer.Address)
		}

		if raftErr = raftNode.Start(); raftErr != nil {
			return fmt.Errorf("start raft node: %w", raftErr)
		}

		clusterMgr = raft.NewClusterManager(raftNode, gatewayFSM, cfg.Admin.Addr, cfg.Admin.APIKey)
		if raftErr = clusterMgr.Start(); raftErr != nil {
			_ = raftNode.Stop()
			return fmt.Errorf("start cluster manager: %w", raftErr)
		}
	}

	var (
		portalHTTP  *http.Server
		portalStore *store.Store
	)
	if cfg.Portal.Enabled {
		portalStore, err = store.Open(cfg)
		if err != nil {
			return fmt.Errorf("open portal store: %w", err)
		}
		portalSrv, err := portal.NewServer(cfg, portalStore)
		if err != nil {
			_ = portalStore.Close()
			return fmt.Errorf("initialize portal server: %w", err)
		}
		portalHTTP = &http.Server{
			Addr:           cfg.Portal.Addr,
			Handler:        portalSrv,
			ReadTimeout:    cfg.Gateway.ReadTimeout,
			WriteTimeout:   cfg.Gateway.WriteTimeout,
			IdleTimeout:    cfg.Gateway.IdleTimeout,
			MaxHeaderBytes: cfg.Gateway.MaxHeaderBytes,
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := writePID(*pidFile); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(*pidFile)
	defer func() {
		if portalStore != nil {
			_ = portalStore.Close()
		}
	}()
	defer func() {
		if clusterMgr != nil {
			_ = clusterMgr.Stop()
		}
		if raftNode != nil {
			_ = raftNode.Stop()
		}
	}()

	printBanner(cfg, *pidFile)

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		_ = gw.Shutdown(shutdownCtx)
		_ = adminHTTP.Shutdown(shutdownCtx)
		if portalHTTP != nil {
			_ = portalHTTP.Shutdown(shutdownCtx)
		}
	}()

	serverCount := 2
	if portalHTTP != nil {
		serverCount++
	}
	errCh := make(chan error, serverCount)
	go func() { errCh <- gw.Start(ctx) }()
	go func() {
		err := adminHTTP.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()
	if portalHTTP != nil {
		go func() {
			err := portalHTTP.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			errCh <- err
		}()
	}

	var firstErr error
	for i := 0; i < serverCount; i++ {
		err := <-errCh
		if err != nil && firstErr == nil {
			firstErr = err
			cancel()
		}
	}

	if firstErr != nil {
		return fmt.Errorf("runtime failure: %w", firstErr)
	}
	return nil
}

func runStop(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	pidFile := fs.String("pid-file", defaultPIDFile, "path to PID file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	data, err := os.ReadFile(*pidFile)
	if err != nil {
		return fmt.Errorf("read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid pid value: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		_ = process.Kill()
	}
	_ = os.Remove(*pidFile)

	fmt.Printf("Sent termination signal to process %d\n", pid)
	return nil
}

func runVersion() error {
	payload := map[string]string{
		"version":    version.Version,
		"commit":     version.Commit,
		"build_time": version.BuildTime,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func runConfig(args []string) error {
	if len(args) == 0 {
		return errors.New("missing config subcommand (expected: validate|export|import|diff)")
	}
	switch args[0] {
	case "validate":
		return runConfigValidate(args[1:])
	case "export":
		return runConfigExport(args[1:])
	case "import":
		return runConfigImport(args[1:])
	case "diff":
		return runConfigDiff(args[1:])
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func runConfigValidate(args []string) error {
	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("config validate requires a path")
	}
	path := fs.Arg(0)

	if _, err := config.Load(path); err != nil {
		return fmt.Errorf("config invalid: %w", err)
	}
	fmt.Printf("Config is valid: %s\n", path)
	return nil
}

func runMCP(args []string) error {
	if len(args) > 0 && strings.EqualFold(strings.TrimSpace(args[0]), "start") {
		args = args[1:]
	}

	fs := flag.NewFlagSet("mcp start", flag.ContinueOnError)
	cfgPath := fs.String("config", "apicerberus.yaml", "path to gateway config file")
	transport := fs.String("transport", "stdio", "MCP transport: stdio or sse")
	addr := fs.String("addr", ":3000", "MCP SSE listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	server, err := mcp.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("initialize mcp server: %w", err)
	}
	defer server.Close()

	var raftNode *raft.Node
	if cfg.Cluster.Enabled {
		raftCfg := &raft.Config{
			NodeID:             cfg.Cluster.NodeID,
			BindAddress:        cfg.Cluster.BindAddress,
			ElectionTimeoutMin: cfg.Cluster.ElectionTimeoutMin,
			ElectionTimeoutMax: cfg.Cluster.ElectionTimeoutMax,
			HeartbeatInterval:  cfg.Cluster.HeartbeatInterval,
		}
		gatewayFSM := raft.NewGatewayFSM()
		t := raft.NewHTTPTransport(cfg.Cluster.BindAddress, cfg.Cluster.NodeID)
		if cfg.Cluster.RPCSecret != "" {
			t.SetRPCSecret(cfg.Cluster.RPCSecret)
		}
		raftNode, err = raft.NewNode(raftCfg, gatewayFSM, t)
		if err != nil {
			return fmt.Errorf("initialize raft node: %w", err)
		}
		for _, peer := range cfg.Cluster.Peers {
			raftNode.AddPeer(peer.ID, peer.Address)
			t.SetPeer(peer.ID, peer.Address)
		}
		if err = raftNode.Start(); err != nil {
			return fmt.Errorf("start raft node: %w", err)
		}
		server.SetRaftNode(raftNode)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(*transport)) {
	case "stdio", "":
		return server.RunStdio(ctx)
	case "sse":
		bind := strings.TrimSpace(*addr)
		if bind == "" {
			bind = ":3000"
		}
		return server.RunSSE(ctx, bind)
	default:
		return fmt.Errorf("unsupported mcp transport %q (expected: stdio|sse)", *transport)
	}
}

func printUsage() {
	fmt.Println("API Cerberus CLI")
	fmt.Println("Usage:")
	fmt.Println("  apicerberus start [--config path] [--pid-file path]")
	fmt.Println("  apicerberus stop [--pid-file path]")
	fmt.Println("  apicerberus version")
	fmt.Println("  apicerberus config validate|export|import|diff ...")
	fmt.Println("  apicerberus mcp start [--config path] [--transport stdio|sse] [--addr :3000]")
	fmt.Println("  apicerberus user list|create|get|update|suspend|activate|apikey|permission|ip ...")
	fmt.Println("  apicerberus credit overview|balance|topup|deduct|transactions ...")
	fmt.Println("  apicerberus audit search|tail|detail|export|stats|cleanup|retention ...")
	fmt.Println("  apicerberus analytics overview|requests|latency ...")
	fmt.Println("  apicerberus service list|add|get|update|delete ...")
	fmt.Println("  apicerberus route list|add|get|update|delete ...")
	fmt.Println("  apicerberus upstream list|add|get|update|delete ...")
	fmt.Println("  apicerberus db migrate status|apply ...")
}

func printBanner(cfg *config.Config, pidFile string) {
	fmt.Println("========================================")
	fmt.Println("      API CERBERUS — CORE GATEWAY      ")
	fmt.Println("========================================")
	fmt.Printf("Version : %s\n", version.Version)
	fmt.Printf("Commit  : %s\n", version.Commit)
	httpAddr := strings.TrimSpace(cfg.Gateway.HTTPAddr)
	httpsAddr := strings.TrimSpace(cfg.Gateway.HTTPSAddr)
	switch {
	case httpAddr != "" && httpsAddr != "":
		fmt.Printf("Gateway : %s (http), %s (https)\n", httpAddr, httpsAddr)
	case httpsAddr != "":
		fmt.Printf("Gateway : %s (https)\n", httpsAddr)
	default:
		fmt.Printf("Gateway : %s\n", httpAddr)
	}
	fmt.Printf("Admin   : %s\n", cfg.Admin.Addr)
	if cfg.Portal.Enabled {
		fmt.Printf("Portal  : %s\n", cfg.Portal.Addr)
	}
	fmt.Printf("PID File: %s\n", filepath.Clean(pidFile))
	fmt.Println("========================================")
}

func writePID(path string) error {
	pid := os.Getpid()
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

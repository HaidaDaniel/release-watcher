package cli

import (
	"context"
	"fmt"

	"github.com/HaidaDaniel/release-watcher/internal/mcp"
)

func (r *runner) mcp(ctx context.Context, args []string) int {
	if len(args) != 1 || args[0] != "serve" {
		return r.fail(&commandError{code: "usage", message: "usage: release-watcher mcp serve", exit: ExitUsage})
	}
	svc, cfg, logger, closeStore, openErr := r.openServiceConfig(ctx)
	if openErr != nil {
		return r.fail(openErr)
	}
	defer closeStore()
	if cfg.MCPToken == "" {
		return r.fail(&commandError{code: "config_error", message: "RELEASE_WATCHER_MCP_TOKEN is required", exit: ExitError})
	}
	if len(cfg.MCPToken) < 32 {
		return r.fail(&commandError{code: "config_error", message: "RELEASE_WATCHER_MCP_TOKEN must contain at least 32 bytes", exit: ExitError})
	}
	server, err := mcp.New(svc, mcp.Config{
		Token:          cfg.MCPToken,
		WriteEnabled:   cfg.MCPWriteEnabled,
		AllowedHosts:   cfg.MCPAllowedHosts,
		AllowedOrigins: cfg.MCPAllowedOrigins,
	}, logger)
	if err != nil {
		return r.fail(&commandError{code: "config_error", message: err.Error(), exit: ExitError})
	}
	logger.Info("mcp listening", "addr", cfg.MCPListen, "write_enabled", cfg.MCPWriteEnabled)
	if err := server.Serve(ctx, cfg.MCPListen); err != nil {
		return r.fail(&commandError{code: "runtime_error", message: err.Error(), exit: ExitError})
	}
	if !r.json {
		fmt.Fprintln(r.stderr, "release-watcher: mcp server stopped")
	}
	return ExitOK
}

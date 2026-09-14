package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
	"github.com/myrrazor/atlas-tasker/internal/service"
	webui "github.com/myrrazor/atlas-tasker/internal/web"
	"github.com/spf13/cobra"
)

var (
	setupLookPath      func(string) (string, error)
	setupClientRunner  adapter.CommandRunner
	appOptionsOverride func(*app.Options)
)

func openApp() (*app.App, error) {
	return openAppWithState("")
}

func openAppWithState(stateDir string) (*app.App, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	opts := app.Options{
		Home:           home,
		StateDir:       strings.TrimSpace(stateDir),
		LookPath:       setupLookPath,
		CommandRunner:  setupClientRunner,
		WriteClientCfg: true,
		OpenBrowser:    openURLFunc,
	}
	if appOptionsOverride != nil {
		appOptionsOverride(&opts)
	}
	return app.Open(opts)
}

func runRootHome(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown command %q", args[0])
	}
	a, err := openApp()
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()
	if a.Settings().AutoRegister {
		if cwd, err := os.Getwd(); err == nil {
			if root, err := service.InitializedWorkspaceRoot(cwd); err == nil {
				_, _ = a.Register(commandContext(cmd), app.RegisterOptions{Root: root, DisplayName: filepath.Base(root)})
			}
		}
	}
	noOpen, _ := cmd.Flags().GetBool("no-open")
	jsonMode, _ := cmd.Flags().GetBool("json")
	status, err := a.EnsureService(commandContext(cmd), app.ServiceOptions{
		OpenBrowser: !noOpen && !jsonMode && canPromptIntegrations(cmd),
	})
	if err != nil {
		return err
	}
	printStatus := status
	printStatus.ClaimURL = ""
	pretty := status.URL
	md := fmt.Sprintf("# Atlas Home\n\n- URL: %s\n- Port: %d\n", status.URL, status.Port)
	return writeCommandOutput(cmd, printStatus, md, pretty)
}

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the user-level Atlas Home loopback service",
		RunE:  runHomeServe,
	}
	cmd.Flags().String("host", app.DefaultHomeBind, "Loopback host")
	cmd.Flags().Int("port", 0, "Port; 0 uses the configured machine port")
	cmd.Flags().String("state-dir", "", "Absolute Atlas state directory; overrides XDG/home resolution")
	cmd.Flags().Bool("no-browser", false, "Do not open a browser")
	addReadOutputFlags(cmd, &outputFlags{})
	return cmd
}

func runHomeServe(cmd *cobra.Command, _ []string) error {
	stateDir, _ := cmd.Flags().GetString("state-dir")
	a, err := openAppWithState(stateDir)
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	if port == 0 {
		port = a.Settings().Service.Port
	}
	if strings.TrimSpace(host) == "" {
		host = a.Settings().Service.Bind
	}
	server, err := webui.NewHomeServer(a, webui.HomeConfig{
		Host:     host,
		Port:     port,
		Actor:    "human:owner",
		ReadOnly: false,
	})
	if err != nil {
		return err
	}
	ln, boundHost, err := webui.ListenLoopback(host, port)
	if err != nil {
		return err
	}
	_ = boundHost
	fmt.Fprintf(cmd.OutOrStdout(), "Atlas Home http://%s/\n", ln.Addr().String())
	return server.Serve(commandContext(cmd), ln)
}

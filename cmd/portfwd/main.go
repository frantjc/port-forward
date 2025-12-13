package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/frantjc/port-forward/internal/controller"
	"github.com/frantjc/port-forward/internal/logutil"
	"github.com/frantjc/port-forward/internal/portfwd/portfwdupnp"
	"github.com/frantjc/port-forward/internal/srcipmasq/srcipmasqiptables"
	"github.com/frantjc/port-forward/internal/svcip"
	"github.com/frantjc/port-forward/internal/svcip/svcipdef"
	"github.com/frantjc/port-forward/internal/svcip/svcipraw"
	"github.com/frantjc/port-forward/internal/upnp"
	xerrors "github.com/frantjc/x/errors"
	xos "github.com/frantjc/x/os"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	err := xerrors.Ignore(NewEntrypoint().ExecuteContext(ctx), context.Canceled)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}

	stop()
	xos.ExitFromError(err)
}

// NewEntrypoint returns the command which acts as
// the entrypoint for `portfwd`.
func NewEntrypoint() *cobra.Command {
	var (
		metricsAddr          string
		probeAddr            string
		webhookAddr          string
		enableLeaderElection bool
		slogConfig           = new(logutil.SlogConfig)
		overrideIPAddressS   string
		cmd                  = &cobra.Command{
			Use:           "portfwd",
			Version:       SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {

				var (
					slogHandler = slog.NewTextHandler(cmd.OutOrStdout(), &slog.HandlerOptions{
						Level: slogConfig,
					})
					log  = slog.New(slogHandler)
					logr = logr.FromSlogHandler(slogHandler)
					ctx  = logutil.SloggerInto(cmd.Context(), log)
				)
				ctrl.SetLogger(logr)

				cfg, err := ctrl.GetConfig()
				if err != nil {
					return err
				}

				webhookHost, rawWebhookPort, err := net.SplitHostPort(webhookAddr)
				if err != nil {
					return err
				}

				webhookPort, err := strconv.Atoi(rawWebhookPort)
				if err != nil {
					return err
				}

				var (
					tlsOpts = []func(*tls.Config){
						func(c *tls.Config) {
							c.NextProtos = []string{"http/1.1"}
						},
					}
					webhookServer = webhook.NewServer(webhook.Options{
						Host:    webhookHost,
						Port:    webhookPort,
						TLSOpts: tlsOpts,
					})
					metricsServerOptions = server.Options{
						BindAddress: metricsAddr,
						TLSOpts:     tlsOpts,
					}
				)

				scheme := runtime.NewScheme()

				if err := corev1.AddToScheme(scheme); err != nil {
					return err
				}

				mgr, err := ctrl.NewManager(cfg, ctrl.Options{
					BaseContext:                   cmd.Context,
					Scheme:                        scheme,
					Metrics:                       metricsServerOptions,
					WebhookServer:                 webhookServer,
					HealthProbeBindAddress:        probeAddr,
					LeaderElection:                enableLeaderElection,
					LeaderElectionID:              "e7a0a735.pf.frantj.cc",
					LeaderElectionReleaseOnCancel: true,
				})
				if err != nil {
					return err
				}

				if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
					return err
				}

				if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
					return err
				}

				upnpClient, err := upnp.NewClient(ctx, upnp.WithAnyConnection)
				if err != nil {
					return err
				}

				family := iptables.ProtocolIPv4
				if upnpClient.GetSourceIPAddress(ctx).To4() == nil {
					family = iptables.ProtocolIPv6
				}

				ipt, err := iptables.New(iptables.IPFamily(family))
				if err != nil {
					return err
				}

				var svcIPAddrGtr svcip.ServiceIPAddressGetter = new(svcipdef.ServiceIPAddressGetter)
				if overrideIPAddressS != "" {
					if overrideIPAddress := net.ParseIP(overrideIPAddressS); overrideIPAddress == nil {
						return fmt.Errorf("parse override IP address: %s", overrideIPAddressS)
					} else {
						svcIPAddrGtr = svcipraw.ServiceIPAddressGetter{overrideIPAddress}
					}
				}

				if err := (&controller.ServiceReconciler{
					ServiceIPAddressGetter: svcIPAddrGtr,
					PortForwarder: &portfwdupnp.PortForwarder{
						Client: upnpClient,
						SourceIPAddressMasqer: &srcipmasqiptables.SourceIPAddressMasqer{
							IPTables: ipt,
						},
					},
				}).SetupWithManager(mgr); err != nil {
					return err
				}

				return mgr.Start(ctx)
			},
		}
	)

	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())
	cmd.Flags().Bool("version", false, "Version for "+cmd.Name())
	cmd.SetVersionTemplate("{{ .Name }}{{ .Version }}")

	slogConfig.AddFlags(cmd.Flags())

	// Allow the --kubeconfig flag, which is consumed by sigs.k8s.io/controller-runtime when we call ctrl.GetConfig().
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	cmd.Flags().StringVar(&metricsAddr, "metrics-addr", "127.0.0.1:8081", "Metrics server bind address")
	cmd.Flags().StringVar(&probeAddr, "probe-addr", "127.0.0.1:8082", "Probe server bind address")
	cmd.Flags().StringVar(&webhookAddr, "webhook-addr", ":9443", "Webhook server bind address")
	cmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager")

	cmd.Flags().StringVar(&overrideIPAddressS, "override-ip-address", "",
		"IP address to use instead of getting it from a Service")

	return cmd
}

/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	xos "github.com/frantjc/x/os"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"golang.org/x/exp/constraints"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/frantjc/port-forward/internal/portfwd/portfwdupnp"
	"github.com/frantjc/port-forward/internal/srcipmasq/srcipmasqiptables"
	"github.com/frantjc/port-forward/internal/svcip"
	"github.com/frantjc/port-forward/internal/svcip/svcipdef"
	"github.com/frantjc/port-forward/internal/svcip/svcipraw"
	"github.com/frantjc/port-forward/internal/upnp"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/frantjc/port-forward/controllers"
	//+kubebuilder:scaffold:imports
)

func main() {
	var (
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err       error
	)

	if err = NewEntrypoint().ExecuteContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		os.Stderr.WriteString(err.Error() + "\n")
		stop()
		xos.ExitFromError(err)
	}

	stop()
}

var (
	scheme = k8sruntime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// NewEntrypoint returns the command which acts as
// the entrypoint for `manager`.
func NewEntrypoint() *cobra.Command {
	var (
		verbosity                                       int
		healthPort, metricsPort, pprofPort, webhookPort int
		leaderElection                                  bool
		overrideIPAddressS                              string
		cmd                                             = &cobra.Command{
			Use:           "manager",
			Version:       SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
			PreRun: func(cmd *cobra.Command, _ []string) {
				var (
					log = slog.New(slog.NewTextHandler(cmd.OutOrStdout(), &slog.HandlerOptions{
						Level: slog.Level(int(slog.LevelError) - 4*verbosity),
					}))
					slogr = logr.FromSlogHandler(log.Handler())
				)

				ctrl.SetLogger(slogr)
				cmd.SetContext(logr.NewContext(cmd.Context(), slogr))
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := ctrl.GetConfig()
				if err != nil {
					return err
				}

				ctx := cmd.Context()

				mgr, err := ctrl.NewManager(cfg, ctrl.Options{
					BaseContext:            cmd.Context,
					Scheme:                 scheme,
					HealthProbeBindAddress: BindAddressFromPort(healthPort),
					PprofBindAddress:       BindAddressFromPort(pprofPort),
					WebhookServer: webhook.NewServer(webhook.Options{
						Port: webhookPort,
					}),
					Metrics: server.Options{
						BindAddress: BindAddressFromPort(metricsPort),
					},
					Logger:                        logr.FromContextOrDiscard(ctx),
					LeaderElection:                leaderElection,
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

				if err := (&controllers.ServiceReconciler{
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

				//+kubebuilder:scaffold:builder

				return mgr.Start(ctx)
			},
		}
	)

	cmd.SetVersionTemplate("{{ .Name }}{{ .Version }} " + runtime.Version() + "\n")
	cmd.Flags().CountVarP(&verbosity, "verbose", "V", "verbosity for manager")

	// Just allow this flag to be passed, it's parsed by ctrl.GetConfig().
	cmd.PersistentFlags().String("kubeconfig", "", "Kube config")

	cmd.Flags().IntVar(&healthPort, "health-port", 8081, "health port")
	cmd.Flags().IntVar(&metricsPort, "metrics-port", 8082, "metrics port")
	cmd.Flags().IntVar(&pprofPort, "pprof-port", 8083, "pprof port")
	cmd.Flags().IntVar(&webhookPort, "webhook-port", webhook.DefaultPort, "webhook port")
	cmd.Flags().BoolVar(&leaderElection, "leader-election", false, "leader election")

	cmd.Flags().StringVar(&overrideIPAddressS, "override-ip-address", "", "IP address to use instead of getting it from a Service")

	return cmd
}

func BindAddressFromPort[T constraints.Integer](port T) string {
	if port <= 0 {
		return "0"
	}

	return fmt.Sprint(":", port)
}

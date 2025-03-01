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
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/frantjc/port-forward/internal/controller"
	"github.com/frantjc/port-forward/internal/portfwd/portfwdupnp"
	"github.com/frantjc/port-forward/internal/srcipmasq/srcipmasqiptables"
	"github.com/frantjc/port-forward/internal/svcip"
	"github.com/frantjc/port-forward/internal/svcip/svcipdef"
	"github.com/frantjc/port-forward/internal/svcip/svcipraw"
	"github.com/frantjc/port-forward/internal/upnp"
	xos "github.com/frantjc/x/os"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	// +kubebuilder:scaffold:imports
)

func main() {
	var (
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err       error
	)

	if err = NewEntrypoint().ExecuteContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
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
	// +kubebuilder:scaffold:scheme
}

// NewEntrypoint returns the command which acts as
// the entrypoint for `portfwd`.
func NewEntrypoint() *cobra.Command {
	var (
		metricsAddr                                      string
		metricsCertPath, metricsCertName, metricsCertKey string
		webhookCertPath, webhookCertName, webhookCertKey string
		enableLeaderElection                             bool
		probeAddr                                        string
		secureMetrics                                    bool
		enableHTTP2                                      bool
		verbosity                                        int
		overrideIPAddressS                               string
		cmd                                              = &cobra.Command{
			Use:           "portfwd",
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

				var (
					ctx     = cmd.Context()
					tlsOpts []func(*tls.Config)
				)

				if !enableHTTP2 {
					tlsOpts = append(tlsOpts, func(c *tls.Config) {
						c.NextProtos = []string{"http/1.1"}
					})
				}

				var (
					metricsCertWatcher *certwatcher.CertWatcher
					webhookCertWatcher *certwatcher.CertWatcher
					webhookTLSOpts     = tlsOpts
				)

				if len(webhookCertPath) > 0 {
					var err error
					webhookCertWatcher, err = certwatcher.New(
						filepath.Join(webhookCertPath, webhookCertName),
						filepath.Join(webhookCertPath, webhookCertKey),
					)
					if err != nil {
						return err
					}

					webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
						config.GetCertificate = webhookCertWatcher.GetCertificate
					})
				}

				webhookServer := webhook.NewServer(webhook.Options{
					TLSOpts: webhookTLSOpts,
				})

				metricsServerOptions := metricsserver.Options{
					BindAddress:   metricsAddr,
					SecureServing: secureMetrics,
					TLSOpts:       tlsOpts,
				}

				if secureMetrics {
					metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
				}

				if len(metricsCertPath) > 0 {
					var err error
					metricsCertWatcher, err = certwatcher.New(
						filepath.Join(metricsCertPath, metricsCertName),
						filepath.Join(metricsCertPath, metricsCertKey),
					)
					if err != nil {
						return err
					}

					metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
						config.GetCertificate = metricsCertWatcher.GetCertificate
					})
				}

				mgr, err := ctrl.NewManager(cfg, ctrl.Options{
					Scheme:                        scheme,
					Metrics:                       metricsServerOptions,
					WebhookServer:                 webhookServer,
					HealthProbeBindAddress:        probeAddr,
					LeaderElection:                enableLeaderElection,
					LeaderElectionID:              "e7a0a735.frantj.cc",
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

				// +kubebuilder:scaffold:builder

				if metricsCertWatcher != nil {
					if err := mgr.Add(metricsCertWatcher); err != nil {
						return err
					}
				}

				if webhookCertWatcher != nil {
					if err := mgr.Add(webhookCertWatcher); err != nil {
						return err
					}
				}

				return mgr.Start(ctx)
			},
		}
	)

	cmd.SetVersionTemplate("{{ .Name }}{{ .Version }} " + runtime.Version() + "\n")
	cmd.Flags().CountVarP(&verbosity, "verbose", "V", "verbosity for manager")

	// Just allow this flag to be passed, it's parsed by ctrl.GetConfig().
	cmd.PersistentFlags().String("kubeconfig", "", "Kube config")

	cmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service")
	cmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to")
	cmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager")
	cmd.Flags().BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead")
	cmd.Flags().StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate")
	cmd.Flags().StringVar(&webhookCertName, "webhook-cert-name", "tls.crt",
		"The name of the webhook certificate file")
	cmd.Flags().StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file")
	cmd.Flags().StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate")
	cmd.Flags().StringVar(&metricsCertName, "metrics-cert-name", "tls.crt",
		"The name of the metrics server certificate file")
	cmd.Flags().StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file")
	cmd.Flags().BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	cmd.Flags().StringVar(&overrideIPAddressS, "override-ip-address", "",
		"IP address to use instead of getting it from a Service")

	return cmd
}

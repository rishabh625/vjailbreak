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

// Package main is the entry point for the migration controller
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	utils "github.com/platform9/vjailbreak/k8s/migration/pkg/utils"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/k8s/migration/internal/controller"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vjailbreakv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr, probeAddr string
	var secureMetrics, enableLeaderElection, enableHTTP2, local bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metric endpoint binds to. "+
		"Use the port :8080. If not set, it will be 0 in order to disable the metrics server")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.BoolVar(&local, "local", false,
		"If set, the controller manager will run in local mode")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// create manager
	mgr, err := GetManager(metricsAddr, secureMetrics, tlsOpts, webhookServer, probeAddr, enableLeaderElection)
	if err != nil {
		setupLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	if err = SetupControllers(mgr, local); err != nil {
		setupLog.Error(err, "unable to set up controllers")
		os.Exit(1)
	}

	if err = (&controller.ESXIMigrationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ESXIMigration")
		os.Exit(1)
	}
	if err = (&controller.ClusterMigrationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterMigration")
		os.Exit(1)
	}
	if err = (&controller.BMConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BMConfig")
		os.Exit(1)
	}
	if err = (&controller.RDMDiskReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RDMDisk")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// handleStartupError logs the error and exits
	handleStartupError := func(err error, msg string) {
		setupLog.Error(err, msg)
		// Since we're in a separate function, os.Exit won't prevent defers from running
		os.Exit(1)
	}

	// Create a single ctx from signal handler to be reused
	ctx := ctrl.SetupSignalHandler()

	setupLog.Info("starting manager")
	// Start the manager's cache first
	go func() {
		if err = mgr.Start(ctx); err != nil {
			setupLog.Error(err, "problem running manager")
			os.Exit(1)
		}
	}()

	// Wait for cache to sync before using the client
	if ok := mgr.GetCache().WaitForCacheSync(ctx); !ok {
		handleStartupError(fmt.Errorf("failed to wait for caches to sync"), "Failed to sync cache")
	}

	// Now that cache is synced, we can create master node entry
	if err = utils.CheckAndCreateMasterNodeEntry(ctx, mgr.GetClient(), local); err != nil {
		handleStartupError(err, "Problem creating master node entry")
	}

	// Block forever
	select {}
}

// GetManager creates and configures a controller manager with the specified options
func GetManager(metricsAddr string,
	secureMetrics bool,
	tlsOpts []func(*tls.Config),
	webhookServer webhook.Server,
	probeAddr string,
	enableLeaderElection bool) (ctrl.Manager, error) {
	return ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "9cf7a6b4.k8s.pf9.io",
	})
}

// SetupControllers initializes and sets up all controllers with the manager
func SetupControllers(mgr ctrl.Manager, local bool) error {
	if err := (&controller.MigrationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Migration")
		return err
	}
	if err := (&controller.OpenstackCredsReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Local:  local,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenstackCreds")
		return err
	}
	if err := (&controller.VMwareCredsReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VMwareCreds")
		return err
	}
	if err := (&controller.StorageMappingReconciler{
		BaseReconciler: controller.BaseReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "StorageMapping")
		return err
	}
	if err := (&controller.NetworkMappingReconciler{
		BaseReconciler: controller.BaseReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NetworkMapping")
		return err
	}
	if err := (&controller.MigrationPlanReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MigrationPlan")
		return err
	}
	if err := (&controller.MigrationTemplateReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MigrationTemplate")
		return err
	}
	if err := (&controller.VjailbreakNodeReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Local:  local,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VjailbreakNode")
		return err
	}
	if err := (&controller.RollingMigrationPlanReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Local:  local,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RollingMigrationPlan")
		return err
	}

	return nil
}

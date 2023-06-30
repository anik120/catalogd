package main

import (
	"flag"
	"fmt"
	"net"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/klog/v2"

	"github.com/spf13/cobra"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericoptions "k8s.io/apiserver/pkg/server/options"

	"github.com/operator-framework/catalogd/api/optional/v1alpha1"
	"github.com/operator-framework/catalogd/pkg/catalogserver"
)

const defaultEtcdPathPrefix = "/storage/optional.catalogd.operator-framework.info"

func main() {

	stopCh := genericapiserver.SetupSignalHandler()
	options := NewCustomServerOptions()
	cmd := NewCommandStartCustomServer(options, stopCh)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	if err := cmd.Execute(); err != nil {
		klog.Fatal(err)
	}
}

type CustomServerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions
}

func NewCustomServerOptions() *CustomServerOptions {
	o := &CustomServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			catalogserver.Codecs.LegacyCodec(v1alpha1.GroupVersion),
		),
	}

	return o
}

// NewCommandStartCustomServer provides a CLI handler for 'start master' command
// with a default CustomServerOptions.
func NewCommandStartCustomServer(defaults *CustomServerOptions, stopCh <-chan struct{}) *cobra.Command {
	o := *defaults
	cmd := &cobra.Command{
		Short: "Launch a custom API server",
		Long:  "Launch a custom API server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	o.RecommendedOptions.AddFlags(flags)

	return cmd
}

func (o CustomServerOptions) Validate() error {
	errors := []error{}
	errors = append(errors, o.RecommendedOptions.Validate()...)
	return utilerrors.NewAggregate(errors)
}

func (o *CustomServerOptions) Complete() error {
	return nil
}

func (o *CustomServerOptions) Config() (*catalogserver.Config, error) {
	// TODO have a "real" external address
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	serverConfig := genericapiserver.NewRecommendedConfig(catalogserver.Codecs)
	if err := o.RecommendedOptions.ApplyTo(serverConfig); err != nil {
		return nil, err
	}

	config := &catalogserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig:   catalogserver.ExtraConfig{},
	}
	return config, nil
}

func (o CustomServerOptions) Run(stopCh <-chan struct{}) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New()
	if err != nil {
		return err
	}

	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}

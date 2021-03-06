// Package service implements business logic to issue flannelificates for clusters
// running on the Giantnetes platform.
package service

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/cenk/backoff"
	"github.com/coreos/etcd/client"
	"github.com/giantswarm/flanneltpr"
	"github.com/giantswarm/microendpoint/service/version"
	"github.com/giantswarm/microerror"
	microtls "github.com/giantswarm/microkit/tls"
	"github.com/giantswarm/micrologger"
	"github.com/giantswarm/operatorkit/client/k8sclient"
	"github.com/giantswarm/operatorkit/framework"
	"github.com/giantswarm/operatorkit/framework/resource/logresource"
	"github.com/giantswarm/operatorkit/framework/resource/metricsresource"
	"github.com/giantswarm/operatorkit/framework/resource/retryresource"
	"github.com/giantswarm/operatorkit/informer"
	"github.com/giantswarm/operatorkit/tpr"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	"github.com/giantswarm/flannel-operator/flag"
	"github.com/giantswarm/flannel-operator/service/etcdv2"
	"github.com/giantswarm/flannel-operator/service/healthz"
	"github.com/giantswarm/flannel-operator/service/operator"
	legacyresource "github.com/giantswarm/flannel-operator/service/resource/legacy"
	namespaceresource "github.com/giantswarm/flannel-operator/service/resource/namespace"
	networkconfigresource "github.com/giantswarm/flannel-operator/service/resource/networkconfig"
)

const (
	ResourceRetries uint64 = 3
)

// Config represents the configuration used to create a new service.
type Config struct {
	// Dependencies.
	Logger      micrologger.Logger
	VaultClient *vaultapi.Client

	// Settings.
	Flag  *flag.Flag
	Viper *viper.Viper

	Description string
	GitCommit   string
	Name        string
	Source      string
}

// DefaultConfig provides a default configuration to create a new service by
// best effort.
func DefaultConfig() Config {
	return Config{
		// Dependencies.
		Logger:      nil,
		VaultClient: nil,

		// Settings.
		Flag:  nil,
		Viper: nil,

		Description: "",
		GitCommit:   "",
		Name:        "",
		Source:      "",
	}
}

type Service struct {
	// Dependencies.
	Healthz  *healthz.Service
	Operator *operator.Operator
	Version  *version.Service

	// Internals.
	bootOnce sync.Once
}

func New(config Config) (*Service, error) {
	// Settings.
	if config.Flag == nil {
		return nil, microerror.Maskf(invalidConfigError, "config.Flag must not be empty")
	}
	if config.Viper == nil {
		return nil, microerror.Maskf(invalidConfigError, "config.Viper must not be empty")
	}

	var err error

	var k8sClient kubernetes.Interface
	{
		k8sConfig := k8sclient.DefaultConfig()

		k8sConfig.Address = config.Viper.GetString(config.Flag.Service.Kubernetes.Address)
		k8sConfig.Logger = config.Logger
		k8sConfig.InCluster = config.Viper.GetBool(config.Flag.Service.Kubernetes.InCluster)
		k8sConfig.TLS.CAFile = config.Viper.GetString(config.Flag.Service.Kubernetes.TLS.CAFile)
		k8sConfig.TLS.CrtFile = config.Viper.GetString(config.Flag.Service.Kubernetes.TLS.CrtFile)
		k8sConfig.TLS.KeyFile = config.Viper.GetString(config.Flag.Service.Kubernetes.TLS.KeyFile)

		k8sClient, err = k8sclient.New(k8sConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var tlsConfig *tls.Config
	{
		rootCAs := []string{}
		{
			v := config.Viper.GetString(config.Flag.Service.Etcd.TLS.CAFile)
			if v != "" {
				rootCAs = []string{v}
			}
		}
		certFiles := microtls.CertFiles{
			RootCAs: rootCAs,
			Cert:    config.Viper.GetString(config.Flag.Service.Etcd.TLS.CrtFile),
			Key:     config.Viper.GetString(config.Flag.Service.Etcd.TLS.KeyFile),
		}

		tlsConfig, err = microtls.LoadTLSConfig(certFiles)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var storageService *etcdv2.Service
	{
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig:     tlsConfig,
		}

		etcdConfig := client.Config{
			Endpoints: []string{config.Viper.GetString(config.Flag.Service.Etcd.Endpoint)},
			Transport: transport,
		}
		etcdClient, err := client.New(etcdConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}

		storageConfig := etcdv2.DefaultConfig()
		storageConfig.EtcdClient = etcdClient
		storageService, err = etcdv2.New(storageConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var legacyResourceBackOff *backoff.ExponentialBackOff
	{
		legacyResourceBackOff = backoff.NewExponentialBackOff()
		legacyResourceBackOff.MaxElapsedTime = 5 * time.Minute
	}

	var legacyResource framework.Resource
	{
		legacyConfig := legacyresource.DefaultConfig()

		legacyConfig.BackOff = legacyResourceBackOff
		legacyConfig.K8sClient = k8sClient
		legacyConfig.Logger = config.Logger

		legacyConfig.EtcdCAFile = config.Viper.GetString(config.Flag.Service.Etcd.TLS.CAFile)
		legacyConfig.EtcdCrtFile = config.Viper.GetString(config.Flag.Service.Etcd.TLS.CrtFile)
		legacyConfig.EtcdKeyFile = config.Viper.GetString(config.Flag.Service.Etcd.TLS.KeyFile)

		legacyResource, err = legacyresource.New(legacyConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var networkConfigResource framework.Resource
	{
		c := networkconfigresource.DefaultConfig()

		c.Logger = config.Logger
		c.Store = storageService

		networkConfigResource, err = networkconfigresource.New(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var namespaceResource framework.Resource
	{
		namespaceConfig := namespaceresource.DefaultConfig()

		namespaceConfig.K8sClient = k8sClient
		namespaceConfig.Logger = config.Logger

		namespaceResource, err = namespaceresource.New(namespaceConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var resources []framework.Resource
	{
		resources = []framework.Resource{
			networkConfigResource,
			namespaceResource,
			legacyResource,
		}

		logWrapConfig := logresource.DefaultWrapConfig()
		logWrapConfig.Logger = config.Logger
		resources, err = logresource.Wrap(resources, logWrapConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}

		retryWrapConfig := retryresource.DefaultWrapConfig()
		retryWrapConfig.BackOffFactory = func() backoff.BackOff { return backoff.WithMaxTries(backoff.NewExponentialBackOff(), ResourceRetries) }
		retryWrapConfig.Logger = config.Logger
		resources, err = retryresource.Wrap(resources, retryWrapConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}

		metricsWrapConfig := metricsresource.DefaultWrapConfig()
		metricsWrapConfig.Name = config.Name
		resources, err = metricsresource.Wrap(resources, metricsWrapConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	initCtxFunc := func(ctx context.Context, obj interface{}) (context.Context, error) {
		return ctx, nil
	}

	var frameworkBackOff *backoff.ExponentialBackOff
	{
		frameworkBackOff = backoff.NewExponentialBackOff()
		frameworkBackOff.MaxElapsedTime = 5 * time.Minute
	}

	var operatorFramework *framework.Framework
	{
		frameworkConfig := framework.DefaultConfig()

		frameworkConfig.BackOff = frameworkBackOff
		frameworkConfig.InitCtxFunc = initCtxFunc
		frameworkConfig.Logger = config.Logger
		frameworkConfig.Resources = resources

		operatorFramework, err = framework.New(frameworkConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var newTPR *tpr.TPR
	{
		c := tpr.DefaultConfig()

		c.K8sClient = k8sClient
		c.Logger = config.Logger

		c.Description = flanneltpr.Description
		c.Name = flanneltpr.Name
		c.Version = flanneltpr.VersionV1

		newTPR, err = tpr.New(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var newWatcherFactory informer.WatcherFactory
	{
		zeroObjectFactory := &informer.ZeroObjectFactoryFuncs{
			NewObjectFunc:     func() runtime.Object { return &flanneltpr.CustomObject{} },
			NewObjectListFunc: func() runtime.Object { return &flanneltpr.List{} },
		}
		newWatcherFactory = informer.NewWatcherFactory(k8sClient.Discovery().RESTClient(), newTPR.WatchEndpoint(""), zeroObjectFactory)
	}

	var newInformer *informer.Informer
	{
		informerConfig := informer.DefaultConfig()

		informerConfig.BackOff = backoff.NewExponentialBackOff()
		informerConfig.WatcherFactory = newWatcherFactory

		informerConfig.RateWait = time.Second * 10
		informerConfig.ResyncPeriod = time.Minute * 5

		newInformer, err = informer.New(informerConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var healthzService *healthz.Service
	{
		healthzConfig := healthz.DefaultConfig()

		healthzConfig.K8sClient = k8sClient
		healthzConfig.Logger = config.Logger

		healthzService, err = healthz.New(healthzConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var operatorBackOff *backoff.ExponentialBackOff
	{
		operatorBackOff = backoff.NewExponentialBackOff()
		operatorBackOff.MaxElapsedTime = 5 * time.Minute
	}

	var operatorService *operator.Operator
	{
		operatorConfig := operator.DefaultConfig()

		operatorConfig.BackOff = operatorBackOff
		operatorConfig.Framework = operatorFramework
		operatorConfig.Informer = newInformer
		operatorConfig.Logger = config.Logger
		operatorConfig.TPR = newTPR

		operatorService, err = operator.New(operatorConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var versionService *version.Service
	{
		versionConfig := version.DefaultConfig()

		versionConfig.Description = config.Description
		versionConfig.GitCommit = config.GitCommit
		versionConfig.Name = config.Name
		versionConfig.Source = config.Source
		versionConfig.VersionBundles = NewVersionBundles()

		versionService, err = version.New(versionConfig)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	newService := &Service{
		// Dependencies.
		Healthz:  healthzService,
		Operator: operatorService,
		Version:  versionService,

		// Internals
		bootOnce: sync.Once{},
	}

	return newService, nil
}

func (s *Service) Boot() {
	s.bootOnce.Do(func() {
		s.Operator.Boot()
	})
}

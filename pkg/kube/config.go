package kube

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientOptions used to build kube rest config.
type ClientOptions struct {
	Master     string
	KubeConfig string
	QPS        float32
	Burst      int
}

// BuildConfig builds kube rest config with the given options.
func BuildConfig(opt ClientOptions) (*rest.Config, error) {
	var cfg *rest.Config
	var err error

	master := opt.Master
	kubeconfig := opt.KubeConfig
	cfg, err = clientcmd.BuildConfigFromFlags(master, kubeconfig)
	if err != nil {
		return nil, err
	}
	cfg.QPS = opt.QPS
	cfg.Burst = opt.Burst

	return cfg, nil
}
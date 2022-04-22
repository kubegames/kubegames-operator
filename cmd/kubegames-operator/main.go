package main

import (
	"flag"
	"net/http"
	"path/filepath"

	"github.com/kubegames/kubegames-operator/internal/pkg/log"
	"github.com/kubegames/kubegames-operator/pkg/admission"
	"github.com/kubegames/kubegames-operator/pkg/game"
	"github.com/kubegames/kubegames-operator/pkg/pod"
	"github.com/kubegames/kubegames-operator/pkg/signals"
	"github.com/kubegames/kubegames-operator/pkg/webhook"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	tlsDir      = `/run/secrets/tls`
	tlsCertFile = `tls.crt`
	tlsKeyFile  = `tls.key`
)

var (
	cfg         string
	kubeconfig  string
	threadiness int
)

func init() {
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) kubeconfig absolute path to the file")
		flag.StringVar(&kubeconfig, "k", filepath.Join(home, ".kube", "config"), "(optional) kubeconfig absolute path to the file")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "(optional) kubeconfig absolute path to the file")
		flag.StringVar(&kubeconfig, "k", "", "(optional) kubeconfig absolute path to the file")
	}
	flag.IntVar(&threadiness, "threadiness", 1, "kubegames controller worker threadiness")
	flag.IntVar(&threadiness, "t", 1, "kubegames controller worker threadiness")
}

func main() {
	flag.Parse()

	// handler signal
	stopCh := signals.SetupSignalHandler()

	//nee k8s client
	var config *rest.Config
	var err error

	if len(kubeconfig) > 0 {
		if config, err = clientcmd.BuildConfigFromFlags("", kubeconfig); err != nil {
			panic(err.Error())
		}
	} else {
		if config, err = rest.InClusterConfig(); err != nil {
			panic(err.Error())
		}
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	//new game
	game := game.NewGame(kubeClient, config)
	go game.Run(threadiness, stopCh)

	//new pod
	pod := pod.NewPod(kubeClient, config)
	go pod.Run(threadiness, stopCh)

	//run http
	go func() {
		certPath := filepath.Join(tlsDir, tlsCertFile)
		keyPath := filepath.Join(tlsDir, tlsKeyFile)
		mux := http.NewServeMux()
		mux.Handle("/validating", admission.AdmissionFuncHandler(webhook.Validating))
		mux.Handle("/mutating", admission.AdmissionFuncHandler(webhook.Mutating))

		server := &http.Server{
			Addr:    ":443",
			Handler: mux,
		}

		if err := server.ListenAndServeTLS(certPath, keyPath); err != nil {
			panic(err)
		}
	}()

	log.Infof("kubegames operator start")
	<-stopCh
}

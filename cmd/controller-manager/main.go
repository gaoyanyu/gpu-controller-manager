package main

import (
	"fmt"
	"github.com/spf13/pflag"
	"gpu-extend-controller/cmd/controller-manager/app"
	"gpu-extend-controller/cmd/controller-manager/app/options"
	"gpu-extend-controller/pkg/version"
	"k8s.io/apimachinery/pkg/util/wait"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog"
	"os"
	"runtime"
	"time"
)

var logFlushFreq = pflag.Duration("log-flush-frequency", 5*time.Second, "Maximum number of seconds between log flushes")

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	klog.InitFlags(nil)

	s := options.NewServerOption()
	s.AddFlags(pflag.CommandLine)

	cliflag.InitFlags()

	if s.PrintVersion {
		version.PrintVersionAndExit()
	}
	if err := s.CheckOptionOrDie(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if s.CertFile != "" && s.KeyFile != "" {
		if err := s.ParseCAFiles(nil); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse CA file: %v\n", err)
			os.Exit(1)
		}
	}

	// The default klog flush interval is 30 seconds, which is frighteningly long.
	go wait.Until(klog.Flush, *logFlushFreq, wait.NeverStop)
	defer klog.Flush()

	if err := app.Run(s); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
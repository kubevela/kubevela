package commands

import (
	"context"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"

	"github.com/spf13/cobra"
	sternCmd "github.com/wercker/stern/cmd"
	"github.com/wercker/stern/kubernetes"
	"github.com/wercker/stern/stern"
)

func NewLogsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "logs <appname>"
	cmd.Short = "Tail pods logs of an application"
	cmd.Long = "Tail pods logs of an application"
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			ioStreams.Errorf("please specify the application name")
		}
		appName := args[0]
		cmd.SetArgs([]string{appName + ".*"})
		env, err := GetEnv(cmd)
		if err != nil {
			return err
		}
		namespace := env.Namespace
		cmd.Flags().String("namespace", namespace, "")
		return nil
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		config, err := sternCmd.ParseConfig(args)
		if err != nil {
			return err
		}
		ctx := context.Background()
		if err := Run(ctx, config, ioStreams); err != nil {
			return err
		}
		return nil
	}
	cmd.Annotations = map[string]string{
		types.TagCommandType: types.TypeApp,
	}
	return cmd
}

// Run refer to the implementation at https://github.com/oam-dev/stern/blob/master/stern/main.go
func Run(ctx context.Context, config *stern.Config, ioStreams cmdutil.IOStreams) error {
	clientConfig := kubernetes.NewClientConfig(config.KubeConfig, config.ContextName)
	clientSet, err := kubernetes.NewClientSet(clientConfig)
	if err != nil {
		return err
	}
	namespace := config.Namespace
	added, removed, err := stern.Watch(ctx, clientSet.CoreV1().Pods(namespace), config.PodQuery, config.ContainerQuery, config.ExcludeContainerQuery, config.ContainerState, config.LabelSelector)
	if err != nil {
		return err
	}
	tails := make(map[string]*stern.Tail)
	logC := make(chan string, 1024)

	go func() {
		for {
			select {
			case str := <-logC:
				ioStreams.Infonln(str)
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		for p := range added {
			id := p.GetID()
			if tails[id] != nil {
				continue
			}

			tail := stern.NewTail(p.Namespace, p.Pod, p.Container, config.Template, &stern.TailOptions{
				Timestamps:   config.Timestamps,
				SinceSeconds: int64(config.Since.Seconds()),
				Exclude:      config.Exclude,
				Include:      config.Include,
				Namespace:    config.AllNamespaces,
				TailLines:    config.TailLines,
			})
			tails[id] = tail

			tail.Start(ctx, clientSet.CoreV1().Pods(p.Namespace), logC)
		}
	}()

	go func() {
		for p := range removed {
			id := p.GetID()
			if tails[id] == nil {
				continue
			}
			tails[id].Close()
			delete(tails, id)
		}
	}()

	<-ctx.Done()

	return nil
}

package server

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest"
)

type server struct {
	restCfg rest.Config
}

// NewServerCommand create server command
func NewServerCommand() *cobra.Command {
	s := &server{}

	cmd := &cobra.Command{
		Use:   "apiserver",
		Short: "Start running apiserver.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.run()
		},
	}

	// rest
	cmd.Flags().IntVar(&s.restCfg.Port, "port", 8000, "The port number used to serve the http APIs.")

	return cmd
}

func (s *server) run() error {
	ctx := context.Background()

	server, err := rest.New(s.restCfg)
	if err != nil {
		return fmt.Errorf("create apiserver failed : %s ", err.Error())
	}
	return server.Run(ctx)
}

package commands

import (
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/AlecAivazis/survey/v2"
	"github.com/gosuri/uitable"
	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/spf13/cobra"
)

func NewAppShowCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show <APPLICATION-NAME>",
		Short:   "Get details of an application",
		Long:    "Get details of an application",
		Example: `vela show <APPLICATION-NAME>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength == 0 {
				ioStreams.Errorf("Hint: please specify the application name\n")
				os.Exit(1)
			}
			appName := args[0]
			env, err := GetEnv(cmd)
			if err != nil {
				ioStreams.Errorf("Error: failed to get Env: %s", err)
				return err
			}

			return showApplication(cmd, env, appName)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.Flags().StringP("svc", "s", "", "service name")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func showApplication(cmd *cobra.Command, env *types.EnvMeta, appName string) error {
	app, err := application.Load(env.Name, appName)
	if err != nil {
		return err
	}

	var svcFlag, chosenSvc string
	var svcFlagStatus string
	// to store the value of flag `--svc` set in Cli, or selected value in survey
	var targetServices []string
	if svcFlag = cmd.Flag("svc").Value.String(); svcFlag == "" {
		svcFlagStatus = oam.FlagNotSet
	} else {
		svcFlagStatus = oam.FlagIsInvalid
	}
	// all services name of the application `appName`
	var services []string
	for svcName := range app.Services {
		services = append(services, svcName)
		if svcFlag == svcName {
			svcFlagStatus = oam.FlagIsValid
			targetServices = append(targetServices, svcName)
		}
	}
	totalServices := len(services)
	if svcFlagStatus == oam.FlagNotSet && totalServices == 1 {
		targetServices = services
	}
	if svcFlagStatus == oam.FlagIsInvalid || (svcFlagStatus == oam.FlagNotSet && totalServices > 1) {
		if svcFlagStatus == oam.FlagIsInvalid {
			cmd.Printf("The service name '%s' is not valid\n", svcFlag)
		}
		chosenSvc, err = chooseSvc(services)
		if err != nil {
			return err
		}

		if chosenSvc == oam.DefaultChosenAllSvc {
			targetServices = services
		} else {
			targetServices = targetServices[:0]
			targetServices = append(targetServices, chosenSvc)
		}
	}

	cmd.Printf("About:\n\n")
	table := uitable.New()
	table.AddRow("  Name:", appName)
	table.AddRow("  Created at:", app.CreateTime.String())
	table.AddRow("  Updated at:", app.UpdateTime.String())
	cmd.Printf("%s\n\n", table.String())

	cmd.Println()
	cmd.Printf("Environment:\n\n")
	cmd.Printf("  Namespace:\t%s\n", env.Namespace)
	cmd.Println()

	table = uitable.New()
	cmd.Printf("Services:\n\n")

	for _, svcName := range targetServices {
		if err := showComponent(cmd, env, svcName, appName); err != nil {
			return err
		}
	}
	cmd.Println(table.String())
	cmd.Println()
	return nil
}

func showComponent(cmd *cobra.Command, env *types.EnvMeta, compName, appName string) error {
	var app *application.Application
	var err error
	if appName != "" {
		app, err = application.Load(env.Name, appName)
	} else {
		app, err = application.MatchAppByComp(env.Name, compName)
	}
	if err != nil {
		return err
	}

	for cname := range app.Services {
		if cname != compName {
			continue
		}
		wtype, data := app.GetWorkload(compName)
		table := uitable.New()
		table.AddRow("  - Name:", compName)
		table.AddRow("    WorkloadType:", wtype)
		cmd.Printf(table.String())
		cmd.Printf("\n    Arguments:\n")
		table = uitable.New()
		for k, v := range data {
			table.AddRow(fmt.Sprintf("      %s:        ", k), v)
		}
		cmd.Printf("%s", table.String())
		traits, err := app.GetTraits(compName)
		if err != nil {
			cmd.PrintErr(err)
			continue
		}
		cmd.Println()
		cmd.Printf("      Traits:\n")
		for k, v := range traits {
			cmd.Printf("        - %s:\n", k)
			table = uitable.New()
			for kk, vv := range v {
				table.AddRow(fmt.Sprintf("            %s:", kk), vv)
			}
			cmd.Printf("%s\n", table.String())
		}
		cmd.Println()
	}
	return nil
}

func chooseSvc(services []string) (string, error) {
	var svcName string
	services = append(services, oam.DefaultChosenAllSvc)
	prompt := &survey.Select{
		Message: "Please choose one service: ",
		Options: services,
		Default: oam.DefaultChosenAllSvc,
	}
	err := survey.AskOne(prompt, &svcName)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve services of the application, err %v", err)
	}
	return svcName, nil
}

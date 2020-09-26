package commands

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"cuelang.org/go/cue"
	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/kyokomi/emoji"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/duration"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/plugins"
)

type appInitOptions struct {
	client client.Client
	cmdutil.IOStreams
	Env *types.EnvMeta

	app          *application.Application
	appName      string
	workloadName string
	workloadType string
}

type CompStatus int

const (
	compStatusInitializing CompStatus = iota
	compStatusInitFail
	compStatusInitialized
	compStatusDeploying
	compStatusDeployFail
	compStatusDeployed
	compStatusHealthChecking
	compStatusHealthCheckDone
	compStatusUnknown
)

var (
	emojiSucceed = emoji.Sprint(":check_mark_button:")
	emojiFail    = emoji.Sprint(":cross_mark:")
	emojiTimeout = emoji.Sprint(":heavy_exclamation_mark:")
)

// NewInitCommand init application
func NewInitCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &appInitOptions{IOStreams: ioStreams}
	cmd := &cobra.Command{
		Use:                   "init",
		DisableFlagsInUseLine: true,
		Short:                 "Init an OAM Application",
		Long:                  "Init an OAM Application by one command",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
		Example: "vela init",
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			o.client = newClient
			o.Env, err = GetEnv(cmd)
			if err != nil {
				return err
			}
			o.IOStreams.Info("Welcome to use KubeVela CLI! We're going to help you run applications through a couple of questions.")
			o.IOStreams.Info()
			if err = o.CheckEnv(); err != nil {
				return err
			}
			if err = o.Naming(); err != nil {
				return err
			}
			if err = o.Workload(); err != nil {
				return err
			}
			if err = o.Traits(); err != nil {
				return err
			}
			_, err = oam.BaseRun(false, o.app, o.client, o.Env)
			if err != nil {
				return err
			}

			tInit := time.Now()
			sInit := spinner.New(spinner.CharSets[14], 100*time.Millisecond,
				spinner.WithColor("green"),
				spinner.WithFinalMSG(""),
				spinner.WithHiddenCursor(true),
				spinner.WithSuffix(color.New(color.Bold, color.FgGreen).Sprintf(" %s", "Initializing ...")))
			sInit.Start()
		TrackInitLoop:
			for {
				time.Sleep(2 * time.Second)
				if time.Since(tInit) > initTimeout {
					ioStreams.Info(red.Sprintf("\n%sInitialization Timeout After %s!", emojiTimeout, duration.HumanDuration(time.Since(tInit))))
					ioStreams.Info(red.Sprint("Please make sure oam-core-controller is installed."))
					sInit.Stop()
					return nil
				}
				initStatus, failMsg, err := trackInitializeStatus(context.Background(), o.client, o.workloadName, o.appName, o.Env)
				if err != nil {
					return err
				}
				switch initStatus {
				case compStatusInitializing:
					continue
				case compStatusInitialized:
					ioStreams.Info(green.Sprintf("\n%sInitialization Succeed!", emojiSucceed))
					sInit.Stop()
					break TrackInitLoop
				case compStatusInitFail:
					ioStreams.Info(red.Sprintf("\n%sInitialization Failed!", emojiFail))
					ioStreams.Info(red.Sprintf("Reason: %s", failMsg))
					sInit.Stop()
					return nil
				}
			}

			sDeploy := spinner.New(spinner.CharSets[14], 100*time.Millisecond,
				spinner.WithColor("green"),
				spinner.WithHiddenCursor(true),
				spinner.WithSuffix(color.New(color.Bold, color.FgGreen).Sprintf(" %s", "Deploying ...")))
			sDeploy.Start()
		TrackDeployLoop:
			for {
				time.Sleep(2 * time.Second)
				deployStatus, failMsg, err := trackDeployStatus(context.Background(), o.client, o.workloadName, o.appName, o.Env)
				if err != nil {
					return err
				}
				switch deployStatus {
				case compStatusDeploying:
					continue
				case compStatusDeployed:
					ioStreams.Info(green.Sprintf("\n%sDeployment Succeed!", emojiSucceed))
					sDeploy.Stop()
					break TrackDeployLoop
				case compStatusDeployFail:
					ioStreams.Info(red.Sprintf("\n%sDeployment Failed!", emojiFail))
					ioStreams.Info(red.Sprintf("Reason: %s", failMsg))
					sDeploy.Stop()
					return nil
				}
			}
			return printComponentStatus(context.Background(), o.client, o.IOStreams, o.workloadName, o.appName, o.Env)
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func (o *appInitOptions) Naming() error {
	prompt := &survey.Input{
		Message: "What would you like to name your application: ",
	}
	err := survey.AskOne(prompt, &o.appName)
	if err != nil {
		return fmt.Errorf("read app name err %v", err)
	}
	return nil
}

func (o *appInitOptions) CheckEnv() error {
	if o.Env.Namespace == "" {
		o.Env.Namespace = "default"
	}
	o.Infof("Environment: %s, namespace: %s\n\n", o.Env.Name, o.Env.Namespace)
	if o.Env.Domain == "" {
		prompt := &survey.Input{
			Message: "Do you want to setup a domain for web service: ",
		}
		err := survey.AskOne(prompt, &o.Env.Domain)
		if err != nil {
			return fmt.Errorf("read app name err %v", err)
		}
	}
	if o.Env.Email == "" {
		prompt := &survey.Input{
			Message: "Provide an email for production certification: ",
		}
		err := survey.AskOne(prompt, &o.Env.Email)
		if err != nil {
			return fmt.Errorf("read app name err %v", err)
		}
	}
	if _, err := oam.CreateOrUpdateEnv(context.Background(), o.client, o.Env.Name, o.Env); err != nil {
		return err
	}
	return nil
}

func (o *appInitOptions) Workload() error {
	workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		return err
	}
	var workloadList []string
	for _, w := range workloads {
		workloadList = append(workloadList, w.Name)
	}
	prompt := &survey.Select{
		Message: "Choose an workload for your component: ",
		Options: workloadList,
	}
	err = survey.AskOne(prompt, &o.workloadType)
	if err != nil {
		return fmt.Errorf("read workload type err %v", err)
	}
	workload, err := GetCapabilityByName(o.workloadType, workloads)
	if err != nil {
		return err
	}
	namePrompt := &survey.Input{
		Message: fmt.Sprintf("What would you name this %s: ", o.workloadType),
	}
	err = survey.AskOne(namePrompt, &o.workloadName)
	if err != nil {
		return fmt.Errorf("read name err %v", err)
	}
	fs := pflag.NewFlagSet("workload", pflag.ContinueOnError)
	for _, p := range workload.Parameters {
		if p.Name == "name" {
			continue
		}
		usage := p.Usage
		if usage == "" {
			usage = "what would you configure for parameter '" + color.New(color.FgCyan).Sprintf("%s", p.Name) + "'"
		}
		switch p.Type {
		case cue.StringKind:
			var data string
			prompt := &survey.Input{
				Message: usage,
			}
			var opts []survey.AskOpt
			if p.Required {
				opts = append(opts, survey.WithValidator(survey.Required))
			}
			err = survey.AskOne(prompt, &data, opts...)
			if err != nil {
				return fmt.Errorf("read param %s err %v", p.Name, err)
			}
			fs.String(p.Name, data, p.Usage)
		case cue.NumberKind, cue.FloatKind:
			var data string
			prompt := &survey.Input{
				Message: usage,
			}
			var opts []survey.AskOpt
			if p.Required {
				opts = append(opts, survey.WithValidator(survey.Required))
			}
			opts = append(opts, survey.WithValidator(func(ans interface{}) error {
				data := ans.(string)
				if data == "" && !p.Required {
					return nil
				}
				_, err := strconv.ParseFloat(data, 64)
				return err
			}))
			err = survey.AskOne(prompt, &data, opts...)
			if err != nil {
				return fmt.Errorf("read param %s err %v", p.Name, err)
			}
			val, _ := strconv.ParseFloat(data, 64)
			fs.Float64(p.Name, val, p.Usage)
		case cue.IntKind:
			var data string
			prompt := &survey.Input{
				Message: usage,
			}
			var opts []survey.AskOpt
			if p.Required {
				opts = append(opts, survey.WithValidator(survey.Required))
			}
			opts = append(opts, survey.WithValidator(func(ans interface{}) error {
				data := ans.(string)
				if data == "" && !p.Required {
					return nil
				}
				_, err := strconv.ParseInt(data, 10, 64)
				return err
			}))
			err = survey.AskOne(prompt, &data, opts...)
			if err != nil {
				return fmt.Errorf("read param %s err %v", p.Name, err)
			}
			val, _ := strconv.ParseInt(data, 10, 64)
			fs.Int64(p.Name, val, p.Usage)
		case cue.BoolKind:
			var data bool
			prompt := &survey.Confirm{
				Message: usage,
			}
			if p.Required {
				err = survey.AskOne(prompt, &data, survey.WithValidator(survey.Required))
			} else {
				err = survey.AskOne(prompt, &data)
			}
			if err != nil {
				return fmt.Errorf("read param %s err %v", p.Name, err)
			}
			fs.Bool(p.Name, data, p.Usage)
		}
	}
	o.app, err = oam.BaseComplete(o.Env.Name, o.workloadName, o.appName, fs, o.workloadType)
	return err
}

func GetCapabilityByName(name string, workloads []types.Capability) (types.Capability, error) {
	for _, v := range workloads {
		if v.Name == name {
			return v, nil
		}
	}
	return types.Capability{}, fmt.Errorf("%s not found", name)
}

func (o *appInitOptions) Traits() error {
	traits, err := plugins.LoadInstalledCapabilityWithType(types.TypeTrait)
	if err != nil {
		return err
	}
	switch o.workloadType {
	case "webservice":
		//TODO(wonderflow) this should get from workload definition to know which trait should be suggestions
		var suggestTraits = []string{"route"}
		for _, tr := range suggestTraits {
			trait, err := GetCapabilityByName(tr, traits)
			if err != nil {
				continue
			}
			tflags := pflag.NewFlagSet("trait", pflag.ContinueOnError)
			for _, pa := range trait.Parameters {
				types.SetFlagBy(tflags, pa)
			}
			//TODO(wonderflow): give a way to add parameter for trait
			o.app, err = oam.AddOrUpdateTrait(o.Env, o.appName, o.workloadName, tflags, trait)
			if err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

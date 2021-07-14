package cli

import (
	"fmt"
	"github.com/aws/copilot-cli/internal/pkg/config"
	"github.com/aws/copilot-cli/internal/pkg/deploy"
	"github.com/aws/copilot-cli/internal/pkg/term/prompt"
	"github.com/aws/copilot-cli/internal/pkg/term/selector"
	"github.com/spf13/cobra"
)

type svcRestartVars struct {
	appName string
	envName string
	svcName string
}

type svcRestartOpts struct {
	svcRestartVars
	store store
	sel   *selector.DeploySelect
}

func (o *svcRestartOpts) Validate() error {
	if o.appName != "" {
		if _, err := o.store.GetApplication(o.appName); err != nil {
			return err
		}
	}
	if o.svcName != "" {
		if _, err := o.store.GetService(o.appName, o.svcName); err != nil {
			return err
		}
	}
	if o.envName != "" {
		if _, err := o.store.GetEnvironment(o.appName, o.envName); err != nil {
			return err
		}
	}
	return nil
}

func (o *svcRestartOpts) Ask() error {
	if err := o.askApp(); err != nil {
		return err
	}
	return o.askSvcEnvName()
}

func (o *svcRestartOpts) Execute() error {
	return nil
}

func (o *svcRestartOpts) askApp() error {
	if o.appName != "" {
		return nil
	}
	app, err := o.sel.Application(svcAppNamePrompt, svcAppNameHelpPrompt)
	if err != nil {
		return fmt.Errorf("select application: %w", err)
	}
	o.appName = app
	return nil
}

func (o *svcRestartOpts) askSvcEnvName() error {
	deployedService, err := o.sel.DeployedService(svcStatusNamePrompt, svcStatusNameHelpPrompt, o.appName, selector.WithEnv(o.envName), selector.WithSvc(o.svcName))
	if err != nil {
		return fmt.Errorf("select deployed services for application %s: %w", o.appName, err)
	}
	o.svcName = deployedService.Svc
	o.envName = deployedService.Env
	return nil
}

func newSvcRestartOpts(vars svcRestartVars) (*svcRestartOpts, error) {
	configStore, err := config.NewStore()
	if err != nil {
		return nil, fmt.Errorf("connect to environment datastore: %w", err)
	}
	deployStore, err := deploy.NewStore(configStore)
	if err != nil {
		return nil, fmt.Errorf("connect to deploy store: %w", err)
	}
	return &svcRestartOpts{
		svcRestartVars: vars,
		store:          configStore,
		sel:            selector.NewDeploySelect(prompt.New(), configStore, deployStore),
	}, nil
}

// buildSvcRestartCmd builds the command for restarting a deployed service.
func buildSvcRestartCmd() *cobra.Command {
	vars := svcRestartVars{}
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart a deployed service.",
		Long:  "Restart a deployed service by rolling out a forced deployment with current configuration",

		Example: `
  Restarts the deployed service "my-svc"
  /code $ copilot svc restart -n my-svc`,
		RunE: runCmdE(func(cmd *cobra.Command, args []string) error {
			opts, err := newSvcRestartOpts(vars)
			if err != nil {
				return err
			}
			if err := opts.Validate(); err != nil {
				return err
			}
			if err := opts.Ask(); err != nil {
				return err
			}
			return opts.Execute()
		}),
	}
	cmd.Flags().StringVarP(&vars.svcName, nameFlag, nameFlagShort, "", svcFlagDescription)
	cmd.Flags().StringVarP(&vars.envName, envFlag, envFlagShort, "", envFlagDescription)
	cmd.Flags().StringVarP(&vars.appName, appFlag, appFlagShort, tryReadingAppName(), appFlagDescription)
	return cmd
}

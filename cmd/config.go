package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/urfave/cli"

	"github.com/manifoldco/manifold-cli/api"
	"github.com/manifoldco/manifold-cli/clients"
	"github.com/manifoldco/manifold-cli/errs"
	"github.com/manifoldco/manifold-cli/middleware"
	"github.com/manifoldco/manifold-cli/prompts"

	"github.com/manifoldco/manifold-cli/generated/marketplace/client/credential"
	"github.com/manifoldco/manifold-cli/generated/marketplace/models"
)

var configKeyRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,1000}$`)

func init() {
	cmd := cli.Command{
		Name:     "config",
		Usage:    "View and modify resource configuration",
		Category: "CONFIGURATION",
		Subcommands: []cli.Command{
			{
				Name:      "set",
				ArgsUsage: "<key=value...>",
				Usage:     "Set one or more config values on a custom resource",
				Flags: append(teamFlags, []cli.Flag{
					resourceFlag(),
				}...),
				Action: middleware.Chain(middleware.EnsureSession, middleware.LoadTeamPrefs,
					configSetCmd),
			},
			{
				Name:      "unset",
				ArgsUsage: "<key...>",
				Usage:     "Unset one or more config values on a custom resource",
				Flags: append(teamFlags, []cli.Flag{
					resourceFlag(),
				}...),
				Action: middleware.Chain(middleware.EnsureSession, middleware.LoadTeamPrefs,
					configUnsetCmd),
			},
		},
	}

	cmds = append(cmds, cmd)
}

func patchConfig(cliCtx *cli.Context, req map[string]*string) error {
	ctx := context.Background()
	if len(req) == 0 {
		return errs.NewUsageExitError(cliCtx, fmt.Errorf("At least one key must be present"))
	}

	for k := range req {
		if !configKeyRegexp.MatchString(k) {
			return cli.NewExitError(fmt.Sprintf("Bad config key `%s`", k), -1)
		}
	}

	name, err := requiredName(cliCtx, "resource")
	if err != nil {
		return err
	}

	teamID, err := validateTeamID(cliCtx)
	if err != nil {
		return err
	}

	client, err := api.New(api.Marketplace)
	if err != nil {
		return err
	}

	resources, err := clients.FetchResources(ctx, client.Marketplace, teamID, "")
	if err != nil {
		return cli.NewExitError("Could not retrieve resources: "+err.Error(), -1)
	}

	// XXX just get a single resource
	var resource *models.Resource
	for _, r := range resources {
		if string(r.Body.Label) == name {
			resource = r
			break
		}
	}

	if resource == nil {
		return cli.NewExitError("No resource found with that name", -1)
	}
	if *resource.Body.Source != "custom" {
		return cli.NewExitError("Config can only be set on custom resources", -1)
	}

	prompts.SpinStart("Updating resource config")
	_, err = client.Marketplace.Credential.PatchResourcesIDConfig(&credential.PatchResourcesIDConfigParams{
		ID:      resource.ID.String(),
		Body:    req,
		Context: ctx,
	}, nil)
	prompts.SpinStop()

	if err != nil {
		switch e := err.(type) {
		case *credential.PatchResourcesIDConfigBadRequest:
			return cli.NewExitError("Could not change config: invalid key.", -1)
		default:
			return cli.NewExitError(e, -1)
		}
	}

	fmt.Println("Your configuration has been updated.")
	fmt.Println("")
	fmt.Println("Use `manifold export` to review your config.")
	return nil
}

func configSetCmd(cliCtx *cli.Context) error {
	req := make(map[string]*string)
	for _, arg := range cliCtx.Args() {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return cli.NewExitError("Config must be of the form KEY=VALUE", -1)
		}
		req[parts[0]] = &parts[1]
	}
	return patchConfig(cliCtx, req)
}

func configUnsetCmd(cliCtx *cli.Context) error {
	args := cliCtx.Args()

	if len(args) == 0 {
		return errs.NewUsageExitError(cliCtx, fmt.Errorf("At least one key must be present"))
	}

	req := make(map[string]*string)
	for _, arg := range args {
		req[arg] = nil
	}
	return patchConfig(cliCtx, req)
}

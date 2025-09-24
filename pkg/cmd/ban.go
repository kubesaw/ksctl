package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/huh"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/banneduser"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	menuKey       string = "menu.json"
	configMapName string = "ban-reason-config"
)

// Menu contains all the fields present in the source JSON file needed to load up options in the interactive menu
type Menu struct {
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Options     []string `json:"options"`
}

// getValuesFromConfigMap retrieves the configMap contents to build the interactive menus
func getValuesFromConfigMap(ctx *clicontext.CommandContext) ([]Menu, error) {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return nil, err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{}
	err = cl.Get(context.TODO(), types.NamespacedName{
		Name:      configMapName,
		Namespace: cfg.OperatorNamespace,
	}, configMap)

	if err != nil {
		return nil, err
	}

	var menus []Menu
	if menuJSON, exists := configMap.Data[menuKey]; exists && menuJSON != "" {
		//var menus []Menu
		if err := json.Unmarshal([]byte(menuJSON), &menus); err != nil {
			return nil, fmt.Errorf("the %s key in the configmap doesn't contain a valid JSON format to render menus: %w", menuKey, err)
		}
	}

	return menus, nil
}

// BanMenu displays an interactive menu for selecting banning reasons
func BanMenu(ctx *clicontext.CommandContext, runForm runFormFunc, cfgMapContent []Menu) (map[string]string, error) {

	// Map to store user's answers
	answers := make(map[string]string)

	if len(cfgMapContent) > 0 {
		for _, item := range cfgMapContent {
			var choice string
			options := make([]huh.Option[string], len(item.Options))
			for i, opt := range item.Options {
				options[i] = huh.Option[string]{Key: opt, Value: opt}
			}

			form := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title(item.Description).
						Options(options...).
						Value(&choice),
				),
			)

			if err := runForm(form); err != nil {
				return nil, err //fmt.Errorf("failed to show interactive menu: %w", err)
			}

			answers[item.Kind] = choice

		}

		ctx.Printlnf("\nYour selection:\n")
		for kind, optionSelected := range answers {
			fmt.Printf("- %s:\t%s\n", kind, optionSelected)
		}

	}

	//return answers, nil
	return answers, nil

}

func NewBanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ban <usersignup-name>",
		Short: "Ban a user for the given UserSignup resource and reason of the ban",
		Long: `Ban the given UserSignup resource. The parameter is the name of the UserSignup to be banned.
The command will try to load banning reasons from a ConfigMap named 'ban-reason-config' in the toolchain-host-operator namespace and show 
an interactive menu for selection. If the ConfigMap doesn't exist, the ban reason must be provided.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)

			return Ban(ctx, func(form *huh.Form) error {
				if err := form.Run(); err != nil {
					return fmt.Errorf("failed to show interactive menu: %w", err)
				}
				return nil
			}, args...)
		},
	}
}

type runFormFunc func(form *huh.Form) error

func Ban(ctx *clicontext.CommandContext, runForm runFormFunc, args ...string) error {

	userSignupName := args[0]
	var banReason string

	// Interactive mode: usersignup name provided, need to get reason from ConfigMap menu
	ctx.Printlnf("Checking for available reasons from ConfigMap...")

	cfgMapContent, err := getValuesFromConfigMap(ctx)

	if err != nil || len(cfgMapContent) == 0 {
		if err != nil {
			ctx.Printlnf("failed to load reasons from ConfigMap %q: %s", configMapName, err)
		} else {
			ctx.Printlnf("the provided ConfigMap %q is empty", configMapName)
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter banning reason:").
					Prompt("> ").
					Value(&banReason),
			),
		)

		err := runForm(form)
		if err != nil {
			return fmt.Errorf("ban reason could not be obtained: %w", err)
		}
	} else {

		ctx.Printlnf("Opening interactive menu...")

		banInfo, err := BanMenu(ctx, runForm, cfgMapContent)
		if err != nil {
			return fmt.Errorf("failed to collect banning information: %w", err)
		}

		banInfoJSON, err := json.Marshal(banInfo)

		if err != nil {
			return fmt.Errorf("error marshaling ban reasons to JSON: %w", err)
		}

		banReason = string(banInfoJSON)
	}

	return CreateBannedUser(ctx, userSignupName, banReason, func(userSignup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
		if _, exists := bannedUser.Labels[toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey]; !exists {
			ctx.Printlnf("\nINFO: The UserSignup doesn't have the label '%s' set, so the resulting BannedUser resource won't have this label either.\n",
				toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey)
		}

		if err := ctx.PrintObject(bannedUser, "BannedUser resource to be created"); err != nil {
			return false, err
		}

		confirmation := ctx.AskForConfirmation(ioutils.WithDangerZoneMessagef(
			"deletion of all user's namespaces and all related data.\nIn addition, the user won't be able to login any more.",
			"ban the user with the UserSignup by creating BannedUser resource that are both above?"))
		return confirmation, nil
	})
}

func CreateBannedUser(ctx *clicontext.CommandContext, userSignupName, banReason string, confirm func(*toolchainv1alpha1.UserSignup, *toolchainv1alpha1.BannedUser) (bool, error)) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	userSignup, err := client.GetUserSignup(cl, cfg.OperatorNamespace, userSignupName)
	if err != nil {
		return err
	}

	ksctlConfig, err := configuration.Load(ctx)
	if err != nil {
		return err
	}

	bannedUser, err := banneduser.NewBannedUser(userSignup, ksctlConfig.Name, banReason)
	if err != nil {
		return err
	}

	alreadyBannedUser, err := banneduser.GetBannedUser(ctx, bannedUser.Labels[toolchainv1alpha1.BannedUserEmailHashLabelKey], cl, cfg.OperatorNamespace)
	if err != nil {
		return err
	}

	if err := ctx.PrintObject(userSignup, "UserSignup to be banned"); err != nil {
		return err
	}

	if alreadyBannedUser != nil {
		ctx.Println("The user was already banned - there is a BannedUser resource with the same labels already present")
		return ctx.PrintObject(alreadyBannedUser, "BannedUser resource")
	}

	if shouldCreate, err := confirm(userSignup, bannedUser); !shouldCreate || err != nil {
		return err
	}

	if err := cl.Create(context.TODO(), bannedUser); err != nil {
		return err
	}

	ctx.Printlnf("\nUserSignup has been banned by creating BannedUser resource with name " + bannedUser.Name)
	return nil
}

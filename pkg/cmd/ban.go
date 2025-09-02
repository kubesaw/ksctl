package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/banneduser"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	menuKey       string = "menu.json"
	configMapName string = "banning-reasons"
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
		Namespace: "toolchain-host-operator",
	}, configMap)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get ConfigMap: %w", err)
		}
		return nil, err
	}

	var menus []Menu
	if menuJSON, exists := configMap.Data[menuKey]; exists && menuJSON != "" {
		//var menus []Menu
		if err := json.Unmarshal([]byte(menuJSON), &menus); err != nil {
			return nil, fmt.Errorf("ConfigMap doesn't contain %s key: %w", menuKey, err)
		}
	}

	return menus, nil
}

// banMenu displays an interactive menu for selecting banning reasons
func banMenu(cfgMapContent []Menu) (*BanInfo, error) {
	banInfo := &BanInfo{}

	// Map to store user's answers
	answers := make(map[string]string)

	if len(cfgMapContent) > 0 {
		for _, item := range cfgMapContent {
			var choice string
			options := make([]huh.Option[string], len(item.Options))
			for i, opt := range item.Options {
				options[i] = huh.Option[string]{Key: opt, Value: opt}
			}

			form := huh.NewSelect[string]().
				Title(item.Description).
				Options(options...).
				Value(&choice)

			if err := form.Run(); err != nil {
				return nil, fmt.Errorf("failed to show interactive menu: %w", err)
			}

			answers[item.Kind] = choice

		}

		fmt.Printf("\nYour selection:\n")
		for kind, optionSelected := range answers {
			fmt.Printf("- %s:\t%s\n", kind, optionSelected)
		}

		// filling the banInfo object
		for kind, answer := range answers {
			switch kind {
			case "workload":
				banInfo.WorkloadType = answer
			case "behavior":
				banInfo.BehaviorClassification = answer
			case "detection":
				banInfo.DetectionMechanism = answer
			}
		}
	}

	return banInfo, nil

}

func NewBanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ban <usersignup-name> [ban-reason]",
		Short: "Ban a user for the given UserSignup resource and reason of the ban",
		Long: `Ban the given UserSignup resource. The first parameter is the name of the UserSignup to be banned.
The second parameter (ban reason) is optional. If not provided, the command will try to load 
banning reasons from a ConfigMap named 'banning-reasons' in the toolchain-host-operator namespace and show 
an interactive menu for selection. If the ConfigMap doesn't exist, the ban reason must be provided.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Ban(ctx, args...)
		},
	}
}

// BanInfo contains all the information needed for banning a user
type BanInfo struct {
	WorkloadType           string `json:"workloadType"`
	BehaviorClassification string `json:"behaviorClassification"`
	DetectionMechanism     string `json:"detectionMechanism"`
}

// FormatBanReason formats the ban information into a structured reason string
func (bi *BanInfo) FormatBanReason() string {
	parts := []string{}

	if bi.WorkloadType != "" {
		parts = append(parts, fmt.Sprintf("Workload Type: %s", bi.WorkloadType))
	}
	if bi.BehaviorClassification != "" {
		parts = append(parts, fmt.Sprintf("Behavior classification: %s", bi.BehaviorClassification))
	}
	if bi.DetectionMechanism != "" {
		parts = append(parts, fmt.Sprintf("Detection mechanism: %s", bi.DetectionMechanism))
	}

	return strings.Join(parts, ";")
}

func Ban(ctx *clicontext.CommandContext, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("UserSignup name is required")
	}

	userSignupName := args[0]
	var banReason string

	if len(args) == 2 {
		// Traditional usage: both usersignup name and ban reason provided
		banReason = args[1]
	} else {
		// Interactive mode: only usersignup name provided, need to get reason from ConfigMap menu
		ctx.Printlnf("No ban reason provided. Checking for available reasons from ConfigMap...")

		cfgMapContent, err := getValuesFromConfigMap(ctx)
		if err != nil {
			return fmt.Errorf("failed to load banning reasons from ConfigMap: %w", err)
		}

		if len(cfgMapContent) == 0 {
			return fmt.Errorf("no banning reasons found in ConfigMap 'banning-reasons' in toolchain-host-operator namespace. Please provide a ban reason as second argument or create the 'banning-reasons' ConfigMap with banning reasons in the toolchain-host-operator namespace")
		}

		ctx.Printlnf("Opening interactive menu...")

		banInfo, err := banMenu(cfgMapContent)
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

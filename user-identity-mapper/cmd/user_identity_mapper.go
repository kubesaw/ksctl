package main

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	userv1 "github.com/openshift/api/user/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateUserIdentityMappings(ctx context.Context, logger *log.Logger, cl runtimeclient.Client) error {
	logger.Info("listing users...")
	users := &userv1.UserList{}
	if err := cl.List(ctx, users, runtimeclient.MatchingLabels{
		"provider": "sandbox-sre",
	}); err != nil {
		return fmt.Errorf("unable to list users: %w", err)
	}
	for _, user := range users.Items {
		logger.Info("listing identities", "username", user.Name)
		identities := userv1.IdentityList{}
		if err := cl.List(ctx, &identities, runtimeclient.MatchingLabels{
			"provider": "sandbox-sre",
			"username": user.Name,
		}); err != nil {
			return fmt.Errorf("unable to list identities: %w", err)
		}
		if len(identities.Items) == 0 {
			logger.Errorf("no identity associated with user %q", user.Name)
			continue
		}
		for _, identity := range identities.Items {
			logger.Info("creating/updating identity mapping", "user", user.Name, "identity", identity.Name)
			if err := cl.Create(ctx, &userv1.UserIdentityMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name: identity.Name,
				},
				User: corev1.ObjectReference{
					Name: user.Name,
				},
				Identity: corev1.ObjectReference{
					Name: identity.Name,
				},
			}); err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("unable to create identity mapping for username %q and identity %q: %w", user.Name, identity.Name, err)
			}
		}
	}
	return nil
}

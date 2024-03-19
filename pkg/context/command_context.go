package context

import (
	"context"

	"github.com/kubesaw/ksctl/pkg/ioutils"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// CommandContext the context of the (standard) command to run
type CommandContext struct {
	context.Context
	ioutils.Terminal
	NewClient NewClientFunc
}

// NewClientFunc a function to create a `client.Client` with the given token and API endpoint
type NewClientFunc func(string, string) (runtimeclient.Client, error)

// NewCommandContext returns the context of the command to run
func NewCommandContext(term ioutils.Terminal, newClient NewClientFunc) *CommandContext {
	return &CommandContext{
		Context:   context.Background(),
		Terminal:  term,
		NewClient: newClient,
	}
}

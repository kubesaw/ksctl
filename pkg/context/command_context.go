package context

import (
	"context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// CommandContext the context of the (standard) command to run
type CommandContext struct {
	context.Context
	ioutils.Terminal
	NewClient     NewClientFunc
	NewRESTClient NewRESTClientFunc
}

// NewClientFunc a function to create a `client.Client` with the given token and API endpoint
type NewClientFunc func(string, string) (runtimeclient.Client, error)

type NewRESTClientFunc func(token, apiEndpoint string) (*rest.RESTClient, error)

// NewCommandContext returns the context of the command to run
func NewCommandContext(term ioutils.Terminal, newClient NewClientFunc, newRESTClient NewRESTClientFunc) *CommandContext {
	return &CommandContext{
		Context:       context.Background(),
		Terminal:      term,
		NewClient:     newClient,
		NewRESTClient: newRESTClient,
	}
}

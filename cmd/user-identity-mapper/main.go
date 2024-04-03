// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	// cmd the command that maps an identity to its parent user
	cmd := &cobra.Command{
		Use: "user-identity-mapper",
		RunE: func(cmd *cobra.Command, args []string) error {

			logger := log.New(cmd.OutOrStderr())
			// Get a config to talk to the apiserver
			cfg, err := config.GetConfig()
			if err != nil {
				logger.Error("unable to load config", "error", err)
				os.Exit(1)
			}

			// create client that will be used for retrieving the host operator secret & ToolchainCluster CRs
			scheme := runtime.NewScheme()
			if err := userv1.Install(scheme); err != nil {
				logger.Error("unable to install scheme", "error", err)
				os.Exit(1)
			}
			cl, err := runtimeclient.New(cfg, runtimeclient.Options{
				Scheme: scheme,
			})
			if err != nil {
				logger.Error("unable to create a client", "error", err)
				os.Exit(1)
			}
			return CreateUserIdentityMappings(cmd.Context(), logger, cl)
		},
	}

	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

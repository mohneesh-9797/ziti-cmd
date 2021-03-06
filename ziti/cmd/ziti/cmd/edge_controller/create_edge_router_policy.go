/*
	Copyright NetFoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package edge_controller

import (
	"fmt"
	"io"

	"github.com/Jeffail/gabs"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type createEdgeRouterPolicyOptions struct {
	commonOptions
	edgeRouterRoles []string
	identityRoles   []string
}

// newCreateEdgeRouterPolicyCmd creates the 'edge controller create edge-router-policy' command
func newCreateEdgeRouterPolicyCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createEdgeRouterPolicyOptions{
		commonOptions: commonOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "edge-router-policy <name>",
		Short: "creates an edge-router-policy managed by the Ziti Edge Controller",
		Long:  "creates an edge-router-policy managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateEdgeRouterPolicy(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringSliceVarP(&options.edgeRouterRoles, "edge-router-roles", "r", nil, "Edge router roles of the new edge router policy")
	cmd.Flags().StringSliceVarP(&options.identityRoles, "identity-roles", "i", nil, "Identity roles of the new edge router policy")
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")

	return cmd
}

// runCreateEdgeRouterPolicy create a new edgeRouterPolicy on the Ziti Edge Controller
func runCreateEdgeRouterPolicy(o *createEdgeRouterPolicyOptions) error {

	entityData := gabs.New()
	setJSONValue(entityData, o.Args[0], "name")
	setJSONValue(entityData, o.edgeRouterRoles, "edgeRouterRoles")
	setJSONValue(entityData, o.identityRoles, "identityRoles")
	result, err := createEntityOfType("edge-router-policies", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	edgeRouterPolicyId := result.S("data", "id").Data()

	if _, err := fmt.Fprintf(o.Out, "%v\n", edgeRouterPolicyId); err != nil {
		panic(err)
	}

	return err
}

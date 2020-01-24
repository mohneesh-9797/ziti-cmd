/*
	Copyright 2019 NetFoundry, Inc.

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

package subcmd

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	listServicesClient = NewMgmtClient(listServices)
	listCmd.AddCommand(listServices)
}

var listServices = &cobra.Command{
	Use:   "services",
	Short: "Retrieve all service definitions",
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := listServicesClient.Connect(); err == nil {
			request := &mgmt_pb.ListServicesRequest{}
			body, err := proto.Marshal(request)
			if err != nil {
				panic(err)
			}
			requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListServicesRequestType), body)
			waitCh, err := ch.SendAndWait(requestMsg)
			if err != nil {
				panic(err)
			}
			select {
			case responseMsg := <-waitCh:
				if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListServicesResponseType) {
					response := &mgmt_pb.ListServicesResponse{}
					if err := proto.Unmarshal(responseMsg.Body, response); err == nil {
						out := fmt.Sprintf("\nServices: (%d)\n\n", len(response.Services))
						out += fmt.Sprintf("%-12s | %-12s | %s\n", "Id", "Binding", "Destination")
						for _, svc := range response.Services {
							out += fmt.Sprintf("%-12s | %-12s | %s\n", svc.Id, svc.Binding, fmt.Sprintf("%-12s -> %s", svc.Egress, svc.EndpointAddress))
						}
						out += "\n"
						fmt.Print(out)
					} else {
						panic(err)
					}
				} else {
					panic(errors.New("unexpected response"))
				}

			case <-time.After(5 * time.Second):
				panic(errors.New("timeout"))
			}

		} else {
			panic(err)
		}
	},
}
var listServicesClient *mgmtClient

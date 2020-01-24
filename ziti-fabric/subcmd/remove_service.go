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
	removeServiceClient = NewMgmtClient(removeService)
	removeCmd.AddCommand(removeService)
}

var removeService = &cobra.Command{
	Use:   "service <serviceId>",
	Short: "Remove a service from the fabric",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if ch, err := removeServiceClient.Connect(); err == nil {
			request := &mgmt_pb.RemoveServiceRequest{
				ServiceId: args[0],
			}
			body, err := proto.Marshal(request)
			if err != nil {
				panic(err)
			}
			requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_RemoveServiceRequestType), body)
			waitCh, err := ch.SendAndWait(requestMsg)
			if err != nil {
				panic(err)
			}
			select {
			case responseMsg := <-waitCh:
				if responseMsg.ContentType == channel2.ContentTypeResultType {
					result := channel2.UnmarshalResult(responseMsg)
					if result.Success {
						fmt.Printf("\nsuccess\n\n")
					} else {
						fmt.Printf("\nfailure [%s]\n\n", result.Message)
					}
				} else {
					panic(fmt.Errorf("unexpected response type %v", responseMsg.ContentType))
				}
			case <-time.After(5 * time.Second):
				panic(errors.New("timeout"))
			}
		}
	},
}
var removeServiceClient *mgmtClient

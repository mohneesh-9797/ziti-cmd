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

package subcmd

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/mux"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"net/http"
	"time"
)

func handleGetService(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	request := &mgmt_pb.GetServiceRequest{ServiceId: id}
	body, err := proto.Marshal(request)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(err.Error()); err != nil {
			pfxlog.Logger().Errorf("error encoding json (%s)", err)
		}
		return
	}

	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_GetServiceRequestType), body)
	waitCh, err := mgmtCh.SendAndWait(requestMsg)
	if err == nil {
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_GetServiceResponseType) {
				response := &mgmt_pb.GetServiceResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err == nil {
					if err := json.NewEncoder(w).Encode(convertService(response.Service)); err != nil {
						pfxlog.Logger().Errorf("error encoding json (%s)", err)
					}

				} else {
					w.WriteHeader(http.StatusBadRequest)
					if err := json.NewEncoder(w).Encode(err.Error()); err != nil {
						pfxlog.Logger().Errorf("error encoding json (%s)", err)
					}
				}
			} else if responseMsg.ContentType == channel2.ContentTypeResultType {
				result := channel2.UnmarshalResult(responseMsg)
				if result.Success {
					// This should never happen (i.e. success should take the form of a GetServiceResponseType)
					w.WriteHeader(http.StatusInternalServerError)
					if err := json.NewEncoder(w).Encode("unexpected response"); err != nil {
						pfxlog.Logger().Errorf("error encoding json (%s)", err)
					}
				} else {
					w.WriteHeader(http.StatusNotFound)
					if err := json.NewEncoder(w).Encode("invalid ID specified"); err != nil {
						pfxlog.Logger().Errorf("error encoding json (%s)", err)
					}
				}
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				if err := json.NewEncoder(w).Encode("unexpected response"); err != nil {
					pfxlog.Logger().Errorf("error encoding json (%s)", err)
				}
			}

		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode("timeout"); err != nil {
				pfxlog.Logger().Errorf("error encoding json (%s)", err)
			}
		}
	} else {
		pfxlog.Logger().Errorf("unexpected error (%s)", err)
	}
}

func convertService(svc *mgmt_pb.Service) interface{} {
	d := &data{Data: make([]interface{}, 0)}
	address := ""
	router := ""
	if len(svc.Terminators) > 0 {
		address = svc.Terminators[0].Address
		router = svc.Terminators[0].RouterId
	}
	d.Data = append(d.Data, &service{
		Id:              svc.Id,
		EndpointAddress: address,
		Egress:          router,
	})
	return d
}

/*
 *  Copyright (c) Huawei Technologies Co., Ltd. 2020-2024. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

// Package rpc provides common rpc functions
package rpc

import (
	"context"
	"errors"

	"google.golang.org/grpc"

	"huawei-csi-driver/lib/drcsi"
)

// GetProviderName returns name of DR-CSI driver.
func GetProviderName(ctx context.Context, conn *grpc.ClientConn) (string, error) {
	client := drcsi.NewIdentityClient(conn)

	req := drcsi.GetProviderInfoRequest{}
	rsp, err := client.GetProviderInfo(ctx, &req)
	if err != nil {
		return "", err
	}
	name := rsp.GetProvider()
	if name == "" {
		return "", errors.New("drcsi name is empty")
	}
	return name, nil
}

// PluginCapabilitySet is set of DR-CSI plugin capabilities. Only supported capabilities are in the map.
type PluginCapabilitySet map[drcsi.ProviderCapability_Service_Type]bool

// GetPluginCapabilities returns set of supported capabilities of DR-CSI driver.
func GetPluginCapabilities(ctx context.Context, conn *grpc.ClientConn) (PluginCapabilitySet, error) {
	client := drcsi.NewIdentityClient(conn)
	req := drcsi.GetProviderCapabilitiesRequest{}
	rsp, err := client.GetProviderCapabilities(ctx, &req)
	if err != nil {
		return nil, err
	}
	caps := PluginCapabilitySet{}
	for _, capability := range rsp.GetCapabilities() {
		if capability == nil {
			continue
		}
		srv := capability.GetService()
		if srv == nil {
			continue
		}
		t := srv.GetType()
		caps[t] = true
	}
	return caps, nil
}

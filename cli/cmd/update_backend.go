/*
 *  Copyright (c) Huawei Technologies Co., Ltd. 2023-2023. All rights reserved.
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

package command

import (
	"github.com/spf13/cobra"

	"huawei-csi-driver/cli/client"
	"huawei-csi-driver/cli/cmd/options"
	"huawei-csi-driver/cli/config"
	"huawei-csi-driver/cli/helper"
	"huawei-csi-driver/cli/resources"
)

func init() {
	options.NewFlagsOptions(updateBackendCmd).
		WithNameSpace(false).
		WithPassword(true).
		WithParent(updateCmd)
}

var (
	updateBackendExample = helper.Examples(`
		# Update backend account information in default(huawei-csi) namespace
		oceanctl update backend <name>  --password

	    # Update backend account information in specified namespace
		oceanctl update backend <name> -n namespace --password`)
)

var updateBackendCmd = &cobra.Command{
	Use:     "backend <name>",
	Short:   "Update a backend for Ocean Storage in Kubernetes",
	Example: updateBackendExample,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpdateBackend(args)
	},
}

func runUpdateBackend(backendNames []string) error {
	res := resources.NewResourceBuilder().
		ResourceNames(string(client.Storagebackendclaim), backendNames...).
		NamespaceParam(config.Namespace).
		DefaultNamespace().
		Build()

	validator := resources.NewValidatorBuilder(res).ValidateNameIsExist().ValidateNameIsSingle().Build()
	if err := validator.Validate(); err != nil {
		return helper.PrintlnError(err)
	}

	return resources.NewBackend(res).Update()
}

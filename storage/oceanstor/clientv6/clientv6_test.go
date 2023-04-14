/*
 *  Copyright (c) Huawei Technologies Co., Ltd. 2022-2022. All rights reserved.
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

package clientv6

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"huawei-csi-driver/storage/oceanstor/client"
	"huawei-csi-driver/utils/log"
)

var testClient *ClientV6

const (
	logName = "clientV6_test.log"

	successStatus int = 200
)

func TestSplitCloneFS(t *testing.T) {
	var cases = []struct {
		name         string
		responseBody string
		wantErr      bool
	}{
		{
			"Normal",
			`{"data": {}, "error": {"code": 0, "description": "0"}}`,
			false,
		},
		{
			"SplitCloneFailed",
			`{"data": {}, "error": {"code": 1, "description": "failed case"}}`,
			true,
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBaseClient := client.NewMockHTTPClient(ctrl)

	temp := testClient.BaseClient.Client
	defer func() { testClient.BaseClient.Client = temp }()
	testClient.BaseClient.Client = mockBaseClient

	for _, c := range cases {
		r := ioutil.NopCloser(bytes.NewReader([]byte(c.responseBody)))
		mockBaseClient.EXPECT().Do(gomock.Any()).AnyTimes().Return(&http.Response{
			StatusCode: successStatus,
			Body:       r,
		}, nil)

		err := testClient.SplitCloneFS(context.TODO(), "test", "0", 2, true)
		assert.Equal(t, c.wantErr, err != nil, "Test SplitCloneFSV6 failed, error: %v", err)
	}
}

func TestMain(m *testing.M) {
	log.MockInitLogging(logName)
	defer log.MockStopLogging(logName)

	testClient = NewClientV6([]string{"https://192.168.125.*:8088"},
		"dev-account",
		"mock-sec-name",
		"mock-sec-namespace",
		"dev-vStore",
		"",
		"mock-backend-id")

	m.Run()
}

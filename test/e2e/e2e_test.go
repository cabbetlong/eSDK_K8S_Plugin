/*
 *  Copyright (c) Huawei Technologies Co., Ltd. 2025-2025. All rights reserved.
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

// Package e2e contains hermetic integration tests that exercise the real CSI
// gRPC server, the real backend sync job and a real HTTP client against an
// in-memory fake Huawei storage array. The Kubernetes side uses the standard
// client-go fake clientset, so no test cluster or external binaries are needed.
package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	xuanwuv1 "github.com/Huawei/eSDK_K8S_Plugin/v4/client/apis/xuanwu/v1"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/app"
	cfg "github.com/Huawei/eSDK_K8S_Plugin/v4/csi/app/config"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/backend/cache"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/backend/handler"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/driver"
	versioned "github.com/Huawei/eSDK_K8S_Plugin/v4/pkg/client/clientset/versioned"
	clientfake "github.com/Huawei/eSDK_K8S_Plugin/v4/pkg/client/clientset/versioned/fake"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/pkg/constants"
	storagefake "github.com/Huawei/eSDK_K8S_Plugin/v4/test/fake/storage"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/utils/k8sutils"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/utils/log"
)

const (
	testNamespace  = "huawei-csi"
	backendName    = "e2e-san-backend"
	poolName       = "StoragePool001"
	configMapName  = "backend-config"
	secretName     = "backend-secret"
	driverNodeName = "node-e2e"
	createdVolName = "e2e-san-volume"
)

func TestMain(m *testing.M) {
	log.MockInitLogging("test")
	code := m.Run()
	log.MockStopLogging("test")
	os.Exit(code)
}

// e2eK8s embeds the k8sutils client but stubs GetVolumeConfiguration so
// CreateVolume does not require a real PVC/PV (tests create volumes directly
// through the CSI RPC like the existing fast integration layer does).
type e2eK8s struct {
	k8sutils.Interface
}

func (e *e2eK8s) GetVolumeConfiguration(_ context.Context, _ string) (map[string]string, error) {
	return map[string]string{}, nil
}

func TestE2E_OceanStorSan_CreateDeleteVolume(t *testing.T) {
	fakeSAN := storagefake.NewOceanStorSAN(poolName)
	defer fakeSAN.Close()
	fakeURL := fakeSAN.Start()

	kubeClient := k8sfake.NewSimpleClientset()
	backendClient := clientfake.NewSimpleClientset()

	// wire the global config like a real controller/CSI process would
	k8sClient := &k8sutils.KubeClient{}
	k8sClient.SetClient(kubeClient)

	globalCfg := cfg.MockCompletedConfig()
	globalCfg.AppConfig.DriverName = constants.DefaultDriverName
	globalCfg.AppConfig.Namespace = testNamespace
	globalCfg.AppConfig.NodeName = driverNodeName
	globalCfg.K8sUtils = &e2eK8s{Interface: k8sClient}
	globalCfg.BackendUtils = backendClient

	origGetGlobalConfig := app.GetGlobalConfig
	app.GetGlobalConfig = func() *cfg.CompletedConfig { return globalCfg }
	defer func() { app.GetGlobalConfig = origGetGlobalConfig }()

	ctx := context.Background()
	createFixture(t, ctx, kubeClient, backendClient, fakeURL)

	// real sync job: fetch StorageBackendContents and BuildBackend against the fake
	cache.BackendCacheProvider.Clear(ctx)
	handler.NewBackendRegister().FetchAndRegisterAllBackend(ctx)
	require.Equal(t, 1, cache.BackendCacheProvider.Count(), "backend must be built into the cache")

	// start a real CSI gRPC server
	driverSrv := driver.NewServer(constants.DefaultDriverName, constants.ProviderVersion, globalCfg.K8sUtils, driverNodeName)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	csi.RegisterIdentityServer(grpcServer, driverSrv)
	csi.RegisterControllerServer(grpcServer, driverSrv)
	csi.RegisterNodeServer(grpcServer, driverSrv)
	go func() { _ = grpcServer.Serve(listener) }()
	defer grpcServer.Stop()

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	controller := csi.NewControllerClient(conn)

	// --- CreateVolume ---
	createResp, err := controller.CreateVolume(ctx, createVolumeRequest())
	require.NoError(t, err)
	require.NotNil(t, createResp.GetVolume())
	require.Equal(t, backendName+"."+createdVolName, createResp.GetVolume().GetVolumeId())
	require.Equal(t, 1, fakeSAN.LunCount(), "exactly one LUN should exist on the fake array")

	// --- DeleteVolume ---
	delResp, err := controller.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: createResp.GetVolume().GetVolumeId()})
	require.NoError(t, err)
	require.NotNil(t, delResp)
	require.Eventually(t, func() bool { return fakeSAN.LunCount() == 0 }, 5*time.Second, 100*time.Millisecond,
		"LUN should be removed from the fake array after DeleteVolume")
}

func createVolumeRequest() *csi.CreateVolumeRequest {
	return &csi.CreateVolumeRequest{
		Name: createdVolName,
		Parameters: map[string]string{
			"volumeType":   "lun",
			"allocType":    "thin",
			"protocol":     "iscsi",
			"fsPermission": "777",
		},
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{FsType: ""},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1024 * 1024 * 1024},
	}
}

func createFixture(t *testing.T, ctx context.Context, kubeClient kubernetes.Interface,
	backendUtils versioned.Interface, fakeURL string) {
	// namespace
	_, err := kubeClient.CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, metav1.CreateOptions{})
	require.NoError(t, err)

	// configmap holding the backend config under key "csi.json"
	configJSON := fmt.Sprintf(`{
		"backends": {
			"storage": %q,
			"name": %q,
			"urls": [%q],
			"parameters": {"protocol": "iscsi", "portals": ["192.168.1.10"]},
			"pools": [%q],
			"maxClientThreads": "10"
		}
	}`, constants.OceanStorSan, backendName, fakeURL, poolName)
	_, err = kubeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: configMapName, Namespace: testNamespace},
			Data:       map[string]string{"csi.json": configJSON},
		}, metav1.CreateOptions{})
	require.NoError(t, err)

	// secret with credentials
	_, err = kubeClient.CoreV1().Secrets(testNamespace).Create(ctx,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
			Data: map[string][]byte{
				"user":     []byte("fake-user"),
				"password": []byte("fake-password"),
			},
		}, metav1.CreateOptions{})
	require.NoError(t, err)

	// StorageBackendClaim (needed for Login to resolve secret meta by backend id)
	_, err = backendUtils.XuanwuV1().StorageBackendClaims(testNamespace).Create(ctx, &xuanwuv1.StorageBackendClaim{
		ObjectMeta: metav1.ObjectMeta{Name: backendName, Namespace: testNamespace},
		Spec: xuanwuv1.StorageBackendClaimSpec{
			Provider:         constants.DefaultDriverName,
			ConfigMapMeta:    testNamespace + "/" + configMapName,
			SecretMeta:       testNamespace + "/" + secretName,
			MaxClientThreads: "10",
		},
		Status: &xuanwuv1.StorageBackendClaimStatus{Phase: xuanwuv1.BackendBound},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// StorageBackendContent (cluster-scoped; created directly - Claim->Content is Phase 2.1)
	createdContent, err := backendUtils.XuanwuV1().StorageBackendContents().Create(ctx, &xuanwuv1.StorageBackendContent{
		ObjectMeta: metav1.ObjectMeta{Name: backendName + "-content"},
		Spec: xuanwuv1.StorageBackendContentSpec{
			Provider:         constants.DefaultDriverName,
			ConfigmapMeta:    testNamespace + "/" + configMapName,
			SecretMeta:       testNamespace + "/" + secretName,
			BackendClaim:     testNamespace + "/" + backendName,
			MaxClientThreads: "10",
		},
		Status: &xuanwuv1.StorageBackendContentStatus{
			ContentName:      backendName + "@" + poolName,
			Online:           true,
			SN:               "fake-sn",
			MaxClientThreads: "10",
			Capabilities: map[string]bool{
				"SupportThin": true,
			},
			Pools: []xuanwuv1.Pool{
				{
					Name: poolName,
					Capacities: map[string]string{
						"FreeCapacity":  "1048576",
						"TotalCapacity": "1048576",
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// fake clients keep the status set on Create; the explicit UpdateStatus call is
	// kept for parity with a real API server's status subresource behavior.
	_, err = backendUtils.XuanwuV1().StorageBackendContents().UpdateStatus(ctx, createdContent, metav1.UpdateOptions{})
	require.NoError(t, err)
}

//go:build e2e

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

// Package e2e contains opt-in end-to-end tests. Files here build with the
// `e2e` tag and are excluded from the default `go test ./...`.
package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	xuanwuv1 "github.com/Huawei/eSDK_K8S_Plugin/v4/client/apis/xuanwu/v1"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/app"
	cfg "github.com/Huawei/eSDK_K8S_Plugin/v4/csi/app/config"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/backend/cache"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/backend/handler"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/driver"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/pkg/client/clientset/versioned"
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

// e2eK8s embeds the real k8sutils client but stubs GetVolumeConfiguration so
// CreateVolume does not require a real PVC/PV in the envtest cluster.
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

	env := &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{filepath.Join("..", "..", "helm", "esdk", "crds", "backend")},
		},
		ErrorIfCRDPathMissing: true,
	}
	restCfg, err := env.Start()
	if err != nil {
		t.Skipf("envtest not available (run `make setup-envtest`): %v", err)
	}
	defer func() { _ = env.Stop() }()

	kubeconfigPath := writeKubeconfig(t, env.KubeConfig, restCfg.Host)

	kubeClient, err := kubernetes.NewForConfig(restCfg)
	require.NoError(t, err)

	k8s, err := k8sutils.NewK8SUtils(kubeconfigPath, k8sutils.WithVolumeNamePrefix(""))
	require.NoError(t, err)
	backendUtils, err := k8sutils.NewBackendUtils(kubeconfigPath)
	require.NoError(t, err)

	// wire the global config like a real controller process would
	globalCfg := cfg.MockCompletedConfig()
	globalCfg.AppConfig.DriverName = constants.DefaultDriverName
	globalCfg.AppConfig.Namespace = testNamespace
	globalCfg.AppConfig.NodeName = driverNodeName
	globalCfg.AppConfig.KubeConfig = kubeconfigPath
	globalCfg.K8sUtils = &e2eK8s{Interface: k8s}
	globalCfg.BackendUtils = backendUtils

	origGetGlobalConfig := app.GetGlobalConfig
	app.GetGlobalConfig = func() *cfg.CompletedConfig { return globalCfg }
	defer func() { app.GetGlobalConfig = origGetGlobalConfig }()

	ctx := context.Background()
	createFixture(t, ctx, kubeClient, backendUtils, fakeURL)

	// real sync job: fetch StorageBackendContents and BuildBackend against the fake
	cache.BackendCacheProvider.Clear(ctx)
	handler.NewBackendRegister().FetchAndRegisterAllBackend(ctx)
	require.Equal(t, 1, cache.BackendCacheProvider.Count(), "backend must be built into the cache")

	// start the real CSI gRPC server
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
	claimClient := backendUtils.XuanwuV1().StorageBackendClaims(testNamespace)
	_, err = claimClient.Create(ctx, &xuanwuv1.StorageBackendClaim{
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

	// StorageBackendContent (directly created; adr: Claim->Content is Phase 2.1)
	contentClient := backendUtils.XuanwuV1().StorageBackendContents()
	createdContent, err := contentClient.Create(ctx, &xuanwuv1.StorageBackendContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: backendName + "-content",
		},
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

	// status is a subresource: persist it explicitly so the sync job sees Online=true
	// and non-empty capabilities.
	createdContent.Status = &xuanwuv1.StorageBackendContentStatus{
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
	}
	_, err = contentClient.UpdateStatus(ctx, createdContent, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func writeKubeconfig(t *testing.T, data []byte, host string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if len(data) > 0 {
		require.NoError(t, os.WriteFile(path, data, 0o600))
		return path
	}
	// envtest does not always expose KubeConfig; build one from the rest config.
	kubeConfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: envtest
  cluster:
    server: %s
contexts:
- name: envtest
  context:
    cluster: envtest
    user: envtest
current-context: envtest
users:
- name: envtest
  user: {}
`, host)
	require.NoError(t, os.WriteFile(path, []byte(kubeConfig), 0o600))
	return path
}

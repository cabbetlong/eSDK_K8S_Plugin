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

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	xuanwuv1 "github.com/Huawei/eSDK_K8S_Plugin/v4/client/apis/xuanwu/v1"
	connutils "github.com/Huawei/eSDK_K8S_Plugin/v4/connector/utils"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/app"
	cfg "github.com/Huawei/eSDK_K8S_Plugin/v4/csi/app/config"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/csi/driver"
	versioned "github.com/Huawei/eSDK_K8S_Plugin/v4/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	clientfake "github.com/Huawei/eSDK_K8S_Plugin/v4/pkg/client/clientset/versioned/fake"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/pkg/constants"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/utils"
	"github.com/Huawei/eSDK_K8S_Plugin/v4/utils/k8sutils"
)

const (
	nasBackendName = "e2e-nas-backend"
	nasVolumeName  = "e2e-nas-volume"
	nasVolID       = nasBackendName + "." + nasVolumeName
)

// nodeEnv bundles the hermetic node-side test environment.
type nodeEnv struct {
	node     csi.NodeClient
	tempDirs []string
	cleanup  func()
}

func TestNode_GetCapabilitiesAndInfoAndVolumeStats(t *testing.T) {
	env := setupNodeEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	// The hostname lookup shells out to nsenter which needs privileges in CI/WSL;
	// stub it via the replaceable-var seam.
	origGetHostName := utils.GetHostNameFunc
	utils.GetHostNameFunc = func(context.Context) (string, error) { return "e2e-node", nil }
	defer func() { utils.GetHostNameFunc = origGetHostName }()

	// NodeGetCapabilities
	caps, err := env.node.NodeGetCapabilities(ctx, &csi.NodeGetCapabilitiesRequest{})
	require.NoError(t, err)
	require.Len(t, caps.GetCapabilities(), 3, "stage/unstage, expand and stats capabilities expected")

	// NodeGetInfo
	info, err := env.node.NodeGetInfo(ctx, &csi.NodeGetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info.GetNodeId())
	require.Equal(t, int64(256), info.GetMaxVolumesPerNode())
	require.NotNil(t, info.GetAccessibleTopology())
	require.Equal(t, "fake-zone", info.GetAccessibleTopology().GetSegments()["topology.kubernetes.io/zone"])

	// NodeGetVolumeStats against a real temp directory (real statfs, no root needed)
	dir := t.TempDir()
	stats, err := env.node.NodeGetVolumeStats(ctx, &csi.NodeGetVolumeStatsRequest{
		VolumeId:   nasVolID,
		VolumePath: dir,
	})
	require.NoError(t, err)
	require.Len(t, stats.GetUsage(), 2, "filesystem stats should return bytes + inodes usage")
}

func TestNode_NfsStagePublishUnpublishUnstage(t *testing.T) {
	env := setupNodeEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	// Replace mount/umount with hermetic stand-ins (replaceable-var seam).
	origMount, origUnmount := connutils.MountToDirFunc, connutils.UnmountFunc
	connutils.MountToDirFunc = func(_ context.Context, _ string, targetPath string, _ connutils.MountParam, _ bool) error {
		return os.MkdirAll(targetPath, 0o755)
	}
	connutils.UnmountFunc = func(_ context.Context, _ string) error { return nil }
	defer func() {
		connutils.MountToDirFunc, connutils.UnmountFunc = origMount, origUnmount
	}()

	staging := filepath.Join(t.TempDir(), "stage")
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(staging, 0o755))
	require.NoError(t, os.MkdirAll(target, 0o755))

	mountCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}

	// NodeStageVolume
	_, err := env.node.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
		VolumeId:          nasVolID,
		StagingTargetPath: staging,
		VolumeCapability:  mountCap,
	})
	require.NoError(t, err)
	require.DirExists(t, staging)

	// NodePublishVolume
	_, err = env.node.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
		VolumeId:          nasVolID,
		StagingTargetPath: staging,
		TargetPath:        target,
		VolumeCapability:  mountCap,
	})
	require.NoError(t, err)
	require.DirExists(t, target)

	// NodeUnpublishVolume
	_, err = env.node.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
		VolumeId:   nasVolID,
		TargetPath: target,
	})
	require.NoError(t, err)
	_, statErr := os.Lstat(target)
	require.ErrorIs(t, statErr, os.ErrNotExist, "target path should be removed after unpublish")

	// NodeUnstageVolume
	_, err = env.node.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
		VolumeId:          nasVolID,
		StagingTargetPath: staging,
	})
	require.NoError(t, err)
	_, statErr = os.Lstat(staging)
	require.ErrorIs(t, statErr, os.ErrNotExist, "staging path should be removed after unstage")
}

func setupNodeEnv(t *testing.T) *nodeEnv {
	t.Helper()

	kubeClient := k8sfake.NewSimpleClientset()
	backendClient := clientfake.NewSimpleClientset()

	k8sClient := &k8sutils.KubeClient{}
	k8sClient.SetClient(kubeClient)

	globalCfg := cfg.MockCompletedConfig()
	globalCfg.AppConfig.DriverName = constants.DefaultDriverName
	globalCfg.AppConfig.Namespace = testNamespace
	globalCfg.AppConfig.NodeName = driverNodeName
	globalCfg.AppConfig.MaxVolumesPerNode = 256
	globalCfg.K8sUtils = &e2eK8s{Interface: k8sClient}
	globalCfg.BackendUtils = backendClient

	origGetGlobalConfig := app.GetGlobalConfig
	app.GetGlobalConfig = func() *cfg.CompletedConfig { return globalCfg }

	ctx := context.Background()
	createNfsFixture(t, ctx, kubeClient, backendClient)

	driverSrv := driver.NewServer(constants.DefaultDriverName, constants.ProviderVersion, globalCfg.K8sUtils, driverNodeName)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	csi.RegisterIdentityServer(grpcServer, driverSrv)
	csi.RegisterControllerServer(grpcServer, driverSrv)
	csi.RegisterNodeServer(grpcServer, driverSrv)
	go func() { _ = grpcServer.Serve(listener) }()

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	return &nodeEnv{
		node: csi.NewNodeClient(conn),
		cleanup: func() {
			_ = conn.Close()
			grpcServer.Stop()
			app.GetGlobalConfig = origGetGlobalConfig
		},
	}
}

// createNfsFixture creates the Claim + ConfigMap that the node side reads to
// build the NFS backend config. No storage REST integration is needed here.
func createNfsFixture(t *testing.T, ctx context.Context, kubeClient kubernetes.Interface,
	backendUtils versioned.Interface) {
	t.Helper()

	_, err := kubeClient.CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, metav1.CreateOptions{})
	require.NoError(t, err)

	configJSON := fmt.Sprintf(`{
		"backends": {
			"storage": %q,
			"name": %q,
			"urls": ["http://127.0.0.1:1"],
			"parameters": {"protocol": "nfs", "portals": ["192.168.1.10"]},
			"maxClientThreads": "10"
		}
	}`, constants.OceanStorNas, nasBackendName)

	_, err = kubeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: configMapName + "-nas", Namespace: testNamespace},
			Data:       map[string]string{"csi.json": configJSON},
		}, metav1.CreateOptions{})
	require.NoError(t, err)

	// The claim carries the configmap meta; the node side resolves the backend
	// config from it through the K8s ConfigMap (no storage REST needed).
	_, err = backendUtils.XuanwuV1().StorageBackendClaims(testNamespace).Create(ctx, &xuanwuv1.StorageBackendClaim{
		ObjectMeta: metav1.ObjectMeta{Name: nasBackendName, Namespace: testNamespace},
		Spec: xuanwuv1.StorageBackendClaimSpec{
			Provider:      constants.DefaultDriverName,
			ConfigMapMeta: testNamespace + "/" + configMapName + "-nas",
		},
		Status: &xuanwuv1.StorageBackendClaimStatus{Phase: xuanwuv1.BackendBound},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
}

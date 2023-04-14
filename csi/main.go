/*
 *  Copyright (c) Huawei Technologies Co., Ltd. 2020-2022. All rights reserved.
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"huawei-csi-driver/connector"
	"huawei-csi-driver/connector/host"
	connutils "huawei-csi-driver/connector/utils"
	"huawei-csi-driver/connector/utils/lock"
	"huawei-csi-driver/csi/app"
	"huawei-csi-driver/csi/backend"
	"huawei-csi-driver/csi/driver"
	"huawei-csi-driver/csi/provider"
	"huawei-csi-driver/lib/drcsi"
	"huawei-csi-driver/utils"
	"huawei-csi-driver/utils/log"
	"huawei-csi-driver/utils/notify"
	"huawei-csi-driver/utils/version"
)

const (
	configFile        = "/etc/huawei/csi.json"
	secretFile        = "/etc/huawei/secret/secret.json"
	versionFile       = "/csi/version"
	controllerLogFile = "huawei-csi-controller"
	nodeLogFile       = "huawei-csi-node"

	csiVersion      = "4.0.0"
	endpointDirPerm = 0755
)

var (
	config CSIConfig
	secret CSISecret
)

type CSIConfig struct {
	Backends []map[string]interface{} `json:"backends"`
}

type CSISecret struct {
	Secrets map[string]interface{} `json:"secrets"`
}

func parseConfig() {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		notify.Stop("Read config file %s error: %v", configFile, err)
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		notify.Stop("Unmarshal config file %s error: %v", configFile, err)
	}

	if len(config.Backends) <= 0 {
		notify.Stop("Must configure at least one backend")
	}

	secretData, err := ioutil.ReadFile(secretFile)
	if err != nil {
		notify.Stop("Read config file %s error: %v", secretFile, err)
	}

	err = json.Unmarshal(secretData, &secret)
	if err != nil {
		notify.Stop("Unmarshal config file %s error: %v", secretFile, err)
	}

	err = mergeData(config, secret)
	if err != nil {
		notify.Stop("Merge configs error: %v", err)
	}

	// nodeName flag is only considered for node plugin
	if "" == app.GetGlobalConfig().NodeName && !app.GetGlobalConfig().Controller {
		log.Warningln("Node name is empty. Topology aware volume provisioning feature may not behave normal")
	}

	connector.ScanVolumeTimeout = time.Second * time.Duration(app.GetGlobalConfig().ScanVolumeTimeout)
}

func getSecret(backendSecret, backendConfig map[string]interface{}, secretKey string) {
	if secretValue, exist := backendSecret[secretKey].(string); exist {
		backendConfig[secretKey] = secretValue
	} else {
		log.Fatalln(fmt.Sprintf("The key %s is not in secret %v.", secretKey, backendSecret))
	}
}

func mergeData(config CSIConfig, secret CSISecret) error {
	for _, backendConfig := range config.Backends {
		backendName, exist := backendConfig["name"].(string)
		if !exist {
			return fmt.Errorf("the key name does not exist in backend")
		}
		Secret, exist := secret.Secrets[backendName]
		if !exist {
			return fmt.Errorf("the key %s is not in secret", backendName)
		}

		backendSecret := Secret.(map[string]interface{})
		getSecret(backendSecret, backendConfig, "user")
		getSecret(backendSecret, backendConfig, "password")
		// 兼容之前的后端注册，后续删除所有相关代码
		backendConfig["secretName"] = backendName
		backendConfig["secretNamespace"] = "huawei-csi"
	}
	return nil
}

func updateBackendCapabilities() {
	err := backend.SyncUpdateCapabilities()
	if err != nil {
		notify.Stop("Update backend capabilities error: %v", err)
	}

	ticker := time.NewTicker(time.Second * time.Duration(app.GetGlobalConfig().BackendUpdateInterval))
	for range ticker.C {
		backend.AsyncUpdateCapabilities()
	}
}

func getLogFileName() string {
	if app.GetGlobalConfig().Controller {
		return controllerLogFile
	}

	return nodeLogFile
}

func ensureRuntimePanicLogging(ctx context.Context) {
	utils.RecoverPanic(ctx)

	log.Flush()
	log.Close()
}

func releaseStorageClient() {
	backend.LogoutBackend()
}

func runCSIController(ctx context.Context) {
	go exitClean(true)

	go updateBackendCapabilities()

	app.GetGlobalConfig().K8sUtils.Activate()

	// register the kahu community DRCSI service
	go registerDRCSIServer()

	// register the K8S community CSI service
	registerCSIServer()
}

func runCSINode(ctx context.Context) {
	go exitClean(false)

	// Init file lock
	err := lock.InitLock(app.GetGlobalConfig().DriverName)
	if err != nil {
		notify.Stop("Init Lock error for driver %s: %v", app.GetGlobalConfig().DriverName, err)
	}

	// Init version file on every node
	err = version.InitVersion(versionFile, csiVersion)
	if err != nil {
		log.AddContext(ctx).Warningf("Init version error: %v", err)
	}

	checkMultiPathService()

	triggerGarbageCollector()

	// Save host info to secret, such as: hostname, initiator
	go func() {
		if err := host.SaveNodeHostInfoToSecret(context.Background()); err != nil {
			notify.Stop("SaveNodeHostInfo fail ,error: [%v]", err)
		}
		log.Infof("save node info to secret success")
	}()

	// register the kahu community DRCSI service
	go registerDRCSIServer()

	// register the K8S community CSI service
	registerCSIServer()
}

func main() {
	// Processing Input Parameters
	if err := app.NewCommand().Execute(); err != nil {
		logrus.Fatalf("Execute app command failed. error: %v", err)
	}

	// Init logger
	err := log.InitLogging(&log.LoggingRequest{
		LogName:       getLogFileName(),
		LogFileSize:   app.GetGlobalConfig().LogFileSize,
		LoggingModule: app.GetGlobalConfig().LoggingModule,
		LogLevel:      app.GetGlobalConfig().LogLevel,
		LogFileDir:    app.GetGlobalConfig().LogFileDir,
		MaxBackups:    app.GetGlobalConfig().MaxBackups,
	})
	if err != nil {
		logrus.Fatalf("Init log error: %v", err)
	}

	// Start CSI service
	if app.GetGlobalConfig().Controller {
		runCSIController(context.Background())
	} else {
		runCSINode(context.Background())
	}
}

func registerDRCSIServer() {
	p := provider.NewProvider(app.GetGlobalConfig().DriverName, csiVersion)
	drListener := listenEndpoint(app.GetGlobalConfig().DrEndpoint)
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(log.EnsureGRPCContext),
	}
	grpcServer := grpc.NewServer(opts...)
	drcsi.RegisterIdentityServer(grpcServer, p)
	drcsi.RegisterStorageBackendServer(grpcServer, p)

	if err := grpcServer.Serve(drListener); err != nil {
		notify.Stop("Start Huawei CSI driver error: %v", err)
	}
}

func registerCSIServer() {
	d := driver.NewDriver(app.GetGlobalConfig().DriverName,
		csiVersion,
		app.GetGlobalConfig().K8sUtils,
		app.GetGlobalConfig().NodeName)
	listener := listenEndpoint(app.GetGlobalConfig().Endpoint)
	registerServer(listener, d)
}

func listenEndpoint(endpoint string) net.Listener {
	endpointDir := filepath.Dir(endpoint)
	_, err := os.Stat(endpointDir)
	if err != nil && os.IsNotExist(err) {
		err = os.Mkdir(endpointDir, endpointDirPerm)
		if err != nil {
			notify.Stop("Error creating directory %s. error: %v", endpoint, err)
		}
	} else {
		_, err = os.Stat(endpoint)
		if err == nil {
			log.Infof("Gonna remove old sock file %s", endpoint)
			err = os.Remove(endpoint)
			if err != nil {
				notify.Stop("Error removing directory %s. error: %v", endpoint, err)
			}
		}
	}
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		notify.Stop("Listen on %s error: %v", endpoint, err)
	}
	return listener
}

func registerServer(listener net.Listener, d *driver.Driver) {
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(log.EnsureGRPCContext),
	}
	server := grpc.NewServer(opts...)

	csi.RegisterIdentityServer(server, d)
	csi.RegisterControllerServer(server, d)
	csi.RegisterNodeServer(server, d)

	log.Infof("Starting Huawei CSI driver, listening on %s", app.GetGlobalConfig().Endpoint)
	if err := server.Serve(listener); err != nil {
		notify.Stop("Start Huawei CSI driver error: %v", err)
	}
}

func checkMultiPathService() {
	multipathConfig := map[string]interface{}{
		"SCSIMultipathType":  app.GetGlobalConfig().ScsiMultiPathType,
		"NVMeMultipathType":  app.GetGlobalConfig().NvmeMultiPathType,
		"volumeUseMultiPath": app.GetGlobalConfig().VolumeUseMultiPath,
	}

	requiredServices, err := utils.GetRequiredMultipath(context.Background(),
		multipathConfig, config.Backends)
	if err != nil {
		notify.Stop("Get required multipath services failed. Error: %v", err)
	}

	err = connutils.VerifyMultipathService(requiredServices,
		utils.GetForbiddenMultipath(context.Background(), multipathConfig, config.Backends))
	if err != nil {
		notify.Stop("Check multipath service failed. error:%v", err)
	}
	log.Infof("Check multipath service success.")
}

func triggerGarbageCollector() {
	// Trigger stale device clean up and exit after cleanup completion or during timeout
	log.Debugf("Enter func triggerGarbageCollector")
	cleanupReport := make(chan error, 1)
	go func(ch chan error) {
		res := nodeStaleDeviceCleanup(context.Background(),
			app.GetGlobalConfig().K8sUtils,
			app.GetGlobalConfig().KubeletRootDir,
			app.GetGlobalConfig().DriverName,
			app.GetGlobalConfig().NodeName)
		ch <- res
		close(ch)
	}(cleanupReport)
	timeoutInterval := time.Second * time.Duration(app.GetGlobalConfig().DeviceCleanupTimeout)
	select {
	case report := <-cleanupReport:
		if report == nil {
			log.Infof("Successfully completed stale device garbage collection")
		} else {
			log.Errorf("Stale device garbage collection exited with error %s", report)
		}
	case <-time.After(timeoutInterval):
		log.Warningf("Stale device garbage collection incomplete, exited due to timeout")
	}
	return
}

func exitClean(isController bool) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	stopChan := notify.GetStopChan()
	defer close(signalChan)
	defer close(stopChan)

	select {
	case sign := <-signalChan:
		log.Infof("Receive exit signal %v", sign)
		clean(isController)
	case <-stopChan:
		log.Infoln("Receive stop event")
		clean(isController)
		os.Exit(-1)
	}
}

func clean(isController bool) {
	// flush log
	ensureRuntimePanicLogging(context.TODO())
	if isController {
		// release client
		releaseStorageClient()
		app.GetGlobalConfig().K8sUtils.Deactivate()
	} else {
		// clean version file
		err := version.ClearVersion(versionFile)
		if err != nil {
			logrus.Warningf("clean version file error: %v", err)
		}
	}
}

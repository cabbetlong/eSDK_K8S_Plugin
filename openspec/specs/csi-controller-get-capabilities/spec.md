## Purpose

定义 CSI ControllerGetCapabilities RPC 的接口规范，用于通告驱动支持的控制器服务能力。能力列表由启动参数决定，不依赖后端配置。基础能力包括卷创建、发布、扩容、快照和克隆功能；当启用健康监控启动参数时，额外通告卷查询和卷状态能力。

## Requirements

### Requirement: ControllerGetCapabilities RPC 必须通告控制器能力

ControllerGetCapabilities RPC SHALL 通告驱动支持的控制器服务能力。基础能力包括：CREATE_DELETE_VOLUME、PUBLISH_UNPUBLISH_VOLUME、EXPAND_VOLUME、CREATE_DELETE_SNAPSHOT、CLONE_VOLUME。
当 HealthMonitorEnabled 启动参数（--health-monitor-enabled）为 true 时，SHALL 额外通告：GET_VOLUME 和 VOLUME_CONDITION（两者必须同时通告，不可独立控制）。能力列表在每次调用时根据当前配置读取，不依赖后端配置。

#### Scenario: CO 查询控制器能力（健康监控未启用）

- **WHEN** CO 发送 ControllerGetCapabilitiesRequest，且 HealthMonitorEnabled 为 false（默认值）
- **THEN** 驱动返回 ControllerGetCapabilitiesResponse，包含能力：CREATE_DELETE_VOLUME、PUBLISH_UNPUBLISH_VOLUME、EXPAND_VOLUME、CREATE_DELETE_SNAPSHOT、CLONE_VOLUME

#### Scenario: CO 查询控制器能力（健康监控已启用）

- **WHEN** CO 发送 ControllerGetCapabilitiesRequest，且 HealthMonitorEnabled 为 true（通过 --health-monitor-enabled 启动参数启用）
- **THEN** 驱动返回 ControllerGetCapabilitiesResponse，包含能力：CREATE_DELETE_VOLUME、PUBLISH_UNPUBLISH_VOLUME、EXPAND_VOLUME、CREATE_DELETE_SNAPSHOT、CLONE_VOLUME、GET_VOLUME、VOLUME_CONDITION

#### Scenario: GET_VOLUME 和 VOLUME_CONDITION 必须同时通告

- **WHEN** CO 发送 ControllerGetCapabilitiesRequest，且 HealthMonitorEnabled 为 true 时
- **THEN** 驱动返回的能力列表中 GET_VOLUME 和 VOLUME_CONDITION 必须同时存在，不可仅通告其中一个

#### Scenario: 能力列表不依赖后端配置

- **WHEN** CO 发送 ControllerGetCapabilitiesRequest，且尚未配置任何存储后端时
- **THEN** 驱动仍返回完整的控制器能力列表（能力列表由驱动自身功能决定，与后端是否已配置无关）

#### Scenario: 处理 panic 恢复

- **WHEN** ControllerGetCapabilities 处理程序在执行期间遇到 panic 时
- **THEN** 驱动恢复 panic，记录堆栈跟踪，并返回适当的错误响应

---

### Requirement: 未实现的控制器 RPC

以下控制器 RPC SHALL NOT 由此驱动实现，对所有请求返回 codes.Unimplemented：ListVolumes、GetCapacity、ValidateVolumeCapabilities。

#### Scenario: CO 请求卷列表

- **WHEN** CO 发送 ListVolumesRequest 时
- **THEN** 驱动返回 codes.Unimplemented 错误，消息为 "Not implemented"

#### Scenario: CO 请求存储容量信息

- **WHEN** CO 发送 GetCapacityRequest 时
- **THEN** 驱动返回 codes.Unimplemented 错误，消息为 "Not implemented"

#### Scenario: CO 请求卷能力验证

- **WHEN** CO 发送 ValidateVolumeCapabilitiesRequest 时
- **THEN** 驱动返回 codes.Unimplemented 错误，消息为 "Not implemented"

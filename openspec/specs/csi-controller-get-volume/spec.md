## Purpose

定义 CSI ControllerGetVolume RPC 的接口规范，用于查询存储卷的健康状态。该接口由 csi-external-health-monitor-controller sidecar 调用，用于在 PVC 上设置 VolumeStatus 条件。不同存储类型使用不同的健康检测策略：OceanStor SAN/NAS 和 FusionStorage SAN 检查 HEALTHSTATUS/health_status 字段，DTree 和 FusionStorage NAS 仅检查卷是否存在，OceanDisk/A-Series/DME 始终返回正常。

## Requirements

### Requirement: ControllerGetVolume RPC 必须返回卷健康状态

ControllerGetVolume RPC SHALL 查询存储后端并返回卷的健康状态。返回的 VolumeCondition 中 `abnormal` 字段表示卷健康状态（true 为异常，false 为正常），`message` 字段提供状态描述。驱动 SHALL NOT 返回 `published` 字段。

#### Scenario: OceanStor SAN 卷健康状态正常

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor SAN 类型存储卷且存储后端 LUN 存在且 HEALTHSTATUS 不为故障值时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 false，`message` 为 "Volume {name} is normal"

#### Scenario: OceanStor SAN 卷健康状态异常（HEALTHSTATUS 故障）

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor SAN 类型存储卷且存储后端 LUN 的 HEALTHSTATUS 为故障值（"2"）时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Volume {name} health status is fault"

#### Scenario: OceanStor SAN 卷不存在

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor SAN 类型存储卷且存储后端 LUN 不存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Volume {name} not found"

#### Scenario: OceanStor SAN 卷查询 API 失败

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor SAN 类型存储卷且 LUN 查询失败时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Query volume {name} error: {err}"

#### Scenario: OceanStor NAS 卷健康状态正常

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor NAS 类型存储卷且存储后端文件系统存在且 HEALTHSTATUS 不为故障值时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 false，`message` 为 "volume {name} is normal"

#### Scenario: OceanStor NAS 卷健康状态异常（HEALTHSTATUS 故障）

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor NAS 类型存储卷且存储后端文件系统的 HEALTHSTATUS 为故障值（"2"）时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "volume {name} health status is fault"

#### Scenario: OceanStor NAS 卷不存在

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor NAS 类型存储卷且存储后端文件系统不存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "volume {name} not found"

#### Scenario: OceanStor NAS 卷查询 API 失败

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor NAS 类型存储卷且文件系统查询失败时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Query volume {name} error: {err}"

#### Scenario: OceanStor DTree 卷健康状态正常

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor DTree 类型存储卷且 DTree 存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 false，`message` 为 "volume {name} is normal"（DTree 仅检查存在性，不检查 HEALTHSTATUS 字段）

#### Scenario: OceanStor DTree 卷不存在

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor DTree 类型存储卷且 DTree 不存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "volume {name} not found"

#### Scenario: OceanStor DTree 卷查询 API 失败

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanStor DTree 类型存储卷且 DTree 查询失败时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Query volume {name} error: {err}"

#### Scenario: FusionStorage SAN 卷健康状态正常

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage SAN 类型存储卷且存储后端卷存在且健康状态不为故障值时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 false，`message` 为 "Volume {name} is normal"

#### Scenario: FusionStorage SAN 卷健康状态异常（health_status 故障）

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage SAN 类型存储卷且存储后端卷的健康状态为故障值时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Volume {name} health status is fault"

#### Scenario: FusionStorage SAN 卷不存在

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage SAN 类型存储卷且存储后端卷不存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Volume {name} not found"

#### Scenario: FusionStorage SAN 卷查询 API 失败

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage SAN 类型存储卷且卷查询失败时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Query volume {name} error: {err}"

#### Scenario: FusionStorage NAS 卷健康状态正常

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage NAS 类型存储卷且存储后端文件系统存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 false，`message` 为 "Volume {name} exists and is normal"（FusionStorage NAS 仅检查存在性，不检查 HEALTHSTATUS 字段）

#### Scenario: FusionStorage NAS 卷不存在

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage NAS 类型存储卷且存储后端文件系统不存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Volume {name} not found"

#### Scenario: FusionStorage NAS 卷查询 API 失败

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage NAS 类型存储卷且文件系统查询失败时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Query volume {name} error: {err}"

#### Scenario: FusionStorage DTree 卷健康状态正常

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage DTree 类型存储卷且 DTree 存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 false，`message` 为 "volume {name} is normal"（FusionStorage DTree 仅检查存在性，不检查 HEALTHSTATUS 字段）

#### Scenario: FusionStorage DTree 卷不存在

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage DTree 类型存储卷且 DTree 不存在时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "volume {name} not found"

#### Scenario: FusionStorage DTree 卷查询 API 失败

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 FusionStorage DTree 类型存储卷且 DTree 查询失败时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 true，`message` 为 "Query volume {name} error: {err}"

#### Scenario: OceanDisk/A-Series/DME 卷状态始终正常

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 OceanDisk SAN、OceanStor A-Series 或 DME 类型存储卷时
- **THEN** 驱动返回 ControllerGetVolumeResponse，VolumeCondition 中 `abnormal` 为 false，`message` 为空（这些存储类型未实现 GetVolumeStatus，继承 basePlugin 默认值）

#### Scenario: 卷 ID 为空

- **WHEN** CO 发送 ControllerGetVolumeRequest，volume_id 为空字符串时
- **THEN** 驱动返回 codes.InvalidArgument 错误，消息为 "no volume ID provided"

#### Scenario: 存储后端不可用

- **WHEN** CO 发送 ControllerGetVolumeRequest，SelectBackend 失败或返回 nil 时
- **THEN** 驱动返回 codes.Internal 错误，消息包含后端名称和错误详情

#### Scenario: DTree 卷获取 parentName 失败

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 DTree 类型存储卷且无法获取 DTree parentName 时
- **THEN** 驱动返回 codes.Internal 错误，包含错误详情

#### Scenario: VolumeId 格式无效

- **WHEN** CO 发送 ControllerGetVolumeRequest，VolumeId 不包含 "." 分隔符（无法解析出 volName）时
- **THEN** 驱动解析 VolumeId 后卷名为空，SelectBackend 可能因 backendName 不匹配返回 nil，驱动返回 codes.Internal 错误

#### Scenario: 处理 panic 恢复

- **WHEN** ControllerGetVolume 处理程序在执行期间遇到 panic 时
- **THEN** 驱动恢复 panic，记录堆栈跟踪，并返回适当的错误响应

---

### Requirement: ControllerGetVolume 必须处理 DTree 存储的 ParentName

ControllerGetVolume RPC SHALL 对 DTree 类型存储卷额外查询 parentName，供 GetVolumeStatus 使用。DTree 的 GetDTreeByName API 需要 parentName 和 vStoreId（OceanStor）或 parentName（FusionStorage）来定位 DTree。

#### Scenario: DTree 存储卷请求

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求 DTree 类型存储卷时
- **THEN** 驱动获取 parentName，并传给 GetVolumeStatus 的 VolumeQuery.ParentName

#### Scenario: 非 DTree 存储卷请求

- **WHEN** CO 发送 ControllerGetVolumeRequest，请求非 DTree 类型存储卷时
- **THEN** VolumeQuery.ParentName 为空字符串

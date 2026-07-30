## Purpose

定义 CSI ControllerDeleteSnapshot RPC 的接口规范，用于从华为存储后端删除卷快照，支持幂等删除操作。

## Requirements

### Requirement: DeleteSnapshot RPC 必须删除卷快照
DeleteSnapshot RPC SHALL 从华为存储后端删除快照。SnapshotId 格式为 "backendName.parentID.snapshotName"。驱动分割 SnapshotId 获取 backendName、snapshotParentId 和 snapshotName，选择后端并委托插件执行删除。插件内部可能对 snapshotName 进行截断或转换（与 CreateSnapshot 一致：OceanStor SAN 截断至 31 字符、FusionStorage SAN 截断至 95 字符、NAS 将 `-` 替换为 `_`）。删除操作是幂等的：如果快照已不存在，返回成功。

#### Scenario:删除快照
- **WHEN** CO 发送带有有效 SnapshotId（格式："backendName.parentID.snapshotName"）的 DeleteSnapshotRequest 时
- **THEN** 驱动分割 SnapshotId 获取 backendName、snapshotParentId 和 snapshotName，选择后端，在后端上删除快照（不同存储类型可能对快照名称进行转换后执行删除），成功后返回空的 DeleteSnapshotResponse

#### Scenario:快照已不存在时删除成功（幂等）
- **WHEN** CO 发送 DeleteSnapshotRequest 且存储后端上该快照已不存在时
- **THEN** 驱动返回空的 DeleteSnapshotResponse，不报错（存储层通过先查询再删除的模式实现幂等：若快照为空则直接返回成功；OceanStor 客户端还会在删除 API 返回"快照不存在"错误码时将其视为成功）

#### Scenario:后端不存在时删除快照（幂等）
- **WHEN** CO 发送 SnapshotId 引用不存在后端的 DeleteSnapshotRequest 时
- **THEN** 驱动记录警告日志，返回空的 DeleteSnapshotResponse，并注明需要从存储阵列手动清理

#### Scenario:拒绝快照 ID 缺失的 DeleteSnapshot
- **WHEN** CO 发送 SnapshotId 为空的 DeleteSnapshotRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，消息为 "Snapshot ID missing in request"

#### Scenario:拒绝插件删除失败的 DeleteSnapshot
- **WHEN** CO 发送 DeleteSnapshotRequest 且后端删除快照操作返回错误时
- **THEN** 驱动返回 codes.Internal 错误，包含插件错误消息

#### Scenario:拒绝不支持快照的后端类型的 DeleteSnapshot
- **WHEN** CO 发送 DeleteSnapshotRequest 且后端类型（如 DTree、OceanDisk、FusionStorage NAS、OceanStor A Series）不支持快照功能时
- **THEN** 驱动返回 codes.Internal 错误，指示该后端类型不支持快照功能

#### Scenario:拒绝 NAS 逻辑端口不在本站且无 HyperMetro 的 DeleteSnapshot
- **WHEN** CO 发送 DeleteSnapshotRequest 且后端为 NAS 类型、逻辑端口未运行在本站且未配置 HyperMetro 远端插件时
- **THEN** 驱动返回 codes.Internal 错误，指示逻辑端口不在本站运行

#### Scenario:NAS HyperMetro 双站同 WWN 时快照不存在的 DeleteSnapshot 返回错误
- **WHEN** CO 发送 DeleteSnapshotRequest 且后端为 NAS 类型、本地和远端客户端的 Site WWN 相同但快照在两端均不存在时
- **THEN** 驱动返回 codes.Internal 错误，指示逻辑端口运行在同一站点上，无法确定快照位置

#### Scenario:OceanStor SAN HyperMetro 远端快照同步清理
- **WHEN** CO 发送 DeleteSnapshotRequest 且后端为 OceanStor SAN 类型、配置了 HyperMetro 且远端也存在该快照时
- **THEN** 驱动按顺序执行：停用远端快照、删除远端快照、停用本地快照、删除本地快照，成功后返回空的 DeleteSnapshotResponse

---

### Requirement: ListSnapshots RPC 未实现
ListSnapshots RPC SHALL NOT 由此驱动实现。驱动对所有请求返回 codes.Unimplemented。

#### Scenario:CO 请求快照列表
- **WHEN** CO 发送 ListSnapshotsRequest 时
- **THEN** 驱动返回 codes.Unimplemented 错误，消息为空

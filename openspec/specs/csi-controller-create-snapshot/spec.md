## Purpose

定义 CSI ControllerCreateSnapshot RPC 的接口规范，用于在华为存储后端上创建卷快照，支持快照 ID 格式化和同步创建。

## Requirements

### Requirement: CreateSnapshot RPC 必须创建卷快照
CreateSnapshot RPC SHALL 在华为存储后端上创建现有卷的快照。快照 ID 格式为 "backendName.parentID.snapshotName"。快照同步创建并立即可用（ReadyToUse=true）。

#### Scenario:从卷创建快照
- **WHEN** CO 发送带有 SourceVolumeId、快照 Name 和可选 Parameters 的 CreateSnapshotRequest 时
- **THEN** 驱动分割 SourceVolumeId 获取 backendName 和 volName，选择后端，复制请求参数，在后端上创建快照（不同存储类型可能对快照名称进行转换：OceanStor SAN 截断至 31 字符、FusionStorage SAN 截断至 95 字符、NAS 将 `-` 替换为 `_`），并返回 CreateSnapshotResponse，包含 Snapshot：SizeBytes（来自插件结果的 int64）、SnapshotId（格式："backendName.ParentID.snapshotName"，其中 snapshotName 使用原始请求值而非转换后的值）、SourceVolumeId（原始值）、CreationTime（从插件结果转换的 int64 转换为 timestamppb），以及 ReadyToUse=true

#### Scenario:拒绝源卷 ID 缺失的 CreateSnapshot
- **WHEN** CO 发送 SourceVolumeId 为空的 CreateSnapshotRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，消息为 "Volume ID missing in request"

#### Scenario:拒绝快照名称缺失的 CreateSnapshot
- **WHEN** CO 发送 Name 为空的 CreateSnapshotRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，消息为 "Snapshot Name missing in request"

#### Scenario:拒绝后端不存在时的 CreateSnapshot
- **WHEN** CO 发送 SourceVolumeId 引用不存在后端的 CreateSnapshotRequest 时
- **THEN** 驱动返回 codes.Internal 错误，指示后端不存在

#### Scenario:拒绝插件创建失败的 CreateSnapshot
- **WHEN** CO 发送 CreateSnapshotRequest 且后端创建快照操作返回错误时
- **THEN** 驱动返回 codes.Internal 错误，包含插件错误消息

#### Scenario:快照已存在且源卷一致（幂等成功）
- **WHEN** CO 发送 CreateSnapshotRequest 且存储后端已存在同名快照且其父卷与 SourceVolumeId 对应的卷一致时
- **THEN** 驱动返回 CreateSnapshotResponse，包含已有快照的信息（SizeBytes、SnapshotId、SourceVolumeId、CreationTime、ReadyToUse=true），不重复创建快照

#### Scenario:拒绝快照已存在但源卷不一致的 CreateSnapshot
- **WHEN** CO 发送 CreateSnapshotRequest 且存储后端已存在同名快照但其父卷与 SourceVolumeId 对应的卷不一致时
- **THEN** 驱动返回 codes.Internal 错误，指示快照已存在但父卷不兼容

#### Scenario:拒绝源卷在存储后端不存在的 CreateSnapshot
- **WHEN** CO 发送 CreateSnapshotRequest 且 SourceVolumeId 对应的卷在存储后端上不存在时
- **THEN** 驱动返回 codes.Internal 错误，指示源卷不存在

#### Scenario:拒绝不支持快照的后端类型的 CreateSnapshot
- **WHEN** CO 发送 CreateSnapshotRequest 且后端类型（如 DTree、OceanDisk）不支持快照功能时
- **THEN** 驱动返回 codes.Internal 错误，指示该后端类型不支持快照功能

#### Scenario:拒绝 NAS 逻辑端口不在本站且无 HyperMetro 的 CreateSnapshot
- **WHEN** CO 发送 CreateSnapshotRequest 且后端为 NAS 类型、逻辑端口未运行在本站且未配置 HyperMetro 远端插件时
- **THEN** 驱动返回 codes.Internal 错误，指示逻辑端口故障切换

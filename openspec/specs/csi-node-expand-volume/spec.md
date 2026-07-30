## Purpose

定义 CSI NodeExpandVolume RPC 的接口规范，用于在节点侧执行卷扩容操作，包括块设备调整和文件系统扩容。

## Requirements

### Requirement: NodeExpandVolume RPC 必须在节点上扩容卷
NodeExpandVolume RPC SHALL 在控制器在存储阵列上扩容卷之后执行节点侧的卷扩容。驱动为后端创建 manage.Manager 并委托扩容操作。对于 SAN 卷，这涉及调整块设备大小以及可选的文件系统扩容。对于 NAS 卷，不需要扩容（NAS 的 NodeExpansionRequired 始终为 false）。

#### Scenario:在节点上扩容 SAN 块卷
- **WHEN** CO 发送带有 VolumeId、VolumePath、CapacityRange（RequiredBytes > 0）和 StagingTargetPath 的 NodeExpandVolumeRequest 时
- **THEN** 驱动验证 CapacityRange（RequiredBytes > 0）和 VolumePath（非空），分割 VolumeId 获取 backendName，为 SAN 后端创建管理器，从暂存路径检索设备 WWN（不检查设备引用或保存到磁盘），调整块设备大小至 RequiredBytes，成功后返回空的 NodeExpandVolumeResponse

#### Scenario:在节点上扩容 SAN 文件系统卷
- **WHEN** CO 发送 VolumeCapability.GetMount() != nil 的 NodeExpandVolumeRequest 时
- **THEN** 驱动执行与块卷相同的块设备调整大小，然后额外扩容文件系统，并返回空的 NodeExpandVolumeResponse

#### Scenario:在节点上扩容 NAS 卷
- **WHEN** CO 发送 NAS 后端上卷的 NodeExpandVolumeRequest 时
- **THEN** 驱动创建 NasManager，NasManager.ExpandVolume 立即返回 nil（NAS 卷不需要节点侧扩容）

#### Scenario:拒绝缺少容量范围的 NodeExpandVolume
- **WHEN** CO 发送缺少或无效 CapacityRange（RequiredBytes <= 0）的 NodeExpandVolumeRequest 时
- **THEN** 驱动返回 codes.Internal 错误，消息为 "NodeExpandVolume CapacityRange must be provided"

#### Scenario:拒绝缺少卷路径的 NodeExpandVolume
- **WHEN** CO 发送 VolumePath 为空的 NodeExpandVolumeRequest 时
- **THEN** 驱动返回 codes.Internal 错误，消息为 "NodeExpandVolume volumePath must be provided"

#### Scenario:拒绝创建管理器失败的 NodeExpandVolume
- **WHEN** CO 发送 NodeExpandVolumeRequest 且后端管理器创建失败时
- **THEN** 驱动返回 codes.Internal 错误，消息指示后端和失败原因

#### Scenario:拒绝块设备调整大小失败的 NodeExpandVolume
- **WHEN** CO 发送 NodeExpandVolumeRequest 且块设备调整大小失败时
- **THEN** 驱动返回 codes.Internal 错误，包含调整大小失败原因

#### Scenario:拒绝文件系统扩容失败的 NodeExpandVolume
- **WHEN** CO 发送文件系统卷的 NodeExpandVolumeRequest 且文件系统扩容失败时
- **THEN** 驱动返回 codes.Internal 错误，包含挂载路径扩容失败原因

#### Scenario:设备已达到目标大小时的幂等 NodeExpandVolume
- **WHEN** CO 发送 NodeExpandVolumeRequest 且块设备当前大小已大于或等于 RequiredBytes 时
- **THEN** 驱动跳过块设备 rescan，直接返回空的 NodeExpandVolumeResponse

#### Scenario:拒绝 WWN 检索失败的 NodeExpandVolume
- **WHEN** CO 发送 NodeExpandVolumeRequest 且从暂存路径检索设备 WWN 失败时
- **THEN** 驱动返回 codes.Internal 错误，包含 WWN 检索失败原因

#### Scenario:拒绝找不到虚拟设备的 NodeExpandVolume
- **WHEN** CO 发送 NodeExpandVolumeRequest 且 WWN 对应的虚拟设备不存在时
- **THEN** 驱动返回 codes.Internal 错误，消息为 "Can not find the device for lun {wwn}"

#### Scenario:拒绝块设备 rescan 超时的 NodeExpandVolume
- **WHEN** CO 发送 NodeExpandVolumeRequest 且块设备 rescan 后 120 秒内未达到目标大小时
- **THEN** 驱动返回 codes.Internal 错误，消息为 "Wait timeout"

#### Scenario:拒绝不支持的文件系统类型的 NodeExpandVolume
- **WHEN** CO 发送文件系统卷的 NodeExpandVolumeRequest 且文件系统类型不在支持列表（ext2/ext3/ext4/xfs）中时
- **THEN** 驱动返回 codes.Internal 错误，消息为 "resize of format {fsType} is not supported for device {devicePath}"

#### Scenario:未格式化设备跳过文件系统扩容的 NodeExpandVolume
- **WHEN** CO 发送文件系统卷的 NodeExpandVolumeRequest 且设备未格式化（无文件系统）时
- **THEN** 驱动跳过文件系统扩容，返回空的 NodeExpandVolumeResponse

#### Scenario:拒绝 findmnt 获取挂载设备失败的 NodeExpandVolume
- **WHEN** CO 发送文件系统卷的 NodeExpandVolumeRequest 且 findmnt 命令无法获取卷路径对应的挂载设备时
- **THEN** 驱动返回 codes.Internal 错误，包含 findmnt 失败原因或 "could not get valid device for mount path"

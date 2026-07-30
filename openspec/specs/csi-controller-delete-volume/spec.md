## Purpose

定义 CSI ControllerDeleteVolume RPC 的接口规范，用于从华为存储后端删除卷，支持 DTree、NAS 和标准 LUN/FileSystem 存储类型。删除操作遵循 CSI 幂等性要求：当卷或后端已不存在时，返回成功。

## Requirements

### Requirement: DeleteVolume RPC 必须删除存储卷
DeleteVolume RPC SHALL 从华为存储后端删除卷。VolumeId 格式为 "backendName.volumeName"。驱动必须处理不同的存储类型（DTree、OceanStor A-Series NAS 和标准 LUN/FileSystem），并使用适当的删除参数。删除操作是幂等的：如果卷已不存在，返回成功；如果后端不再存在，请求返回成功并带有警告。

#### Scenario:删除标准 LUN/FileSystem 卷
- **WHEN** CO 发送带有有效 VolumeId（格式："backendName.volumeName"）的 DeleteVolumeRequest 时
- **THEN** 驱动分割 VolumeId，选择后端，并在后端上删除卷，成功后返回空的 DeleteVolumeResponse

#### Scenario:删除 DTree 卷
- **WHEN** CO 发送 DTree 存储后端（包括 oceanstor-dtree、fusionstorage-dtree、oceanstor-a-series-dtree）上卷的 DeleteVolumeRequest 时
- **THEN** 驱动从卷 ID 映射中获取 DTree parentName，并在后端上删除 DTree 卷

#### Scenario:删除 OceanStor A-Series NAS 卷
- **WHEN** CO 发送 OceanStorASeriesNas 存储上卷的 DeleteVolumeRequest 时
- **THEN** 驱动按 volumeId 获取 KvCacheStoreId，如果存在则构造包含 KvCacheStoreId 的删除参数，并在后端上删除卷

#### Scenario:后端不存在时删除卷（幂等）
- **WHEN** CO 发送 VolumeId 引用不再存在后端的 DeleteVolumeRequest 时
- **THEN** 驱动记录警告日志，返回成功（codes.OK）并带有空的 DeleteVolumeResponse，并注明需要从存储阵列手动清理

#### Scenario:后端选择失败时删除卷（幂等）
- **WHEN** CO 发送 DeleteVolumeRequest 且后端选择返回错误（后端 CRD 存在但加载/初始化失败）时
- **THEN** 驱动将其视为与后端未找到相同的情况，返回成功并带有空响应，并记录警告

#### Scenario:卷已不存在时删除成功（幂等）
- **WHEN** CO 发送 DeleteVolumeRequest 且存储后端上该卷已不存在时
- **THEN** 驱动返回空的 DeleteVolumeResponse，不报错（存储层通过先查询再删除的模式实现幂等：SAN 插件查询 LUN 为空时直接返回 nil；NAS 插件查询文件系统为空时直接返回 nil；DTree 插件查询 DTree 或父文件系统不存在时直接返回 nil）

#### Scenario:拒绝 VolumeId 为空的 DeleteVolume
- **WHEN** CO 发送 VolumeId 为空的 DeleteVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，消息为 "Volume ID missing in request"

#### Scenario:拒绝 VolumeId 格式无效的 DeleteVolume
- **WHEN** CO 发送 VolumeId 不包含 "." 分隔符（无法解析出 volName）的 DeleteVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示 VolumeId 格式无效

#### Scenario:拒绝插件删除失败的 DeleteVolume
- **WHEN** CO 发送 DeleteVolumeRequest 且后端删除卷操作返回错误时
- **THEN** 驱动返回 codes.Internal 错误，包含插件错误消息

#### Scenario:DTree parentName 查找失败时删除卷
- **WHEN** CO 发送 DTree 存储后端上卷的 DeleteVolumeRequest，但无法获取 DTree parentName 时
- **THEN** 驱动返回错误（可能原因包括：PV 不存在、PV 缺少 dTreeParentName 字段、多个 PV 具有相同 volumeId 但 dTreeParentName 冲突）

#### Scenario:A-Series NAS kvCacheStoreId 查找失败时删除卷
- **WHEN** CO 发送 OceanStorASeriesNas 存储上卷的 DeleteVolumeRequest，但无法获取 KvCacheStoreId 时
- **THEN** 驱动返回错误（可能原因包括：PV 不存在、PV volume attributes 为空）

#### Scenario:NAS 卷存在快照时删除失败
- **WHEN** CO 发送 NAS 存储上卷的 DeleteVolumeRequest，且该文件系统上仍存在快照时
- **THEN** 驱动返回 codes.Internal 错误，指示文件系统上存在快照，需要先删除快照

#### Scenario:删除 HyperMetro SAN 卷
- **WHEN** CO 发送含有 HyperMetro 配置的 SAN 卷的 DeleteVolumeRequest 时
- **THEN** 驱动先删除 HyperMetro 对，再删除远端 LUN，最后删除本地 LUN，成功后返回空的 DeleteVolumeResponse

#### Scenario:删除 HyperMetro NAS 卷
- **WHEN** CO 发送含有 HyperMetro 配置的 NAS 卷的 DeleteVolumeRequest 时
- **THEN** 驱动设置活跃客户端，删除 HyperMetro 共享，删除 HyperMetro 关系，删除远端文件系统，最后删除本地文件系统，成功后返回空的 DeleteVolumeResponse

#### Scenario:NAS 逻辑端口不在本站时删除失败
- **WHEN** CO 发送 NAS 存储上卷的 DeleteVolumeRequest，且逻辑端口未运行在本站（failover 状态）且未配置 HyperMetro 远端插件时
- **THEN** 驱动返回 codes.Internal 错误，指示逻辑端口不在本站运行

#### Scenario:删除 FusionStorage SAN 卷时先移除 QoS
- **WHEN** CO 发送 FusionStorage SAN 存储上卷的 DeleteVolumeRequest 时
- **THEN** 驱动先移除卷关联的 QoS 策略，再删除卷，成功后返回空的 DeleteVolumeResponse

#### Scenario:删除 A-Series NAS 卷时处理 KV Cache 清理
- **WHEN** CO 发送 OceanStor A-Series NAS 卷的 DeleteVolumeRequest，且卷关联了 KvCacheStoreId 时
- **THEN** 驱动按顺序执行：删除 NFS/DataTurbo 共享、删除文件系统、删除 KV Cache 存储，使用事务模式（任一步骤失败则回滚），成功后返回空的 DeleteVolumeResponse

---

## Purpose

定义 CSI ControllerCreateVolume RPC 的接口规范，用于在华为存储后端（LUN、FileSystem 或 DTree）上创建新卷，支持多种创建模式、池过滤和拓扑约束。

## Requirements

### Requirement: CreateVolume RPC 必须创建存储卷
CreateVolume RPC SHALL 在华为存储后端（LUN、FileSystem 或 DTree）上创建新卷。它支持多种创建模式：正常创建、托管卷（卷导入）、从快照创建和卷克隆。驱动必须验证请求参数，基于能力/拓扑/容量过滤器选择合适的存储池，并处理拓扑约束。

#### Scenario:正常创建新卷
- **WHEN** CO 发送带有唯一名称、有效 CapacityRange（RequiredBytes > 0）、VolumeCapabilities 和可选 StorageClass 参数的 CreateVolumeRequest 时
- **THEN** 驱动验证请求，处理 StorageClass 参数（backend, volumeType, fsType, fsPermission, allocType, hyperMetro, replication, qos, applicationType, storagepool, parentname, reservedSnapshotSpaceRatio, description, volumeName 模板, nfsAutoAuthClient, nfsAutoAuthClientCIDRs, fileSystemMode, disableVerifyCapacity, advancedOptions），通过后端选择器使用能力过滤器（backend, volumeType）、主过滤器（backend, pool, volumeType, allocType, qos, hyperMetro, replication, applicationType, storageQuota, sourceVolumeName, sourceSnapshotName, nfsProtocol）、拓扑过滤器和容量过滤器选择存储池，在华为存储阵列上创建卷，并返回 CreateVolumeResponse，包含 VolumeId（格式："backendName.volumeName"）、CapacityBytes（对齐到扇区大小）、VolumeContext（backend, name, fsPermission, dtreeParentName, disableVerifyCapacity, 可选 lunWWN, 可选 kvcacheStoreId）和 AccessibleTopology

#### Scenario:使用托管（导入）卷创建卷
- **WHEN** CO 发送 CreateVolumeRequest，且 PVC 注释同时包含 manageVolumeName 和 manageBackendName（格式：driverName + "/manageVolumeName" 和 driverName + "/manageBackendName"）时
- **THEN** 驱动在指定后端上查询现有卷，验证 StorageClass 和 PVC 注释之间的后端名称和 volumeType 匹配，验证容量与请求完全匹配，并返回 CreateVolumeResponse 而不在存储阵列上创建新卷

#### Scenario:从快照创建卷
- **WHEN** CO 发送 VolumeContentSource 包含 Snapshot 源的 CreateVolumeRequest 时
- **THEN** 驱动从快照 ID（格式："backendName.parentID.snapshotName"）中提取 sourceBackendName、snapshotParentId 和 sourceSnapshotName，将它们作为参数（sourceSnapshotName, snapshotParentId, backend）传递给后端插件，按 SupportClone 能力过滤池，并从快照数据创建新卷

#### Scenario:从克隆（卷内容源）创建卷
- **WHEN** CO 发送 VolumeContentSource 包含 Volume 源的 CreateVolumeRequest，或在 StorageClass 中带有 "cloneFrom" 参数（格式："backendName.volumeName"）时
- **THEN** 驱动提取 sourceBackendName 和 sourceVolumeName，将它们传递给后端插件（sourceVolumeName, backend），按 SupportClone 能力过滤池，并创建克隆卷

#### Scenario:使用双活（active-active）创建卷
- **WHEN** CO 发送 StorageClass 参数 hyperMetro="true" 的 CreateVolumeRequest 时
- **THEN** 驱动验证 replication 未同时设置为 "true"（互斥），按双活能力过滤本地池，使用辅助过滤函数（volumeType, allocType, qos, replication, applicationType）从 metroBackend 选择远程池，使用后端配置中的 metroDomain 和 metrovStorePairID 创建卷，并返回响应

#### Scenario:使用远程复制创建卷
- **WHEN** CO 发送 StorageClass 参数 replication="true" 的 CreateVolumeRequest 时
- **THEN** 驱动验证 hyperMetro 未同时设置为 "true"（互斥），按远程复制能力过滤本地池，使用辅助过滤函数从本地后端的池（localBackend.Pools）选择远程池，并使用远程复制配置创建卷

#### Scenario:拒绝同时启用双活和远程复制的 CreateVolume
- **WHEN** CO 发送同时设置 hyperMetro="true" 和 replication="true" 的 CreateVolumeRequest 时
- **THEN** 驱动返回错误 "cannot create volume with hyperMetro and replication properties"，并将其包装为 codes.Internal 错误

#### Scenario:使用 allocType thin 或 thick 创建卷
- **WHEN** CO 发送 StorageClass 参数 allocType="thin" 或 allocType="thick" 的 CreateVolumeRequest 时
- **THEN** 驱动分别按 SupportThin 或 SupportThick 能力过滤池；对于厚置备分配，还按池 FreeCapacity >= requestSize 过滤；对于精简分配，跳过 FreeCapacity 检查；创建后，对于厚置备分配，将池的 FreeCapacity 减去 requestSize

#### Scenario:使用 QoS 参数创建卷
- **WHEN** CO 发送 StorageClass 参数 qos 设置为 QoS 规范字符串的 CreateVolumeRequest 时
- **THEN** 驱动按 SupportQoS 能力过滤池，验证 QoS 参数是否符合存储阵列支持，并使用 QoS 配置创建卷

#### Scenario:使用拓扑约束创建卷
- **WHEN** CO 发送带有 AccessibilityRequirements（包含 Requisite 和 Preferred 拓扑）的 CreateVolumeRequest 时
- **THEN** 驱动将拓扑需求处理为 AccessibleTopology 结构（RequisiteTopologies, PreferredTopologies），将它们传递给后端池选择器，按匹配后端 SupportedTopologies 过滤池（包括协议拓扑组合），按首选拓扑顺序对池排序并在每个偏好桶内随机打乱，并基于支持的后端拓扑在响应中返回 AccessibleTopology

#### Scenario:使用 NFS 协议规范创建卷
- **WHEN** CO 发送 VolumeCapabilities 中包含包含 "nfsvers=X"（X 为 3、4、4.0、4.1 或 4.2）的 mountFlags 的 CreateVolumeRequest 时
- **THEN** 驱动将 nfsvers 映射为对应的协议名称（nfs3, nfs4, nfs41, nfs42），将其作为 nfsProtocol 参数传递给后端，并按 SupportNFS3/SupportNFS4/SupportNFS41/SupportNFS42 能力过滤池（DME A 系列池始终通过此过滤器）

#### Scenario:使用 volumeName 模板创建卷
- **WHEN** CO 发送 StorageClass 参数 volumeName 设置为模板字符串的 CreateVolumeRequest 时
- **THEN** 驱动验证模板，从参数中提取元数据（PVCNameKey, PVCNamespaceKey, PVNameKey），使用元数据执行 Go 模板并追加 volumeNameSuffix（"-{{.PVCUid}}"），并使用生成的名称作为存储阵列上的实际卷名称

#### Scenario:使用 parentname（DTree）创建卷
- **WHEN** CO 发送带有 StorageClass 参数 parentname 的 CreateVolumeRequest 时
- **THEN** 驱动验证设置 parentname 时也配置了 backend；parentname 针对后端的 parentname 配置进行验证（如果两者都设置则必须匹配，或其中一个可以为空）；有效的 parentname 用于 DTree 卷操作

#### Scenario:使用 nfsAutoAuthClient 创建卷
- **WHEN** CO 发送 protocol=nfs 且 StorageClass 参数 nfsAutoAuthClient=true 的 CreateVolumeRequest 时
- **THEN** 驱动验证 nfsAutoAuthClientCIDRs（如果提供，必须是有效的 CIDR 格式），并在卷附加期间按 CIDR 过滤节点的主机 IP 以确定授权的 NFS 客户端

#### Scenario:拒绝容量无效的 CreateVolume
- **WHEN** CO 发送缺少或无效 CapacityRange（RequiredBytes <= 0）的 CreateVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，消息为 "CreateVolume CapacityRange must be provided"

#### Scenario:拒绝卷模式与类型不兼容的 CreateVolume
- **WHEN** CO 发送 VolumeMode=Block 但 volumeType=fs 或 volumeType=dtree 的 CreateVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示不匹配

#### Scenario:拒绝在 LUN 文件系统上使用 RWX 访问的 CreateVolume
- **WHEN** CO 发送 volumeType=lun、volumeMode=FileSystem 且 accessMode=ReadWriteMany 的 CreateVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示不支持此组合

#### Scenario:拒绝 fsType 无效的 CreateVolume
- **WHEN** CO 发送 fsType 不是 ext2、ext3、ext4 或 xfs 之一的 CreateVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，包含支持的文件系统类型列表

#### Scenario:拒绝 fsPermission 格式无效的 CreateVolume
- **WHEN** CO 发送 fsPermission 不匹配模式 [0-7][0-7][0-7] 的 CreateVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示格式必须为八进制

#### Scenario:拒绝 description 长度无效的 CreateVolume
- **WHEN** CO 发送 description 参数超过 255 个字符的 CreateVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示长度超过最大值

#### Scenario:拒绝 reservedSnapshotSpaceRatio 无效的 CreateVolume
- **WHEN** CO 发送 reservedSnapshotSpaceRatio 不是整数或不在 [0, 50] 范围内的 CreateVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示值必须在 [0, 50] 范围内

#### Scenario:拒绝 nfsAutoAuthClientCIDRs 格式无效的 CreateVolume
- **WHEN** CO 发送 nfsAutoAuthClientCIDRs 包含无效 CIDR 字符串的 CreateVolumeRequest 时
- **THEN** 驱动在后端配置验证阶段和卷附加阶段均返回错误，指示无效的 CIDR 格式

#### Scenario:拒绝托管卷带内容源的 CreateVolume
- **WHEN** CO 发送托管卷（带有 manageVolumeName 注释）且同时设置了 VolumeContentSource 的 CreateVolumeRequest 时
- **THEN** 驱动返回错误，指示托管卷不能有源内容

#### Scenario:拒绝注释不匹配的托管卷的 CreateVolume
- **WHEN** CO 发送 CreateVolumeRequest，且 PVC 注释中仅存在 manageVolumeName 或 manageBackendName 之一时
- **THEN** 驱动返回 codes.FailedPrecondition 错误，指示两者必须一起配置

#### Scenario:拒绝 filesystemMode 注释无效的 CreateVolume
- **WHEN** CO 发送 PVC 注释 filesystemMode 不是 "HyperMetro" 或 "local" 的 CreateVolumeRequest 时
- **THEN** 驱动返回错误，指示无效的 filesystemMode 值

#### Scenario:拒绝没有存储池匹配需求的 CreateVolume
- **WHEN** CO 发送 CreateVolumeRequest 且没有存储池通过所有过滤阶段（能力、拓扑、容量）时
- **THEN** 驱动返回 codes.Internal 错误，消息为 "no storage pool meets the requirements"，并包含哪个过滤阶段失败的详情

#### Scenario:拒绝后端双活配置不完整的 CreateVolume
- **WHEN** CO 发送 hyperMetro="true" 但后端缺少 metroBackend，或有 metroBackend 但同时缺少 hyperMetroDomain 和 metrovStorePairID 的 CreateVolumeRequest 时
- **THEN** 后端初始化失败，错误指示双活配置不正确

#### Scenario:拒绝 NFS 协议重复的 CreateVolume
- **WHEN** CO 发送 mountFlags 中包含不同 nfsvers 值的多个条目的 CreateVolumeRequest 时
- **THEN** 驱动返回错误，指示 NFS 协议重复

#### Scenario:拒绝不支持的 NFS 协议版本的 CreateVolume
- **WHEN** CO 发送 mountFlags 中包含 nfsvers 不是 3、4、4.0、4.1 或 4.2 的 CreateVolumeRequest 时
- **THEN** 驱动返回错误，指示不支持的 NFS 协议版本

#### Scenario:创建卷时容量对齐到扇区大小
- **WHEN** CO 发送 RequiredBytes 不是存储池扇区大小整数倍的 CreateVolumeRequest 时
- **THEN** 驱动将容量向上舍入到下一个扇区大小倍数，记录容量调整，并在响应中返回实际（对齐的）CapacityBytes

#### Scenario:使用 cloneFrom 参数创建卷
- **WHEN** CO 发送 StorageClass 参数 cloneFrom 设置为 "backendName.volumeName" 的 CreateVolumeRequest 时
- **THEN** 驱动解析 cloneFrom 值以提取 sourceBackendName 和 sourceVolumeName，在参数中设置为 "backend" 和 "cloneFrom"，按 SupportClone 能力过滤池，并创建克隆卷

#### Scenario:使用 storagepool 参数创建卷
- **WHEN** CO 发送 StorageClass 参数 storagepool 设置为特定池名称的 CreateVolumeRequest 时
- **THEN** 驱动将 storagepool 传递给后端，仅选择匹配指定名称的池（如果为空，所有池都通过）

#### Scenario:使用 applicationType 参数创建卷
- **WHEN** CO 发送 StorageClass 参数 applicationType 已设置的 CreateVolumeRequest 时
- **THEN** 驱动按 SupportApplicationType 能力过滤池（仅 Dorado V6/V7 支持），并将 applicationType 传递给插件用于卷创建

#### Scenario:使用 storageQuota 参数创建卷
- **WHEN** CO 发送 StorageClass 参数 storageQuota 已设置的 CreateVolumeRequest 时
- **THEN** 驱动按 SupportQuota 能力过滤池，验证存储配额配置，并使用配额设置创建卷

#### Scenario:使用 volumeName 注释创建卷
- **WHEN** CO 发送 PVC 注释包含 driverName + "/volumeName" 的 CreateVolumeRequest 时
- **THEN** 驱动验证注释值不为空，在参数中将其设置为 "annVolumeName" 供后端插件使用

#### Scenario:创建 DTree 卷时自动选择父级
- **WHEN** CO 发送 volumeType=dtree 但 StorageClass 中未指定 parentname 的 CreateVolumeRequest 时
- **THEN** 将候选池过滤为仅那些 GetDTreeParentName() 非空的池，如果不存在此类池则返回错误 "no found any available dtree backend for volume"

#### Scenario:拒绝 DTree 池没有可用父级的 CreateVolume
- **WHEN** CO 发送 volumeType=dtree 且所有候选 DTree 池的 DTreeParentName 为空的 CreateVolumeRequest 时
- **THEN** 驱动返回错误 "no found any available dtree backend for volume"

#### Scenario:使用 advancedOptions 参数创建卷
- **WHEN** CO 发送 StorageClass 参数 advancedOptions 设置为 JSON 字符串的 CreateVolumeRequest 时
- **THEN** 驱动将 advancedOptions 字符串传递到卷创建参数中，A-Series 插件将 JSON 字符串反序列化为映射并合并到创建模型中，允许将存储特定选项传递给后端插件

#### Scenario:使用快照的 restoreMode 创建卷（SAN 特定）
- **WHEN** CO 发送 VolumeContentSource 包含 Snapshot 源且 StorageClass 参数 restoreMode="snapshot" 的 CreateVolumeRequest 时
- **THEN** 驱动返回快照 LUN 名称而非原始 LUN 名称，对于非双活卷，SAN 插件从快照 LUN 数据创建卷

#### Scenario:使用双活快照的 restoreMode 创建卷
- **WHEN** CO 发送 VolumeContentSource 包含 Snapshot 源、restoreMode="snapshot" 且 hyperMetro="true" 的 CreateVolumeRequest 时
- **THEN** 驱动返回原始 LUN 名称（双活卷不支持快照恢复），并从原始 LUN 创建卷

#### Scenario:池选择后设置创建参数
- **WHEN** CO 发送 CreateVolumeRequest 且存储池选择成功时
- **THEN** 驱动从选择的池设置 storagepool、metroDomain（vStorePairID）和 remoteStoragePool，验证扇区对齐（默认 disableVerifyCapacity=true 跳过验证）

#### Scenario:拒绝池选择后容量对齐验证失败的 CreateVolume
- **WHEN** CO 发送 CreateVolumeRequest 且驱动验证容量对齐时（disableVerifyCapacity=false）发现容量不是扇区大小的倍数时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示容量不是扇区大小的倍数

#### Scenario:创建卷时使用默认 description
- **WHEN** CO 发送不带 description 参数的 CreateVolumeRequest 时
- **THEN** 驱动将 description 设置为默认值 "Created from Kubernetes CSI"

#### Scenario:拒绝 parentname 需要后端的 CreateVolume
- **WHEN** CO 发送设置了 parentname 但未设置 backend 的 CreateVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示 parentname 需要配置 backend

#### Scenario:后端创建卷超时时的 CreateVolume
- **GIVEN** CO 发送 CreateVolumeRequest，存储池选择成功
- **WHEN** 驱动调用后端插件创建卷但超时时
- **THEN** 驱动返回 codes.Internal 错误，包含后端插件返回的超时错误信息，并记录详细错误日志

#### Scenario:卷已存在时的幂等性处理
- **GIVEN** CO 发送 CreateVolumeRequest，卷名称与已存在卷相同
- **WHEN** 请求参数（容量、能力）与已存在卷完全匹配时
- **THEN** 驱动依赖后端插件的幂等性实现返回现有卷信息，不创建新卷
- **WHEN** 请求参数与已存在卷不匹配时
- **THEN** 驱动依赖后端插件返回错误（如 codes.AlreadyExists）；驱动层不显式检查幂等性

#### Scenario:拒绝 parentname 与池 parentname 不匹配的 CreateVolume
- **GIVEN** CO 发送 volumeType=dtree 和 parentname 参数，存储池已选择
- **WHEN** 请求中的 parentname 与所选池配置的 DTreeParentName 不匹配时
- **THEN** 后端插件在卷创建时验证 parentname 匹配性并返回错误

#### Scenario:后端连接失败时的 CreateVolume
- **GIVEN** 存储池已通过过滤阶段被选中
- **WHEN** 驱动调用后端插件创建卷时后端连接失败时
- **THEN** 驱动返回 codes.Internal 错误，包含后端插件返回的连接错误详情

#### Scenario:精简置备容量过滤行为
- **GIVEN** CO 发送 allocType=thin 的 CreateVolumeRequest
- **WHEN** 存储池的 FreeCapacity 小于请求容量时
- **THEN** 驱动在容量过滤阶段不检查 FreeCapacity（精简置备跳过容量检查），所有 SupportThin=true 的池都通过；容量不足由存储阵列在卷创建时返回错误

#### Scenario:拒绝容量不足的 CreateVolume（厚置备）
- **GIVEN** CO 发送 allocType=thick 的 CreateVolumeRequest
- **WHEN** 存储池的 FreeCapacity 小于请求容量时
- **THEN** 驱动在容量过滤阶段排除 FreeCapacity < requestSize 的池；若无池通过容量过滤，返回容量过滤失败错误

#### Scenario:并发 CreateVolume 请求的容量竞争处理
- **GIVEN** 多个 CO 同时发送 CreateVolumeRequest 到同一存储池
- **WHEN** 多个请求的总容量超过池的 FreeCapacity 时
- **THEN** 驱动对厚置备卷扣减 FreeCapacity 但无互斥锁保护，可能存在竞争条件；容量不足的请求由存储阵列在卷创建时返回错误

#### Scenario:卷名称冲突时的 CreateVolume
- **GIVEN** CO 发送 CreateVolumeRequest，卷名称已存在于存储阵列
- **WHEN** 不是托管卷导入场景且卷名冲突时
- **THEN** 驱动层不显式检查卷名冲突，依赖后端插件在卷创建时返回错误

#### Scenario:使用多个后端时的 CreateVolume
- **GIVEN** 存在多个后端，每个后端有多个存储池
- **WHEN** CO 发送 CreateVolumeRequest 且未指定特定后端时
- **THEN** 驱动遍历所有后端的池，应用能力、拓扑和容量过滤器，选择最优池创建卷

#### Scenario:所有候选后端都不可用时的 CreateVolume
- **GIVEN** CO 发送 CreateVolumeRequest
- **WHEN** 所有后端都不可用或所有候选池都被过滤掉时
- **THEN** 驱动返回 codes.Internal 错误，消息为 "no found any available storage pool for volume"

#### Scenario:存储能力检查失败时的 CreateVolume
- **GIVEN** CO 发送带有特定存储能力参数（如 QoS、hyperMetro）的 CreateVolumeRequest
- **WHEN** 后端插件验证能力参数时返回失败时
- **THEN** 驱动返回 codes.Internal 错误，包含能力验证失败的详细原因

#### Scenario:使用 NFS 认证客户端创建卷
- **GIVEN** 后端配置了 nfsAutoAuthClient=true 和 nfsAutoAuthClientCIDRs
- **WHEN** CO 发送 protocol=nfs 的 CreateVolumeRequest 时
- **THEN** 驱动在后端配置阶段验证 CIDR 格式，在卷附加（ControllerPublishVolume）阶段按 CIDR 过滤节点主机 IP 以确定授权的 NFS 客户端

#### Scenario:拒绝 NFS 认证客户端 CIDR 格式无效
- **GIVEN** 后端配置了 nfsAutoAuthClient=true
- **WHEN** nfsAutoAuthClientCIDRs 包含无效 CIDR 字符串时
- **THEN** 驱动在后端配置验证阶段和卷附加阶段均返回错误，指示无效的 CIDR 格式

#### Scenario:使用 fileSystemMode=local 创建 NFS 卷
- **GIVEN** CO 发送 PVC 注释 filesystemMode="local" 的 CreateVolumeRequest
- **WHEN** 后端支持本地文件系统模式时
- **THEN** 驱动将 fileSystemMode 作为参数传递给后端插件，由插件决定文件系统模式行为

#### Scenario:使用 disableVerifyCapacity=true 跳过容量验证
- **GIVEN** CO 发送 CreateVolumeRequest 且 StorageClass 参数 disableVerifyCapacity=true（默认值为 true）
- **WHEN** 请求容量不是存储池扇区大小的整数倍时
- **THEN** 驱动跳过容量对齐验证，接受非对齐的容量请求

#### Scenario:存储阵列返回错误时的 CreateVolume
- **GIVEN** 存储池已选中，后端插件已调用
- **WHEN** 存储阵列返回错误（如 LUN 已满、权限不足、超时等）时
- **THEN** 驱动统一返回 codes.Internal 错误，包含后端插件返回的错误详情

#### Scenario:源快照不可用的 CreateVolume
- **GIVEN** CO 发送 VolumeContentSource 包含 Snapshot 的 CreateVolumeRequest
- **WHEN** 快照 ID 解析后源快照不存在或已被删除时
- **THEN** 后端插件在卷创建时返回错误，驱动返回 codes.Internal 错误

#### Scenario:源卷不可用的 CreateVolume（克隆场景）
- **GIVEN** CO 发送 VolumeContentSource 包含 Volume 或 cloneFrom 参数的 CreateVolumeRequest
- **WHEN** 源卷 ID 解析后源卷不存在或已被删除时
- **THEN** 后端插件在卷创建时返回错误，驱动返回 codes.Internal 错误

#### Scenario:存储池 FreeCapacity 耗尽时的 CreateVolume
- **GIVEN** CO 发送 allocType=thick 的 CreateVolumeRequest
- **WHEN** 所有候选存储池的 FreeCapacity 都小于请求容量时
- **THEN** 驱动排除所有池，返回容量过滤失败错误

#### Scenario:使用预留快照空间创建卷
- **GIVEN** CO 发送 StorageClass 参数 reservedSnapshotSpaceRatio=20 的 CreateVolumeRequest
- **WHEN** 参数值在 [0, 50] 范围内时
- **THEN** 驱动验证参数格式和范围，将 reservedSnapshotSpaceRatio 作为参数传递给后端插件，由插件在卷创建时处理预留空间计算

---

### Requirement: 池过滤应通过能力、拓扑和容量选择存储池
池过滤系统 SHALL 通过多个阶段过滤候选存储池：能力过滤、拓扑过滤和容量过滤，使用可配置的过滤函数。

#### Scenario:按后端名称过滤池
- **WHEN** CreateVolume 请求在 StorageClass 参数中指定后端时
- **THEN** 仅选择属于指定后端的池

#### Scenario:按卷类型过滤池
- **WHEN** CreateVolume 请求指定 volumeType=lun 时
- **THEN** 仅选择 SAN 池（oceanstor-san, fusionstorage-san, oceandisk-san）
- **WHEN** volumeType=fs 时
- **THEN** 仅选择 NAS 池（oceanstor-nas, oceanstor-9000, oceanstor-a-series-nas, oceanstor-a-series-nas-dme, fusionstorage-nas）
- **WHEN** volumeType=dtree 时
- **THEN** 仅选择 DTree 池（oceanstor-dtree, fusionstorage-dtree, oceanstor-a-series-dtree）

#### Scenario:按分配类型过滤池
- **WHEN** CreateVolume 请求指定 allocType=thin 或 allocType 为空时
- **THEN** 仅选择 SupportThin=true 的池（allocType 为空时默认按 thin 过滤）
- **WHEN** allocType=thick 时
- **THEN** 仅选择 SupportThick=true 的池

#### Scenario:按 QoS 过滤池
- **WHEN** CreateVolume 请求指定 QoS 参数时
- **THEN** 仅选择 SupportQoS=true 的池，并针对每个池的插件验证 QoS 参数

#### Scenario:按双活过滤池
- **WHEN** CreateVolume 请求指定 hyperMetro=true 时
- **THEN** 仅选择 SupportMetro=true 且配置了 metroBackend 的池

#### Scenario:按远程复制过滤池
- **WHEN** CreateVolume 请求指定 replication=true 时
- **THEN** 仅选择 SupportReplication=true 且配置了 replicaBackend 的池

#### Scenario:按克隆源过滤池
- **WHEN** CreateVolume 请求带有 VolumeContentSource（快照或卷）时
- **THEN** 仅选择 SupportClone=true 的池

#### Scenario:按 NFS 协议过滤池
- **WHEN** CreateVolume 请求指定 nfsProtocol（nfs3/nfs4/nfs41/nfs42）时
- **THEN** 仅选择具有相应 SupportNFS* 能力的池（DME A 系列池始终通过）

#### Scenario:按拓扑过滤池
- **WHEN** CreateVolume 请求带有包含 Requisite 拓扑的 AccessibilityRequirements 时
- **THEN** 仅选择后端的 SupportedTopologies 匹配至少一个必需拓扑的池，包括协议拓扑组合
- **注意**：当后端没有配置 SupportedTopologies 或 Requisite 列表为空时，过滤器通过所有池

#### Scenario:按容量过滤池
- **WHEN** CreateVolume 请求指定 allocType=thick 时
- **THEN** 仅选择 SupportThick=true 且 FreeCapacity >= requestSize 的池
- **WHEN** allocType=thin 或 allocType 为空时
- **THEN** 所有 SupportThin=true 的池都通过（不检查容量）

#### Scenario:拒绝没有池匹配
- **WHEN** 没有池通过所有过滤阶段时
- **THEN** 系统返回错误 "no storage pool meets the requirements"，并包含哪个过滤阶段失败的详情

#### Scenario:按存储池名称过滤池
- **WHEN** CreateVolume 请求在 StorageClass 参数中指定 storagepool 时
- **THEN** 仅选择匹配指定池名称的池（如果为空字符串，所有池都通过）

#### Scenario:按应用类型过滤池
- **WHEN** CreateVolume 请求在 StorageClass 参数中指定 applicationType 时
- **THEN** 仅选择 SupportApplicationType=true 的池（仅 Dorado V6/V7）；如果 applicationType 为空，所有池都通过

#### Scenario:按存储配额过滤池
- **WHEN** CreateVolume 请求在 StorageClass 参数中指定 storageQuota 时
- **THEN** 仅选择 SupportQuota=true 的池，并验证配额配置；如果配额验证失败则返回错误

#### Scenario:验证托管卷的后端名称
- **WHEN** 处理托管卷（卷导入）请求时
- **THEN** 驱动比较 StorageClass 参数中的后端名称与 PVC 注释的后端名称；如果不同则返回错误

#### Scenario:验证托管卷的卷类型
- **WHEN** 处理托管卷（卷导入）请求时
- **THEN** 驱动验证 StorageClass 中的 volumeType 是否与后端的池存储类型匹配；如果不匹配则返回错误 "filterPools is empty"

#### Scenario:为 DME A 系列按 NFS 协议过滤池
- **WHEN** CreateVolume 请求指定 nfsProtocol 且候选池来自 DME A 系列后端时
- **THEN** 始终让池通过（DME A 系列池绕过 NFS 协议能力检查）

#### Scenario:applicationType 为空时过滤池
- **WHEN** CreateVolume 请求未在 StorageClass 参数中指定 applicationType 时
- **THEN** 所有池都通过，不考虑 SupportApplicationType 能力

#### Scenario:验证托管卷的 authClient
- **WHEN** 为 NFS 协议后端处理托管卷（卷导入）请求时
- **THEN** 驱动仅验证后端名称和卷类型，authClient 配置由后端插件在卷操作时验证

---

### Requirement: 池加权应通过空闲容量选择最优存储池
池加权系统 SHALL 基于空闲容量从过滤后的候选池中选择最优存储池，支持双活和远程复制场景的本地/远程池对选择。

#### Scenario:按最大空闲容量选择池
- **WHEN** 多个池通过所有过滤阶段时
- **THEN** 选择 FreeCapacity 最高的池

#### Scenario:选择本地和远程池对
- **WHEN** 启用双活或远程复制时
- **THEN** 驱动首先按空闲容量选择本地池，然后从池对列表中查找对应的远程池（双活场景从 metroBackend.Pools 选择，远程复制场景从 localBackend.Pools 选择）

#### Scenario:独立选择远程池
- **WHEN** 仅需要远程池选择时
- **THEN** 驱动使用辅助过滤函数（volumeType, allocType, qos, replication, applicationType）过滤远程池，并按空闲容量选择

#### Scenario:拒绝同时使用双活和远程复制
- **WHEN** 同时指定 hyperMetro=true 和 replication=true 时
- **THEN** 系统返回错误 "cannot create volume with hyperMetro and replication properties"

#### Scenario:厚置备分配时递减空闲容量
- **WHEN** 创建厚置备卷时
- **THEN** 驱动将所选池的 FreeCapacity 减去 requestSize 以防止过度分配

#### Scenario:按父名称可用性过滤 DTree 池
- **WHEN** volumeType=dtree 且 StorageClass 中未指定 parentname 时
- **THEN** 将候选池过滤为仅那些 GetDTreeParentName() 非空的池；如果不存在此类池则返回错误

#### Scenario:指定 parentname 时跳过 DTree 池过滤
- **WHEN** volumeType=dtree 且 StorageClass 参数中显式设置了 parentname 时
- **THEN** 返回所有候选池而不进行过滤（父名称验证在其他地方处理）

#### Scenario:为本地和远程选择构建池对
- **WHEN** 启用双活或远程复制时
- **THEN** 驱动遍历每个本地池，选择对应的远程池，构建池对结构（Local + Remote），并将它们传递给加权选择进行最终选择

#### Scenario:远程池未找到匹配时返回 nil
- **WHEN** 为双活或远程复制卷选择远程池但没有远程池通过辅助过滤函数时
- **THEN** 返回 nil——不是错误，允许在没有远程配对的情况下使用本地池

---

### Requirement: 拓扑匹配应将卷请求与后端拓扑匹配
拓扑匹配系统 SHALL 将带有可访问性要求的 CreateVolume 请求与后端支持的拓扑匹配，包括协议特定的拓扑组合。

#### Scenario:匹配必需拓扑
- **WHEN** CreateVolume 请求带有包含 Requisite 拓扑的 AccessibilityRequirements 时
- **THEN** 仅选择后端的 SupportedTopologies 匹配至少一个必需拓扑的池

#### Scenario:按首选拓扑对池排序
- **WHEN** CreateVolume 请求带有 Preferred 拓扑时
- **THEN** 按偏好匹配对池排序，在每个偏好桶内随机打乱池以防止热点

#### Scenario:匹配协议拓扑
- **WHEN** 拓扑需求包含协议键（topology.kubernetes.io/protocol.iscsi）时
- **THEN** 从需求中提取协议拓扑，并检查后端是否通过协议特定的拓扑标签支持它

#### Scenario:向后端添加协议拓扑
- **WHEN** 后端使用协议（例如 iscsi）初始化时
- **THEN** 将协议拓扑组合添加到后端的 SupportedTopologies（例如 topology.kubernetes.io/protocol.iscsi = csi.huawei.com）

#### Scenario:处理没有支持拓扑的后端
- **WHEN** 后端没有配置 SupportedTopologies 时
- **THEN** 拓扑过滤器通过所有池（无拓扑约束）

#### Scenario:处理空的必需拓扑
- **WHEN** CreateVolume 请求带有 AccessibilityRequirements 但 Requisite 列表为空时
- **THEN** 拓扑过滤器通过所有池（无必需约束）

#### Scenario:首选拓扑匹配后打乱剩余池
- **WHEN** 首选拓扑匹配完成后
- **THEN** 未匹配任何偏好的剩余池被随机打乱并追加到有序列表以防止热点

#### Scenario:协议拓扑组合与现有拓扑
- **WHEN** 为已配置 SupportedTopologies 的后端添加协议拓扑时
- **THEN** 通过复制每个现有拓扑并添加协议键（例如 topology.kubernetes.io/protocol.iscsi = csi.huawei.com）来创建协议拓扑组合，然后追加这些组合以及独立的协议拓扑条目

#### Scenario:协议拓扑独立条目
- **WHEN** 为后端添加协议拓扑时
- **THEN** 无论现有拓扑如何，始终添加独立的协议拓扑条目（仅包含协议键和驱动名称作为值的映射）

---

### Requirement: 双活-远程复制应管理 active-active 和远程复制
双活-远程复制系统 SHALL 管理存储池的双活（active-active）和远程复制配置，包括远程池选择和配对管理。

#### Scenario:为双活选择远程池
- **WHEN** CreateVolume 中指定 hyperMetro=true 时
- **THEN** 驱动从缓存加载 metroBackend，使用辅助过滤函数过滤其池，并按空闲容量选择最优远程池

#### Scenario:为远程复制选择远程池
- **WHEN** CreateVolume 中指定 replication=true 时
- **THEN** 驱动从缓存加载本地后端，使用辅助过滤函数过滤本地后端的池（localBackend.Pools），并按空闲容量选择最优远程池

#### Scenario:拒绝没有 metroBackend 的双活
- **WHEN** hyperMetro=true 但后端没有配置 metroBackend 时
- **THEN** 系统返回错误 "no metro backend exists for volume"

#### Scenario:拒绝没有 replicaBackend 的远程复制
- **WHEN** replication=true 且本地后端存在于缓存但 ReplicaBackend 字段为 nil 时
- **THEN** 系统返回错误 "no replica backend exists for volume"

#### Scenario:配置双活域和 vStore 对
- **WHEN** 使用双活创建卷时
- **THEN** 驱动在参数中设置 metroDomain、metrovStorePairID 和 remoteStoragePool 供插件使用

#### Scenario:拒绝配置不完整的双活
- **WHEN** 后端设置了 metroBackend 但缺少 hyperMetroDomain 或 metrovStorePairID，或设置了 hyperMetroDomain/metrovStorePairID 但缺少 metroBackend 时
- **THEN** 驱动返回错误 "hyperMetro configuration in backend <name> is incorrect"

#### Scenario:拒绝 MetroBackend 为 nil 的双活
- **WHEN** hyperMetro=true 但本地后端的 MetroBackend 字段为 nil 时
- **THEN** 返回错误 "no metro backend of <name> exists for volume: <params>"

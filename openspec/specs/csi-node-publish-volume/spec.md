## Purpose

定义 CSI NodePublishVolume RPC 的接口规范，用于在节点目标路径上发布已暂存的卷，支持块卷和文件系统卷模式。

## Requirements

### Requirement: NodePublishVolume RPC 必须将卷发布到节点目标路径
NodePublishVolume RPC SHALL 使暂存卷在节点的目标路径上可用。驱动支持块卷模式（从暂存设备绑定挂载）和文件系统卷模式（从暂存路径绑定挂载或 DTree 卷的 NFS 挂载）。

#### Scenario:将块卷发布到节点
- **WHEN** CO 发送 VolumeCapability.GetBlock() != nil 的 NodePublishVolumeRequest 时
- **THEN** 驱动设置 sourcePath=StagingTargetPath + "/" + VolumeId，执行从暂存源路径到 TargetPath 的裸块设备绑定挂载，使用来自 VolumeCapability 的 mountFlags，成功后返回空的 NodePublishVolumeResponse

#### Scenario:发布文件系统卷（非 DTree）
- **WHEN** CO 发送 VolumeCapability.GetBlock() == nil 的 NodePublishVolumeRequest 且为非 DTree 后端时
- **THEN** 驱动获取后端配置（storage, protocol, portals, metroPortals），设置 sourcePath=StagingTargetPath，构造挂载选项（bind，如果 Readonly 为 true 则加上 "ro"），创建包含 srcType=MountFSType、sourcePath、targetPath、mountFlags、protocol 和 portals 的 connectInfo，使用适当的连接器（NFS 或 NFS+）挂载，并返回空的 NodePublishVolumeResponse

#### Scenario:发布 DTree 文件系统卷
- **WHEN** CO 发送 DTree 存储后端上卷的 NodePublishVolumeRequest 时
- **THEN** 驱动获取后端配置，从 parentName 确定 DTree 源路径（如果 PublishContext.publishInfo 中有 DTreeParentName 则使用，否则从后端配置获取），按协议生成路径前缀（NFS/NFS+ 为 "portal:/"，DPC/DTFS 为 "/"），将 sourcePath 构造为 prefix + parentName + "/" + volumeName，确定挂载选项（来自 VolumeCapability 的 mountFlags，ReadOnly 访问使用 "ro"，A 系列上的 DTFS 使用 "cid=deviceWWN"），并将 DTree 共享挂载到目标路径

#### Scenario:发布 DTree 卷时缺少 parentName
- **WHEN** CO 发送 DTree 卷的 NodePublishVolumeRequest 且无法确定 parentName（不在 PublishContext 中也不在后端配置中）时
- **THEN** 驱动返回 codes.Internal 错误，指示 parentName 缺失，并建议应启用 attachRequired 参数

#### Scenario:拒绝获取后端配置失败的 NodePublishVolume
- **WHEN** CO 发送 NodePublishVolumeRequest 且 GetBackendConfig 失败（后端 ConfigMap 中缺少参数、protocol、portals 或 storage）时
- **THEN** 驱动返回 codes.Internal 错误，包含具体的失败原因

#### Scenario:拒绝挂载失败的 NodePublishVolume
- **WHEN** CO 发送 NodePublishVolumeRequest 且挂载操作失败时
- **THEN** 驱动返回 codes.Internal 错误，包含挂载失败原因

#### Scenario:块卷发布幂等性（目标已是相同设备）
- **WHEN** CO 发送块卷的 NodePublishVolumeRequest 且目标路径已存在为指向相同源设备的设备文件时
- **THEN** 驱动检测到源设备与目标设备相同，跳过绑定挂载并返回成功的 NodePublishVolumeResponse

#### Scenario:块卷发布时目标路径为符号链接
- **WHEN** CO 发送块卷的 NodePublishVolumeRequest 且目标路径是符号链接（V4.6.0 之前的旧格式）时
- **THEN** 驱动移除符号链接，将目标路径重新创建为常规文件，然后执行绑定挂载

#### Scenario:块卷发布时目标路径已挂载
- **WHEN** CO 发送块卷的 NodePublishVolumeRequest 且目标路径已挂载时
- **THEN** 驱动使用 bind 标志重新执行挂载操作覆盖现有挂载

#### Scenario:块卷发布时目标已被不同设备占用
- **WHEN** CO 发送块卷的 NodePublishVolumeRequest 且目标路径已存在为指向不同源设备的设备文件时
- **THEN** 驱动返回错误，指示目标路径已被其他设备占用，包含期望设备路径和实际设备路径信息

#### Scenario:块卷发布时目标不存在创建文件
- **WHEN** CO 发送块卷的 NodePublishVolumeRequest 且目标路径不存在时
- **THEN** 驱动在目标路径创建常规文件（权限 0750），然后执行绑定挂载

#### Scenario:文件系统卷发布幂等性
- **WHEN** CO 发送文件系统卷的 NodePublishVolumeRequest 且目标路径已挂载到相同源路径时
- **THEN** 驱动检测到挂载已存在，跳过挂载操作并返回成功的 NodePublishVolumeResponse

#### Scenario:文件系统卷发布时目标已被不同源占用
- **WHEN** CO 发送文件系统卷的 NodePublishVolumeRequest 且目标路径已挂载到与期望不同的源路径时
- **THEN** 驱动返回错误，指示目标路径已被其他源占用，包含期望源路径和实际源路径信息

#### Scenario:非 DTree 文件系统卷使用 bind mount
- **WHEN** CO 发送非 DTree 文件系统卷的 NodePublishVolumeRequest 时
- **THEN** 驱动将源路径设为 StagingTargetPath，挂载选项包含 "bind"（如果 Readonly 为 true 则加上 "ro"），执行从暂存路径到目标路径的绑定挂载

#### Scenario:DTree 卷使用 NFS 协议直接挂载（非 bind mount）
- **WHEN** CO 发送 DTree 文件系统卷的 NodePublishVolumeRequest 时
- **THEN** 驱动不使用 "bind" 挂载标志，而是构造 NFS/DTFS 源路径（门户+parentName+volumeName），执行从远程共享到目标路径的直接挂载

#### Scenario:DTree 卷 IPv6 门户地址
- **WHEN** CO 发送 DTree 卷的 NodePublishVolumeRequest 且门户地址为 IPv6 格式时
- **THEN** 驱动在源路径中将 IPv6 地址用方括号包裹，格式为 `[IPv6地址]:/parentName/volumeName`

#### Scenario:DTree PublishContext 中 DTreeParentName 覆盖
- **WHEN** CO 发送 DTree 卷的 NodePublishVolumeRequest 且 PublishContext.publishInfo 中包含 DTreeParentName 时
- **THEN** 驱动使用 PublishContext 中的 DTreeParentName 覆盖后端配置中的 dTreeParentName

#### Scenario:NFS+ 多门户 remoteaddrs 参数
- **WHEN** CO 发送 NFS+ 协议卷的 NodePublishVolumeRequest 且存在多个门户时
- **THEN** 驱动将多个门户格式化为 `remoteaddrs=p1~p2~p3` 挂载参数，使用 `mount -t nfs -o remoteaddrs=<portals>,<其他选项>` 挂载

#### Scenario:NFS+ 缺少门户报错
- **WHEN** CO 发送 NFS+ 协议卷的 NodePublishVolumeRequest 且后端配置中缺少 portals 信息时
- **THEN** 驱动返回错误，指示连接信息中缺少门户

#### Scenario:NFS 仅支持单门户
- **WHEN** CO 发送 NFS 协议卷的 NodePublishVolumeRequest 且后端配置了多个门户时
- **THEN** 驱动返回错误，指示 NFS 协议仅支持一个门户

#### Scenario:DPC/DTFS 协议挂载类型设置
- **WHEN** CO 发送 DTree 卷的 NodePublishVolumeRequest 且协议为 dpc 或 dtfs 时
- **THEN** 驱动将挂载命令的文件系统类型参数设置为对应协议类型（`-t dpc` 或 `-t dtfs`）

#### Scenario:DTFS 协议缺少 DeviceWWN 报错
- **WHEN** CO 发送 DTFS 协议卷的 NodePublishVolumeRequest 且后端配置中缺少 DeviceWWN 时
- **THEN** 驱动返回错误，指示使用 DTFS 协议时 DeviceWWN 为空

#### Scenario:NFS+ HyperMetro 门户合并
- **WHEN** CO 发送 NFS+ 协议卷的 NodePublishVolumeRequest 且 PublishContext 中 filesystemMode=HyperMetro 时
- **THEN** 驱动将本地门户和双活门户合并为单个门户列表，用于构造 remoteaddrs 挂载参数

#### Scenario:DTree 卷 ReadOnly 基于 accessMode
- **WHEN** CO 发送 DTree 卷的 NodePublishVolumeRequest 且 VolumeCapability 的 accessMode 为 SINGLE_NODE_READER_ONLY 或 MULTI_NODE_READER_ONLY 时
- **THEN** 驱动在挂载选项中添加 "ro"（与非 DTree 卷基于 req.GetReadonly() 的判断方式不同）

#### Scenario:不支持的协议生成路径前缀报错
- **WHEN** CO 发送 DTree 卷的 NodePublishVolumeRequest 且后端协议不属于 nfs、nfs+、dpc、dtfs 时
- **THEN** 驱动返回错误，指示不支持的协议类型

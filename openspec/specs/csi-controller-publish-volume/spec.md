## Purpose

定义 CSI ControllerPublishVolume RPC 的接口规范，用于在华为存储阵列上将卷附加到节点，支持多种协议（iSCSI、FC、FC-NVMe、RoCE、NVMe、SCSI）和双活配置。

## Requirements

### Requirement: ControllerPublishVolume RPC 必须将卷附加到节点
ControllerPublishVolume RPC SHALL 在华为存储阵列上将卷附加（发布）到特定节点。nodeId 是一个包含节点信息（HostName）的 JSON 编码字符串。驱动必须调用后端的 AttachVolume 操作，并返回包含映射信息（ControllerPublishInfo）和文件系统模式的发布上下文。

#### Scenario:将卷发布到节点（iSCSI 协议）
- **WHEN** CO 发送带有 VolumeId、NodeId（JSON 编码包含 HostName）和可选 VolumeContext 的 ControllerPublishVolumeRequest 时
- **THEN** 驱动分割 VolumeId 获取 backendName 和 volName，解组 NodeId JSON 提取 HostName，将 VolumeContext 添加到参数中，驱动在后端上附加卷，获取包含 TgtPortals、TgtIQNs、TgtHostLUNs 和 TgtLunWWN 的映射信息，将映射信息编组为 publishInfo，确定文件系统模式，并返回 ControllerPublishVolumeResponse，其 PublishContext 包含 "publishInfo"（ControllerPublishInfo 的 JSON 字符串）和 "filesystemMode"

#### Scenario:将卷发布到节点（FC 协议）
- **WHEN** CO 发送 FC 后端上卷的 ControllerPublishVolumeRequest 时
- **THEN** 插件在 publishInfo 中返回包含 TgtLunWWN、TgtWWNs 和 TgtHostLUNs 的 mappingInfo

#### Scenario:将卷发布到节点（FC-NVMe 协议）
- **WHEN** CO 发送 FC-NVMe 后端上卷的 ControllerPublishVolumeRequest 时
- **THEN** 插件在 publishInfo 中返回包含 PortWWNList（PortWWNPair 数组）和 TgtLunGuid 的 mappingInfo

#### Scenario:将卷发布到节点（RoCE/NVMe 协议）
- **WHEN** CO 发送 RoCE 或 NVMe 后端上卷的 ControllerPublishVolumeRequest 时
- **THEN** 插件在 publishInfo 中返回包含 TgtPortals 和 TgtLunGuid 的 mappingInfo

#### Scenario:将卷发布到节点（SCSI 协议）
- **WHEN** CO 发送 SCSI 后端上卷的 ControllerPublishVolumeRequest 时
- **THEN** 插件在 publishInfo 中返回包含 TgtLunWWN 的 mappingInfo

#### Scenario:将卷发布到节点（DTree 存储）
- **WHEN** CO 发送 DTree 存储后端上卷的 ControllerPublishVolumeRequest 时
- **THEN** 插件返回包含 DTreeParentName 的 mappingInfo，该信息被包装在 DTreePublishInfo 中并通过 PublishContext 传递

#### Scenario:后端不存在时发布卷
- **WHEN** CO 发送 VolumeId 引用不存在后端的 ControllerPublishVolumeRequest 时
- **THEN** 驱动返回 codes.Internal 错误，指示后端不存在

#### Scenario:使用 NfsPlus 协议发布卷
- **WHEN** CO 发送 protocol=nfs+ 后端上卷的 ControllerPublishVolumeRequest（且不是 DTree 存储）时
- **THEN** 驱动从后端查询卷以获取其文件系统模式（本地或双活），并将其包含在 PublishContext 中

#### Scenario:拒绝 nodeId JSON 无效的 ControllerPublishVolume
- **WHEN** CO 发送 NodeId 无法解组为 JSON 的 ControllerPublishVolumeRequest 时
- **THEN** 驱动返回 codes.Internal 错误，包含解组错误消息

#### Scenario:发布卷时合并双活双站点映射信息
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerPublishVolumeRequest，且本地和远程存储均在线时
- **THEN** metroHandler 调用 MetroAttacher.ControllerAttach，合并来自两个站点的映射信息：对于 iSCSI，追加 tgtPortals、tgtIQNs 和 tgtHostLUNs 数组；对于 FC，追加 tgtWWNs 和 tgtHostLUNs 数组，返回组合的映射信息用于多路径访问

#### Scenario:发布卷时双活仅本地回退
- **WHEN** CO 发送双活卷的 ControllerPublishVolumeRequest，且仅本地存储在线（远程离线）时
- **THEN** 处理方法回退到使用本地客户端的 commonHandler，仅返回本地站点映射信息

#### Scenario:发布卷时双活仅远程回退
- **WHEN** CO 发送双活卷的 ControllerPublishVolumeRequest，且仅远程存储在线（本地离线）时
- **THEN** 处理方法回退到使用远程客户端的 commonHandler，仅返回远程站点映射信息

#### Scenario:拒绝启动器与另一主机冲突的 ControllerPublishVolume
- **WHEN** CO 发送 ControllerPublishVolumeRequest 且后端插件检测到启动器（iSCSI IQN 或 FC WWPN）已与存储阵列上的不同主机关联时
- **THEN** 插件返回错误，指示启动器已被另一主机使用

#### Scenario:发布卷时更新 ALUA 配置
- **WHEN** CO 发送 ControllerPublishVolumeRequest 且附加器检测到主机或启动器 ALUA 配置与当前存储阵列配置不同时
- **THEN** 附加器在完成附加操作之前更新启动器 ALUA 设置和/或主机 ALUA 设置

#### Scenario:发布卷时创建主机组和映射组
- **WHEN** CO 发送 ControllerPublishVolumeRequest 且主机在存储阵列上尚不存在时
- **THEN** 附加器创建主机，添加启动器，创建主机组，创建 LUN 与主机组之间的映射，并创建命名空间组（对于 Oceandisk），返回映射信息

#### Scenario:发布卷时使用 NFS 自动认证客户端 CIDR 过滤
- **WHEN** CO 发送启用 nfsAutoAuthClient 的 DTree 或 NAS 卷的 ControllerPublishVolumeRequest 时
- **THEN** 驱动从 Secret 获取节点的主机 IP，按配置的 CIDR 过滤它们，并仅将匹配的 IP 添加为具有 ReadWrite 访问权限的授权 NFS 客户端

#### Scenario:拒绝本地和远程存储均离线的 ControllerPublishVolume
- **WHEN** CO 发送双活卷的 ControllerPublishVolumeRequest 且本地和远程存储均离线时
- **THEN** 驱动返回错误 "both local and remote storage not online"

#### Scenario:拒绝 VolumeId 为空的 ControllerPublishVolume
- **WHEN** CO 发送 VolumeId 为空的 ControllerPublishVolumeRequest 时
- **THEN** 驱动返回 codes.InvalidArgument 错误，指示 VolumeId 缺失

#### Scenario:拒绝 VolumeId 格式无效的 ControllerPublishVolume
- **WHEN** CO 发送 VolumeId 不包含 "." 分隔符（无法解析出 volName）的 ControllerPublishVolumeRequest 时
- **THEN** 驱动解析 VolumeId 后卷名为空，后端附加操作将失败，驱动返回错误

#### Scenario:拒绝 NodeId 为空的 ControllerPublishVolume
- **WHEN** CO 发送 NodeId 为空的 ControllerPublishVolumeRequest 时
- **THEN** 驱动无法解组 NodeId JSON 提取 HostName，返回错误

#### Scenario:后端插件附加卷失败
- **WHEN** CO 发送 ControllerPublishVolumeRequest 且后端附加卷操作返回错误时
- **THEN** 驱动返回 codes.Internal 错误，包含插件返回的错误消息

#### Scenario:拒绝双活 pair 处于暂停状态的 ControllerAttach
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerPublishVolumeRequest，且 HyperMetro pair 的 RUNNINGSTATUS 为暂停状态（非正常）时
- **THEN** 驱动拒绝附加操作，返回错误指示双活 pair 状态异常（仅 RUNNINGSTATUS 为正常状态 "1" 时才允许 ControllerAttach，暂停状态 "41" 仅允许 Detach/Unstage）

#### Scenario:双活远端附加成功但本地附加失败
- **WHEN** CO 发送启用双活的 OceanStor SAN 后端上卷的 ControllerPublishVolumeRequest，且 MetroAttacher 远端站点附加成功但本地站点附加失败时
- **THEN** 驱动返回错误，且不回滚远端站点的附加操作（远端 LUN 映射保留在存储阵列上）

#### Scenario:NFS+ 查询文件系统模式失败时返回空值
- **WHEN** CO 发送 NFS+ 协议后端上卷的 ControllerPublishVolumeRequest，且查询卷信息失败时
- **THEN** 驱动记录错误日志，返回的 PublishContext 中 filesystemMode 为空字符串

#### Scenario:FC 启动器不在线时跳过
- **WHEN** CO 发送 FC 协议后端上卷的 ControllerPublishVolumeRequest，且部分 FC WWPN 启动器的运行状态不为在线时
- **THEN** 附加器跳过不在线的启动器，仅使用在线的启动器完成附加操作

#### Scenario:OceanDisk 命名空间不存在时发布卷失败
- **WHEN** CO 发送 OceanDisk SAN 后端上卷的 ControllerPublishVolumeRequest，且存储阵列上不存在对应命名空间时
- **THEN** 插件返回错误，指示命名空间不存在

#### Scenario:发布卷时处理 iSCSI CHAP 认证
- **WHEN** CO 发送 iSCSI 协议后端上卷的 ControllerPublishVolumeRequest，且存储配置了 CHAP 认证时
- **THEN** 附加器在建立 iSCSI 会话时使用 CHAP 用户名和密码进行认证

#### Scenario:DTree 卷使用 NFS 自动认证客户端
- **WHEN** CO 发送启用 nfsAutoAuthClient 的 DTree 存储后端上卷的 ControllerPublishVolumeRequest 时
- **THEN** 驱动从 Secret 获取节点主机 IP，按配置的 CIDR 过滤，将匹配的 IP 添加为 DTree 卷的授权 NFS 客户端

#### Scenario:FusionStorage SAN 发布卷
- **WHEN** CO 发送 FusionStorage SAN 后端上卷的 ControllerPublishVolumeRequest 时
- **THEN** 驱动使用 FusionStorage 专用附加器创建附加映射，附加器返回包含 tgtLunWWN 的映射信息

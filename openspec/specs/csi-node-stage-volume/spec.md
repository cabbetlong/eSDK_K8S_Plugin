## Purpose

定义 CSI NodeStageVolume RPC 的接口规范，用于在节点上暂存卷以供使用，支持 SAN 和 NAS 存储类型的连接和挂载操作。

## Requirements

### Requirement: NodeStageVolume RPC 必须在节点上暂存卷
NodeStageVolume RPC SHALL 通过将卷暂存到暂存目标路径来准备卷以供节点使用。驱动创建 manage.Manager（根据后端协议为 SanManager 或 NasManager）并将暂存操作委托给它。SAN 暂存涉及通过适当的协议连接器（iSCSI、FC、FC-NVMe、RoCE、NVMe、SCSI）将卷连接到主机，而 NAS 暂存涉及挂载 NFS/NFS+/DPC/DTFS 共享。

#### Scenario:暂存 SAN 卷（iSCSI/FC/FC-NVMe/RoCE/NVMe/SCSI）
- **WHEN** CO 发送带有 VolumeId、StagingTargetPath、VolumeCapability、PublishContext（包含 publishInfo）和 VolumeContext 的 NodeStageVolumeRequest 时
- **THEN** 驱动分割 VolumeId 获取 backendName，为后端协议创建 SanManager，使用 WithProtocol、WithConnector、WithVolumeCapability、WithControllerPublishInfo 和 WithMultiPathType 构建参数，执行暂存流程：清理残留路径、连接卷、根据卷模式暂存（块或文件系统）、持久化 WWN，成功后返回空的 NodeStageVolumeResponse

#### Scenario:暂存 SAN 块卷
- **WHEN** CO 发送 VolumeCapability.GetBlock() != nil 的 NodeStageVolumeRequest 时
- **THEN** 驱动在参数中设置 volumeMode="Block" 和 stagingPath=StagingTargetPath + "/" + VolumeId，执行 stageForBlock 任务在暂存路径创建块设备，并将 WWN 保存到磁盘

#### Scenario:暂存 SAN 文件系统卷
- **WHEN** CO 发送 VolumeCapability.GetMount() != nil 的 NodeStageVolumeRequest 时
- **THEN** 驱动从挂载能力中提取 fsType，验证 fsType（如果指定必须是 ext2、ext3、ext4 或 xfs），提取 mountFlags，确定 accessMode（ReadOnly 添加 "ro" 标志），在参数中设置 targetPath=StagingTargetPath、fsType、mountFlags 和 accessMode，执行 stageForMount 任务创建文件系统并挂载，并将 WWN 保存到磁盘

#### Scenario:暂存 NAS 卷（NFS/NFS+/DPC/DTFS）
- **WHEN** CO 发送 NAS 后端（协议：nfs、nfs+、dpc、dtfs）上卷的 NodeStageVolumeRequest 时
- **THEN** 驱动创建 NasManager，使用 WithProtocol、WithPortals、WithVolumeCapability 和 WithDeviceWWN 构建参数；对于 DTree 存储，跳过暂存（立即返回）；对于其他 NAS 类型，从协议和门户生成 sourcePath（例如 NFS 为 "portal:/"，DPC/DTFS 为 "/"），与 volumeName 拼接，并将共享挂载到暂存目标路径

#### Scenario:使用双活文件系统模式暂存 NAS 卷
- **WHEN** CO 发送 NAS 卷的 NodeStageVolumeRequest，且 PublishContext 中 protocol=nfs+ 且 filesystemMode=HyperMetro 时
- **THEN** 驱动将本地门户和双活门户合并为单个门户列表用于挂载操作

#### Scenario:使用设备 WWN 暂存 DTFS 卷
- **WHEN** CO 发送 DTFS 协议上带有 deviceWWN 的卷的 NodeStageVolumeRequest 时
- **THEN** 驱动在挂载操作的 mountFlags 中添加 "cid=deviceWWN"

#### Scenario:拒绝 publishInfo 缺失且自动附加失败的 NodeStageVolume
- **WHEN** CO 发送 PublishContext 中不带 publishInfo 的 NodeStageVolumeRequest 且自动附加操作失败（后端离线、构建后端失败、获取主机名失败、附加失败或编组失败）时
- **THEN** 驱动返回 codes.Internal 错误，包含具体的失败原因

#### Scenario:拒绝 fsType 无效的 NodeStageVolume
- **WHEN** CO 发送 fsType 不是 ext2、ext3、ext4 或 xfs 之一的 NodeStageVolumeRequest（对于挂载类型）时
- **THEN** 驱动返回 codes.Internal 错误，包含支持的文件系统类型列表

#### Scenario:拒绝卷能力无效的 NodeStageVolume
- **WHEN** CO 发送 VolumeCapability 既不是 Block 也不是 Mount 类型的 NodeStageVolumeRequest 时
- **THEN** 驱动返回 codes.Internal 错误，指示卷能力无效

#### Scenario:publishInfo 缺失时自动附加（后端离线）
- **WHEN** CO 发送不带 publishInfo 的 NodeStageVolumeRequest 且 StorageBackendContent 状态为 nil 或 Online=false 时
- **THEN** 驱动返回错误 "attach volume failed cause backend offline, backend name: <name>"，返回 codes.Internal

#### Scenario:publishInfo 缺失时自动附加（构建后端失败）
- **WHEN** CO 发送不带 publishInfo 的 NodeStageVolumeRequest 且 backend.BuildBackend 失败（配置无效、缺少凭据、插件初始化失败）时
- **THEN** 驱动返回错误 "attach volume failed while building backend, backend name: <name>, err: <error>"，返回 codes.Internal

#### Scenario:publishInfo 缺失时自动附加（获取主机名失败）
- **WHEN** CO 发送不带 publishInfo 的 NodeStageVolumeRequest 且无法获取主机名时
- **THEN** 驱动返回错误 "attach volume failed while getting hostname, err: <error>"，返回 codes.Internal

#### Scenario:publishInfo 缺失时自动附加（附加到存储阵列失败）
- **WHEN** CO 发送不带 publishInfo 的 NodeStageVolumeRequest 且存储阵列附加操作失败时
- **THEN** 驱动返回错误 "attach volume failed while attaching volume, volume name: <name>, err: <error>"，返回 codes.Internal

#### Scenario:仅对 UltraPath 使用 LUN ID 清理残留路径
- **WHEN** SanManager StageVolume 运行 clearResidualPathWithLunId 任务时
- **THEN** 当使用 UltraPath 多路径且协议为 iSCSI 或 FC 时，在连接之前按 LUN ID 清理过时设备

#### Scenario:暂存卷时检查 NVMe CLI 版本
- **WHEN** CO 发送 NVMe 协议后端上卷的 NodeStageVolumeRequest 时
- **THEN** NVMe 连接器验证已安装的 nvme-cli 版本 >= 1.9 后再继续连接；如果版本过低则返回错误

#### Scenario:暂存 SAN 文件系统卷时使用 XFS nouuid 挂载选项
- **WHEN** CO 发送 fsType=xfs 的 SAN 文件系统卷的 NodeStageVolumeRequest 时
- **THEN** 驱动在挂载 XFS 文件系统时自动添加 "nouuid" 挂载选项，以允许在同一节点上挂载具有相同 UUID 的克隆卷

#### Scenario:暂存块卷时使用旧版符号链接处理
- **WHEN** CO 发送块卷的 NodeStageVolumeRequest 且暂存目标路径是符号链接（V4.6.0 之前的旧格式）时
- **THEN** 驱动移除符号链接并将目标路径重新创建为常规文件，然后执行绑定挂载

#### Scenario:暂存 SAN 文件系统卷时使用基于磁盘大小的 mkfs 策略
- **WHEN** CO 发送未格式化块设备上 SAN 文件系统卷的 NodeStageVolumeRequest 时
- **THEN** 驱动根据设备大小确定格式化模板：<=0.5TiB="default"，0.5-1TiB="big"，1-10TiB="huge"，10-100TiB="large"，100-512TiB="veryLarge"，>512TiB 返回错误；驱动应用相应的格式化模板（例如 ext 文件系统使用 "-T big"）

#### Scenario:拒绝磁盘大小超出最大值的暂存卷
- **WHEN** CO 发送大于 512TiB 的未格式化块设备上 SAN 文件系统卷的 NodeStageVolumeRequest 时
- **THEN** 驱动返回错误 "the disk size does not support"

#### Scenario:暂存 SAN 文件系统卷时检测并发格式化
- **WHEN** CO 发送 SAN 文件系统卷的 NodeStageVolumeRequest 且另一个进程已经在格式化该设备时
- **THEN** 驱动在格式化输出中检测到设备正在使用，休眠 10 秒，并返回错误

#### Scenario:暂存 SAN 文件系统卷时跳过分区设备
- **WHEN** 驱动扫描设备路径时
- **THEN** 分区设备（例如 sdc1、nvme0n1p1、dm-1 带尾随数字）在残留路径检测期间被显式跳过

#### Scenario:暂存 SAN 文件系统卷时处理 blkid 退出码 2
- **WHEN** CO 发送 SAN 文件系统卷的 NodeStageVolumeRequest 且 blkid 返回退出码 2 时
- **THEN** 驱动验证设备是否确实已格式化；如果已格式化但 blkid 失败，则返回模糊错误

#### Scenario:暂存 SAN 文件系统卷时挂载后扩容
- **WHEN** CO 发送已格式化设备上 SAN 文件系统卷的 NodeStageVolumeRequest 且 accessMode 不是 MULTI_NODE_MULTI_WRITER 或 MULTI_NODE_READER_ONLY 时
- **THEN** 挂载后扩容文件系统（ext* 使用 resize2fs，xfs 使用 xfs_growfs）

#### Scenario:多节点访问时暂存 SAN 文件系统卷跳过扩容
- **WHEN** CO 发送 accessMode=MULTI_NODE_MULTI_WRITER 或 MULTI_NODE_READER_ONLY 的 SAN 文件系统卷的 NodeStageVolumeRequest 时
- **THEN** 挂载后跳过扩容步骤以防止与其他节点冲突

#### Scenario:使用 DPC/DTFS 协议挂载选项暂存卷
- **WHEN** CO 发送 protocol=dpc 或 protocol=dtfs 的 NAS 卷的 NodeStageVolumeRequest 时
- **THEN** 驱动将挂载命令的文件系统类型设置为对应协议类型

#### Scenario:重复暂存同一卷（幂等性）
- **WHEN** CO 对已暂存到相同路径的卷再次发送 NodeStageVolumeRequest 时
- **THEN** 驱动检测到目标路径已挂载到相同源路径，跳过挂载操作并返回成功的 NodeStageVolumeResponse

#### Scenario:并发暂存同一卷
- **WHEN** 同一节点上多个 Pod 同时请求暂存同一卷（相同 WWN）时
- **THEN** 驱动通过 WWN 级同步锁串行化连接操作，确保只有一个协程执行卷连接，其他等待

#### Scenario:并发 iSCSI Portal 连接
- **WHEN** 多个卷共享同一 iSCSI Portal:IQN 对且同时发起连接时
- **THEN** 驱动防止重复登录同一 Portal:IQN 对，后续请求复用已有连接

#### Scenario:iSCSI 登录重试
- **WHEN** CO 发送 iSCSI 后端上卷的 NodeStageVolumeRequest 且 iSCSI Session 登录失败时
- **THEN** 驱动自动重试登录最多 60 次，每次间隔 2 秒；若全部失败则返回错误

#### Scenario:iSCSI CHAP 认证
- **WHEN** CO 发送 iSCSI 后端上卷的 NodeStageVolumeRequest 且后端配置了 CHAP 认证时
- **THEN** 驱动在 iSCSI 登录前设置 CHAP 用户名和密码参数，完成安全认证后建立连接

#### Scenario:iSCSI 手动扫描模式
- **WHEN** CO 发送 iSCSI 后端上卷的 NodeStageVolumeRequest 时
- **THEN** 驱动在 iSCSI 登录后设置 `node.session.scan=manual` 以防止内核自动扫描干扰设备发现

#### Scenario:iSCSI 自动启动
- **WHEN** CO 发送 iSCSI 后端上卷的 NodeStageVolumeRequest 且 iSCSI 登录成功时
- **THEN** 驱动设置 `node.startup=automatic` 确保节点重启后 iSCSI 会话自动恢复

#### Scenario:设备扫描等待
- **WHEN** CO 发送 SAN 卷的 NodeStageVolumeRequest 且卷连接成功但设备尚未出现在系统中时
- **THEN** 驱动以指数退避方式轮询等待设备出现，iSCSI 最多等待约 30 秒，FC 最多等待约 60 秒；超时则返回错误

#### Scenario:DM-Multipath 设备验证
- **WHEN** CO 发送使用 DM-Multipath 的 SAN 卷的 NodeStageVolumeRequest 且设备连接成功时
- **THEN** 驱动轮询 multipathd 确认 DM 设备映射已建立且设备可用，超时则返回错误

#### Scenario:UltraPath 设备验证
- **WHEN** CO 发送使用 UltraPath 的 SAN 卷的 NodeStageVolumeRequest 且设备连接成功时
- **THEN** 驱动通过 `upadmin show vlun` 验证磁盘状态为正常，异常则返回错误

#### Scenario:FC HBA 端口在线检查
- **WHEN** CO 发送 FC 后端上卷的 NodeStageVolumeRequest 时
- **THEN** 驱动读取 `/sys/class/fc_host/host*/port_state`，仅当端口状态为 "Online" 时继续连接；若所有端口均不在线则返回错误

#### Scenario:块设备可读性验证
- **WHEN** CO 发送 SAN 文件系统卷的 NodeStageVolumeRequest 且卷已连接到主机时
- **THEN** 驱动在挂载前读取块设备 512KiB 数据验证设备可读，不可读则返回错误

#### Scenario:IPv6 NFS 门户地址
- **WHEN** CO 发送 NAS 卷的 NodeStageVolumeRequest 且门户地址为 IPv6 格式时
- **THEN** 驱动在源路径中将 IPv6 地址用方括号包裹，格式为 `[IPv6地址]:/volumeName`

#### Scenario:NFS+ 多门户挂载参数
- **WHEN** CO 发送 protocol=nfs+ 的 NAS 卷的 NodeStageVolumeRequest 且存在多个门户时
- **THEN** 驱动将多个门户格式化为 `remoteaddrs=p1~p2~p3` 挂载参数，使用 `mount -t nfs -o remoteaddrs=<portals>,<其他选项>` 挂载

#### Scenario:NFS+ 缺少门户报错
- **WHEN** CO 发送 protocol=nfs+ 的 NAS 卷的 NodeStageVolumeRequest 且 PublishContext 中缺少 portals 信息时
- **THEN** 驱动返回错误，指示 NFS+ 协议要求 portals 字段

#### Scenario:NFS 仅支持单门户
- **WHEN** CO 发送 protocol=nfs 的 NAS 卷的 NodeStageVolumeRequest 且后端配置了多个门户时
- **THEN** 驱动返回错误，指示 NFS 协议仅支持一个门户

#### Scenario:挂载目标已被不同源占用
- **WHEN** CO 发送 NodeStageVolumeRequest 且目标路径已挂载到与期望不同的源路径时
- **THEN** 驱动返回错误，指示目标路径已被其他源占用，包含当前源路径和期望源路径信息

#### Scenario:不支持的 NAS 协议
- **WHEN** CO 发送 NodeStageVolumeRequest 且后端协议不属于 nfs、nfs+、dpc、dtfs 时
- **THEN** 驱动返回错误，指示不支持的 NAS 协议类型

#### Scenario:volumeId 格式无效导致卷名为空
- **WHEN** CO 发送 NodeStageVolumeRequest 且 VolumeId 分割后卷名部分为空时
- **THEN** 驱动返回错误，指示卷名为空并包含原始 VolumeId

#### Scenario:SAN 文件系统卷设置文件系统权限
- **WHEN** CO 发送 SAN 文件系统卷的 NodeStageVolumeRequest 且 VolumeContext 中包含 fsPermission 时
- **THEN** 驱动在挂载完成后根据 fsPermission 值设置目标路径的目录权限

#### Scenario:块卷模式跳过 WWN 残留路径清理
- **WHEN** CO 发送 volumeMode=Block 的 SAN 卷的 NodeStageVolumeRequest 时
- **THEN** 驱动检测到 Block 模式后直接跳过，不执行残留路径清理

#### Scenario:WWN 持久化保存
- **WHEN** CO 发送 SAN 卷的 NodeStageVolumeRequest 且暂存流程全部成功时
- **THEN** 驱动将卷的 WWN 写入 `/csi/disks/<volumeId>.wwn` 文件，供后续 NodeUnstageVolume 在缺少 PublishContext 时识别卷

#### Scenario:WWN 文件名截断
- **WHEN** CO 发送 SAN 卷的 NodeStageVolumeRequest 且 VolumeId 长度超过 64 字符时
- **THEN** 驱动截取 VolumeId 末尾 64 字符作为 WWN 文件名

#### Scenario:DM 设备刷盘重试
- **WHEN** CO 发送使用 DM-Multipath 的 SAN 卷的 NodeStageVolumeRequest 且清理残留路径时 DM 设备 Flush 失败时
- **THEN** 驱动重试 Flush 操作最多 3 次，每次间隔 20 秒；全部失败则返回错误

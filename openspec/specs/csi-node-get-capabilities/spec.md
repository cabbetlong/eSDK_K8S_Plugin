## Purpose

定义 CSI NodeGetCapabilities RPC 的接口规范，用于通告驱动支持的节点服务能力，包括暂存、扩容和卷统计功能。

## Requirements

### Requirement: NodeGetCapabilities RPC 必须通告节点能力
NodeGetCapabilities RPC SHALL 通告驱动支持的节点服务能力。驱动支持：STAGE_UNSTAGE_VOLUME、EXPAND_VOLUME 和 GET_VOLUME_STATS。

#### Scenario:CO 查询节点能力
- **WHEN** CO 发送 NodeGetCapabilitiesRequest 时
- **THEN** 驱动返回 NodeGetCapabilitiesResponse，包含能力：STAGE_UNSTAGE_VOLUME、EXPAND_VOLUME 和 GET_VOLUME_STATS

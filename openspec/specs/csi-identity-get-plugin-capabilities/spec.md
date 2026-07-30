## Purpose

定义 CSI GetPluginCapabilities RPC 的接口规范，用于通告驱动支持的插件能力，包括控制器服务和拓扑约束功能。

## Requirements

### Requirement: GetPluginCapabilities RPC 必须通告驱动能力
GetPluginCapabilities RPC SHALL 通告驱动支持 CONTROLLER_SERVICE 和 VOLUME_ACCESSIBILITY_CONSTRAINTS 插件能力，使 CO 能够理解驱动提供哪些功能。

#### Scenario:CO 查询插件能力
- **WHEN** CO 发送 GetPluginCapabilitiesRequest 时
- **THEN** 驱动返回的能力包括 CONTROLLER_SERVICE（指示控制器服务可用）和 VOLUME_ACCESSIBILITY_CONSTRAINTS（指示支持拓扑感知的卷放置）

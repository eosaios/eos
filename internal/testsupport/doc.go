// Package testsupport 提供 EOS CLI 测试底座。
//
// 设计约束（对应 eos-docs/eos-cli/testing/TEST-STRATEGY.md §5）：
//
//  1. 全部数据落在测试自建的临时目录，测试不得读写真实用户配置、
//     凭据或会话。凭据类环境变量在继承时主动剔除。
//  2. 真实 Core 通过 pkg/coreapi/sidecar 的 resolver 定位，记录实际路径、
//     sha256 与 manifest，测试报告里必须带上这三个值。
//  3. 假 Core（fake peer）只能证明 Go 装配链，不能替代真实 Core 集成；
//     提供者身份由 NewFakeCore 显式标注，不得用来满足 P0 契约用例。
//  4. 异步等待一律用屏障 / 条件等待 + deadline，禁止裸 sleep 兜底。
//
// 本包只服务于 *_test.go，不进入任何分发产物。
package testsupport

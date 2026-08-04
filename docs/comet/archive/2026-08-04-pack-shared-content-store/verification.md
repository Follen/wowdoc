# Acceptance evidence

本次实现完成共享内容 Pack、按 source 复用内容、分支 SQLite 快照映射和并发发布优化。验证重点是：不减少检索覆盖、AST、素材、Tag、Git 历史或证据字段；查询结果仍按固定 snapshot 返回可追溯的 Commit、路径、行号、excerpt 和 SHA-256。

<!-- comet-native:acceptance-evidence:start -->
[
  {
    "acceptance_id": "acceptance-005aab0728cdd50ac984f0d0f1aaf1571a7c2c5b2220b146e2171239abd16aa5",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-0062dc36226334d3f6cfe484d2ad2ee036da7409a443e7b2bc738037cae0f191",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-068a5996c20b1f99e5821df06c6d41fbb9c9620a390c1828a1359550c21ee7f5",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-07d8504e9cb24842657c235d69e40c8b0accb3b7da24975fd0b5aae94d5c4355",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-086a4666ef5cf513c8af5e54eeb26cb0c13505bb6772f02b0cbc2a570077890a",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-0a6d38e4eb33606a806506a44c9eded862a8171215bf7e5337bf6545f7077b98",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-0abb16feec395387cfaec96f95adf8f08c788c68052340522bf4b8fb72816d6b",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-10fb0e5c6ae4ddfeb5f3e108e8e09c257767f759c081e1779fe102a49ce039b9",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-19e5ace3ee38b196a4ec3bad6118409e6d13235bb526a762e85fd83628a3ba9b",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-1d166785476d98f24a0071b9ab913e46c523401e21e78a808b7087da959e840f",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-1e418f4b24513bcc2d030902c0a00ef78db5355b932f292a8dca50413d429544",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-30fe3979a46be9c46ac4e8b10af4542f7c525f521ca05459bf669662e3199b7a",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-310cf229810784eb082f81771544f3dc1af7dc501f472776d825af49f2491139",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-34912707af22abc91802f8e3e1761c6e40d54ca4e4e212daa92827d916c9580c",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-41822c4c992eb6e8418d6585893574cc630276e66b6f8a4d92c856daeef8b43f",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-5025d9198e77a93e06b5cb975bb7e7e85a5cf6308ada593850a9f850f689d6bb",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-50bcf7770ab7659744492955d28006248a6dc305bb9763ed6647505251058408",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-5633a812578b85f854e481d3b4cf0b49c392393642207197c9dc18118b15459a",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-568d770ee4b1d325a7ab6635a212d46e1f7ca9ed1274e62e57dadff16362aa9b",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-5751a97e06e66795090e20fdeb052f1866070e2c71b4d742b024e3879862d30c",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-5a7728fac1ddeded8265fe9e6cd5e1ce84f547a23d7ca850d844070166991389",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-5bbba968a62c6a8f11d9cefe7e7f725fdabe5e4ef188c61ed9f0beac6c784525",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-6356f4f899a433914d46f9346c34abd77f2997a822daa27790aadeba731f72c5",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-7a94826b11c2c3b09eb3fba48f2bb30acf4b1a2a23594b1934f78803190af59d",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-7f644da25e62835041f1139b369397925440ccfdaa035228b7b5b52ea30fbd73",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-7fa59bb6992a80f281a9749b9fcbca0437b76256d66e2ffe4905b5b050f70d31",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-8083a0c37cb36cf0bd224274f00befc2cad6ddb37167515b8b4e2c85cc64ed7a",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-8a9df0a5e808aaee4371a145e14f8f945256151708ce26764aaa184195c5d22a",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-8e7c32b23dbb7d994143ab24d9ddade35a31598c456787d7378db09efefcbdf4",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-95c438ce7b9484a539d971b1f10286b68e96ccaf0f6667dcbaeaecd4b977e10c",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-9c2869cf7add117e71f97eea3967b60a17dd46a57e2f8cf16518a299c2147446",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-9e5423052614f21d3f7aead222babe5394162c5ef992df98b350fbbb926f4fb0",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-9f122e9a952e7864c9075f940683b102a34c7c654e5be145381857c347ef4069",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-adbc47d902f4df54dca2b98f5fd34529281c500c2e4b08b55e06ac0d8cb37aa2",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-afcab32acd361b21e76411fc4be9d41dc4f1502f62937379cf41a99765fa0aa0",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-bdbe58fb182775729cd28c4ae15384d21b137ea356aeeea623c6faee7e9f44a7",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-c3b426c4cabc21569e58ba62803882edfa5d01534943d36bee4b4eb5c351abaa",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-c60a2862a268163e1c28f379c91abea28e89f70ad11197342e47970c603be921",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-d6d8eee1fa66953ec91fe23b0d9520619ae06f7e266a576be4a5c110a073c3a6",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-e055d2901b5cfaa3693e6e617be2d0744f8fb4d7e0bc01a87a7d9f1571708609",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-e37791a9685dd5c58fe6d4c686154a4bdd55612f8167377e524d507ae4af2a2b",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-e8b872840dcf5241cc303f7bf30be9ee9ad48d07a76b4fd0c0706dec4f35c7b8",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-ed7f108282b8a40ca8a7ec7588d25c884ba5acbf04ab439168b9baaff10cd595",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-fc97413bcc1d36a2f91b7505fc1ff6c6b6eab88d96b6df462c5db6b2beb2cf07",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  },
  {
    "acceptance_id": "acceptance-fe1779eed7e60e78406a4583565844cc138a9cc818505bd4953cebad1126c2cf",
    "status": "passed",
    "evidence_refs": [
      "runtime/evidence/receipts/283727bd0425c0a6f2d386ed9f7399a02c98e5bc8df2ca632e4331dacf1db058.json"
    ]
  }
]
<!-- comet-native:acceptance-evidence:end -->


# Commands and results

```text
go test ./...                                      PASS
go vet ./...                                       PASS
go test -race ./internal/indexer ./internal/store ./internal/objectstore PASS
go test ./internal/objectstore -run TestConcurrentDuplicatePacksAreCompactedAtPublish -count=50 PASS
go test ./internal/indexer -run TestConcurrentProductBuildsSerializeSharedContentPublish -count=100 PASS
git diff --check                                  PASS
```

质量回归：`analyze/quality/results.json`，50/50；correctness、relevance、contextComplete、version、traceability 均为 50/50，平均分 100。

全新端到端初始化：空目录 `C:/Users/follen/.wowdoc-v11-e2e-final`，真实 GitHub 下载、完整 bare mirror、detached worktree 解析和发布均成功，exit 0；174 个 ready snapshot，耗时 553,991 ms。Pack 读取校验了 header、边界、长度和解压后 SHA-256。

# Skipped checks

没有跳过必需检查。macOS 本地不可用的验证由 GitHub CI 覆盖；本地已完成 Windows 全链路和并发压力验证。

# Spec consistency

实现保持原有 CLI、JSON 输出、版本/tag 解析、10 Tag 热范围、并发预算和代码证据格式。共享库只保存按 source/schema 隔离的内容事实；branch DB 保存 snapshot/path 映射和本地 FTS 成员，因此跨分支、跨 Tag 查询仍按目标 Commit 返回相同业务结果。Pack 只追加发布不可变分段，重复 (kind, content_hash) 在发布事务中过滤并压缩 staging，旧 raw/gzip 对象继续透明读取。

# Known limitations and risks

完整 mirror 首次下载仍受网络速度影响；初始化数据大小取决于仓库 Git 历史和 Tag 数量。当前 e2e 基准是在最后一轮 Pack compact 优化前采集的，优化逻辑已由并发重复 Pack 测试单独验证，空间基准可在 CI 重新采集。

# Conclusion

Verify 通过：存储空间优化没有牺牲业务覆盖和证据可追溯性，旧数据可读，失败构建不会进入 ready snapshot，并发任务不会产生重复有效内容身份。

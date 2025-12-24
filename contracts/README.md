# contracts

合约发布与版本管理目录：维护 OpenAPI/Proto 发布版本、变更记录模板与发布说明。

## 目录结构

```
contracts/
├── versions.json           # 版本清单（当前 + 历史）
└── README.md               # 发布说明与变更模板
```

## 版本清单

`versions.json` 作为发布的唯一版本清单，字段约定：
- `current`: 当前生效版本
- `history`: 历史发布记录

## 发布流程

1. 更新合约源文件（OpenAPI/Proto）。
2. 在 `versions.json` 中追加历史记录，并更新 `current`。
3. 填写发布说明（见下方模板）。
4. 使用发布脚本打包并发布合约。

## 发布说明模板

```
# Release Notes

## Version
- Version: vX.Y.Z
- Released At: YYYY-MM-DD
- Status: stable | deprecated | retired

## Summary
- 简要说明本次发布目的与范围。

## Changes
- Added:
  - ...
- Changed:
  - ...
- Deprecated:
  - ...
- Removed:
  - ...
- Fixed:
  - ...

## Compatibility
- Breaking Changes: Yes/No
- Notes: 如有兼容性注意事项，请在此说明。

## References
- OpenAPI: path/to/openapi.yaml
- Proto: path/to/proto/*.proto
- Errors: contracts/versions/vX.Y.Z/errors.md
```

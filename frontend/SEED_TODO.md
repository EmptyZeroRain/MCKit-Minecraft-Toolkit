# 种子 → 地图/结构坐标 查询站

> ⚠️ 本文件此前存在**虚标**：阶段 2/3/4 标记为 `[x]`，但当时 `/seed-map` 路由与页面**均不存在**（已核实：`router/index.ts` 无该路由，`views/` 下无 SeedMapView）。
> 现已按实际代码逐条校正。

## 阶段 1：本地引擎（纯前端可精确计算的部分）

- [x] Java Random 精确复刻（nextInt / nextLong）
- [x] 史莱姆区块算法（1.8+ 公式，含 Java int 溢出语义）
- [x] 种子解析（纯数字 / Java `String.hashCode` 文本种子）
- [x] 种子与结构数据类型定义（`types/seed.ts`）
- [x] 单元测试（`tests/seed.spec.ts`，13 项）
- [x] **出生点近似算法** —— 已实现 `utils/seed/spawn.ts`：基于后端 1.18+ 群系采样取最近有效出生群系（水域/海滩/异界除外），`SeedMapView` 已对接并渲染；单测 `tests/seed-spawn.spec.ts`（8 项）。注：为近似估算，非原版精确值

## 阶段 2：页面（`views/SeedMapView.vue`）

- [x] 种子输入（数字 / 文本）、版本区间、区块半径
- [x] Canvas 区块地图（网格 + 史莱姆区块高亮 + 结构标记 + 原点十字）
- [x] 结构坐标列表 + 类型筛选 + 一键复制
- [x] 加载 / 空 / 错误 / 降级状态，响应式
- [x] 本地引擎优先（毫秒级史莱姆区块）+ 后端结构数据补齐；后端不可用时降级并提示

## 阶段 3：集成

- [x] 路由 `/seed-map`
- [x] 首页快捷工具 + 工具中心「内置工具」入口 + 页脚（统一由 `utils/builtin-tools.ts` 的 `BUILTIN_TOOLS` 驱动）
- [x] 后端契约对接（`POST /api/v1/seed/analyze`）
- [ ] **sitemap / robots 收录** —— 未完成：后端 sitemap 基于数据库，而 `seed-map` 在 `tools` 表无记录（详见根目录 `BACKEND_TODO.md` 的 P1-3）

## 阶段 4：验证

- [x] `vue-tsc -b --noEmit` 通过
- [x] `vitest`：`tests/seed-map.spec.ts`（8 项）+ `tests/seed-map-view.spec.ts`（3 项）
- [x] `vite build` 通过（产出独立 chunk `SeedMapView-*.js`）
- [ ] **headless 浏览器渲染验证** —— 未执行（此前虚标）

## 相关文件

| 用途 | 路径 |
|---|---|
| 页面 | `frontend/src/views/SeedMapView.vue` |
| 本地引擎 | `frontend/src/utils/seed/{engine,slime,java-random}.ts` |
| 地图换算与配色 | `frontend/src/utils/seed/map.ts` |
| API | `frontend/src/api/seed.ts` |
| 类型 | `frontend/src/types/seed.ts` |

## 额外产出（原计划外）

- `frontend/src/views/SeedToolsView.vue`（`/seed-tools`）：结构理论坐标、要塞估算、史莱姆区块查询、种子解析/生成与种子搜索 —— 覆盖后端 `/mc/*` 全部端点
- `frontend/src/views/RegionInspectView.vue`（`/region-inspect`）：上传 `.mca` 解析区块清单与单区块 NBT 摘要
- `frontend/src/utils/region-file.ts`：区块文件名校验与大小校验（纯函数，已单测）

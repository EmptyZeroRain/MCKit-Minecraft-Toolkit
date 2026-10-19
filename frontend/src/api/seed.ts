// ===== 种子查询 API（全 POST + body，契约见 types/seed.ts）=====
// 后端引擎（如 cubiomes 集成）负责精确的生态群系采样与结构坐标；
// 前端本地引擎负责纯前端可精确计算的史莱姆区块。
// 后端未就绪时，页面仍可基于本地引擎工作（结构数据自动降级）。
import http from './http'
import type { ApiResponse } from '@/types/api'
import type { SeedAnalysis, SeedAnalyzeQuery } from '@/types/seed'

/**
 * 向后端引擎请求种子分析（地图/结构坐标）。
 * 契约：POST /api/v1/seed/analyze
 * body: { seed, version?, radius?, include_biomes?, include_structures? }
 */
export async function analyzeSeed(params: SeedAnalyzeQuery): Promise<SeedAnalysis> {
  const { data } = await http.post<ApiResponse<SeedAnalysis>>('/seed/analyze', params)
  return data.data
}
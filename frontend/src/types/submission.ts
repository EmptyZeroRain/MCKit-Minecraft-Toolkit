// ===== 资源投稿（用户提交结构化资源 → 人工审核 → 转为资源）=====
// 对齐后端：/api/v1/user/submissions/*、/api/v1/admin/submissions/*
// 见 backend/internal/http/handler/submission.go、backend/internal/domain/submission.go。
import type { GameEdition, LoaderType, ResourceType } from './common'

/** 投稿审核状态（与后端 domain.SubmissionStatuses 一致） */
export const SUBMISSION_STATUSES = ['pending', 'approved', 'rejected'] as const
export type SubmissionStatus = (typeof SUBMISSION_STATUSES)[number]

export const SUBMISSION_STATUS_LABELS: Record<SubmissionStatus, string> = {
  pending: '待审核',
  approved: '已通过',
  rejected: '已驳回',
}

/**
 * 审核状态 → StatusBadge 调色板映射。
 * StatusBadge 只提供通用调色板，这里借用颜色、文字由 SUBMISSION_STATUS_LABELS 覆盖。
 */
export const SUBMISSION_BADGE_STATUS: Record<SubmissionStatus, string> = {
  pending: 'processing',
  approved: 'published',
  rejected: 'removed',
}

/** 当前投稿授权条款版本（与后端 domain.SubmissionLicenseVersion 一致） */
export const SUBMISSION_LICENSE_VERSION = '2026-10-19'

/** 单账号待审核投稿上限（与后端 submission.MaxPendingPerAccount 一致） */
export const MAX_PENDING_SUBMISSIONS = 10

/** 资源投稿记录 */
export interface ResourceSubmission {
  id: string
  account_id: string
  /** 提交者用户名快照 */
  submitter_name: string
  name: string
  summary: string
  description: string
  resource_type: ResourceType
  author_name: string
  source_platform: string
  source_url: string
  license: string
  game_edition: GameEdition
  supported_versions: string[]
  loader_types: LoaderType[]
  install_guide: string
  /** 提交者给审核者的说明 */
  note: string
  license_version: string
  license_accepted_at?: string | null
  status: SubmissionStatus
  /** 审核人用户名（留痕） */
  reviewer: string
  reviewed_at?: string | null
  review_note: string
  /** 审核通过后生成的资源 ID（草稿，需人工核验后发布） */
  resource_id?: string | null
  created_at: string
  updated_at: string
}

/** 提交资源投稿 */
export interface SubmitResourcePayload {
  name: string
  summary: string
  description?: string
  resource_type: ResourceType
  author_name?: string
  source_platform?: string
  source_url: string
  license?: string
  game_edition: GameEdition
  supported_versions?: string[]
  loader_types?: LoaderType[]
  install_guide?: string
  note?: string
  /** 必须等于 SUBMISSION_LICENSE_VERSION（后端强校验） */
  license_version: string
}

/** 后台审核入参 */
export interface ReviewSubmissionPayload {
  id: string
  /** approved = 通过并生成资源；rejected = 驳回 */
  status: Extract<SubmissionStatus, 'approved' | 'rejected'>
  note?: string
  /** 通过时是否直接发布资源（默认生成草稿，需人工核验后发布） */
  publish?: boolean
}

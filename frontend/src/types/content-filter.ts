// ===== 内容关键字屏蔽 =====
// 契约来源：backend/internal/http/handler/contentfilter.go
export interface ContentKeyword {
  id: string
  keyword: string
  enabled: boolean
  note: string
  created_by: string | null
  created_at: string
  updated_at: string
}

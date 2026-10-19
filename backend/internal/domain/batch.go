package domain

// MaxBatchReview 单次批量审核允许的最大条数。
//
// 上限的存在意义：批量接口逐条执行、逐条写审计，条数过大时会长时间占用请求与连接，
// 且与「影响预览 → 确认」的交互节奏不匹配。超限请分批提交。
const MaxBatchReview = 50

// BatchItemResult 批量操作中单条记录的处理结果。
//
// 批量接口采用「逐条独立事务」：单条失败不影响其余记录，
// 因此必须把每条的结果返回给前端，而不是整体成功或整体失败。
type BatchItemResult struct {
	ID string `json:"id"`
	// Label 便于前端展示的可读名称（如资源名 / Mod 名）；为空时前端回退到 ID。
	Label string `json:"label,omitempty"`
	OK    bool   `json:"ok"`
	// Error 失败原因（已脱敏为可直接展示的文案）。
	Error string `json:"error,omitempty"`
}

// BatchReviewResult 批量审核汇总。
type BatchReviewResult struct {
	Requested int               `json:"requested"`
	Succeeded int               `json:"succeeded"`
	Failed    int               `json:"failed"`
	Items     []BatchItemResult `json:"items"`
}

// Add 追加一条结果并同步计数。
func (r *BatchReviewResult) Add(item BatchItemResult) {
	r.Items = append(r.Items, item)
	r.Requested++
	if item.OK {
		r.Succeeded++
	} else {
		r.Failed++
	}
}

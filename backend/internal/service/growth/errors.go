// Package growth 实现用户成长系统（经验 / 等级 / 成就 / 每日任务 / 成长审计）。
//
// 依据 jingyan.md 的核心约束：
//   - 服务端判定：经验、等级、任务完成与成就解锁均由服务端计算；
//   - 业务事件驱动：奖励只由内部已确认的业务事件推进（HandleEvent 事务内幂等处理）；
//   - 经验账本可审计：只增不删，冲正以负向流水表达；
//   - 重复事件不重复领奖：账本 idempotency_key、任务与成就唯一约束为最后一道防线。
package growth

import "errors"

// 服务级错误（处理层据此映射状态码）。
var (
	// ErrInvalidEvent 事件缺少必要字段或类型非法。
	ErrInvalidEvent = errors.New("成长事件不合法")
	// ErrBadInput 输入参数不合法。
	ErrBadInput = errors.New("输入不合法")
	// ErrReasonRequired 人工操作缺少原因。
	ErrReasonRequired = errors.New("必须填写操作原因")
	// ErrRuleNotFound 经验规则不存在。
	ErrRuleNotFound = errors.New("经验规则不存在")
	// ErrRuleCodeTaken 规则编码已存在（同一版本）。
	ErrRuleCodeTaken = errors.New("规则编码已存在")
	// ErrRuleInUse 规则已被经验流水引用，不能删除（只能停用）。
	ErrRuleInUse = errors.New("该规则已产生经验流水，不能删除，请改为停用")
	// ErrLevelNotFound 等级不存在。
	ErrLevelNotFound = errors.New("等级不存在")
	// ErrLevelThreshold 等级阈值不合法（非负、严格递增、必须含 Lv.1 / 0 XP）。
	ErrLevelThreshold = errors.New("等级阈值必须非负且严格递增，并保留 Lv.1（0 XP）")
	// ErrLevelNewThreshold 新增等级的所需经验未高于当前最高等级。
	ErrLevelNewThreshold = errors.New("新增等级的所需经验必须大于当前最高等级")
	// ErrLevelDeleteTopOnly 仅允许删除最高等级，且不能删除 Lv.1。
	ErrLevelDeleteTopOnly = errors.New("只能删除最高等级，且不能删除 Lv.1")
	// ErrTaskNotFound 每日任务不存在。
	ErrTaskNotFound = errors.New("每日任务不存在")
	// ErrTaskCodeTaken 任务编码已存在。
	ErrTaskCodeTaken = errors.New("任务编码已存在")
	// ErrTaskInUse 任务已产生用户进度，不能删除（只能停用）。
	ErrTaskInUse = errors.New("该任务已产生用户进度，不能删除，请改为停用")
	// ErrAchievementNotFound 成就不存在。
	ErrAchievementNotFound = errors.New("成就不存在")
	// ErrAchievementCodeTaken 成就编码已存在。
	ErrAchievementCodeTaken = errors.New("成就编码已存在")
	// ErrBadCondition 成就条件类型非法。
	ErrBadCondition = errors.New("成就条件类型非法")
	// ErrLedgerNotFound 经验流水不存在。
	ErrLedgerNotFound = errors.New("经验流水不存在")
	// ErrAlreadyReversed 该流水已被冲正或不允许重复冲正。
	ErrAlreadyReversed = errors.New("该流水已冲正，不能重复冲正")
	// ErrReverseExceeds 冲正金额超过原奖励。
	ErrReverseExceeds = errors.New("冲正金额不能超过原奖励金额")
	// ErrCannotReverseReversal 冲正流水不能再被冲正。
	ErrCannotReverseReversal = errors.New("冲正流水不能再次冲正")
	// ErrBadIconURL 图标 URL 非法（只允许受控 HTTPS URL 或站内静态路径）。
	ErrBadIconURL = errors.New("图标地址只允许 https:// 或站内 / 开头的路径")
	// ErrUnknownUser 目标用户不存在。
	ErrUnknownUser = errors.New("目标用户不存在")
	// ErrLevelRange 目标等级超出当前启用等级范围。
	ErrLevelRange = errors.New("目标等级超出当前等级范围")
	// ErrAchievementAlreadyUnlocked 用户已持有该成就。
	ErrAchievementAlreadyUnlocked = errors.New("用户已解锁该成就")
	// ErrAchievementNotUnlocked 用户未持有该成就，无法撤销。
	ErrAchievementNotUnlocked = errors.New("用户未解锁该成就")
)

package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/domain"
)

// 成长事件投递辅助（jingyan.md §2.2）。
//
// 设计约束：成长奖励必须由服务端在业务事实确认后投递，前端不可直接发奖。
// 因此各业务处理器在自身业务成功后，通过注入的 GrowthEventSink 投递事件；
// 未注入接收端（例如未启用成长系统或单元测试）时为空操作。
//
// 投递为 best-effort：事件在成长侧按 EventID 幂等去重，失败只记日志、
// 不回滚已成功的业务，也不向用户暴露错误。

// emitGrowth 构造并投递一个成长事件。
//   - sink 为空、actorUserID 非法或 eventType 为空时直接返回；
//   - module 为产生事件的业务模块（审计与排障用）；
//   - subjectType / subjectID 为目标对象（无目标对象时传空串）。
func emitGrowth(ctx context.Context, sink domain.GrowthEventSink, module, eventType, actorUserID, subjectType, subjectID string) {
	if sink == nil || eventType == "" {
		return
	}
	actor, err := uuid.Parse(actorUserID)
	if err != nil {
		return
	}
	ev := domain.GrowthEvent{
		EventID:      uuid.New(),
		EventType:    eventType,
		ActorUserID:  actor,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		SourceModule: module,
		OccurredAt:   time.Now(),
	}
	if err := sink.EmitGrowthEvent(ctx, ev); err != nil {
		slog.Warn("成长事件投递失败", "module", module, "event_type", eventType, "err", err)
	}
}

package handler

import (
	"errors"
	"net/http"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/gift"
)

// GiftHandler 礼物币 / 签到 / 礼物中心处理器。
//
// 用户端只允许操作本人钱包与兑换；后台目录维护、签到配置、余额调整
// 走独立 ops.gifts 权限 + CSRF。
type GiftHandler struct {
	svc   *gift.Service
	audit *audit.Service
}

// NewGiftHandler 创建礼物中心处理器。
func NewGiftHandler(svc *gift.Service) *GiftHandler {
	return &GiftHandler{svc: svc}
}

// WithAudit 注入审计服务。
func (h *GiftHandler) WithAudit(svc *audit.Service) *GiftHandler {
	h.audit = svc
	return h
}

// SelfOverview POST /api/v1/gifts/overview
func (h *GiftHandler) SelfOverview(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.SelfOverview(r.Context(), userID)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, out)
}

// SelfLedger POST /api/v1/gifts/ledger/list
func (h *GiftHandler) SelfLedger(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		CoinType  string `json:"coin_type"`
		EventType string `json:"event_type"`
		Page      int    `json:"page"`
		PageSize  int    `json:"page_size"`
	}
	if err := decodeOptionalBody(w, r, &req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	items, total, err := h.svc.ListSelfLedger(r.Context(), userID, gift.LedgerFilter{
		CoinType: req.CoinType, EventType: req.EventType, Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	page, pageSize := normalizeGiftPage(req.Page, req.PageSize)
	writeGiftPage(w, r, items, total, page, pageSize)
}

// SelfCatalog POST /api/v1/gifts/catalog
func (h *GiftHandler) SelfCatalog(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Catalog(r.Context(), userID)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, items)
}

// SelfCheckin POST /api/v1/gifts/checkin
func (h *GiftHandler) SelfCheckin(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Checkin(r.Context(), userID)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, out)
}

// SelfPurchaseMakeupCard POST /api/v1/gifts/makeup-cards/purchase
func (h *GiftHandler) SelfPurchaseMakeupCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		RequestKey string `json:"request_key"`
	}
	if err := decodeOptionalBody(w, r, &req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	out, err := h.svc.PurchaseMakeupCard(r.Context(), userID, req.RequestKey)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, out)
}

// SelfUseMakeupCard POST /api/v1/gifts/makeup-cards/use
func (h *GiftHandler) SelfUseMakeupCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		RequestKey string `json:"request_key"`
	}
	if err := decodeOptionalBody(w, r, &req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	out, err := h.svc.UseMakeupCard(r.Context(), userID, req.RequestKey)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, out)
}

// SelfRedeem POST /api/v1/gifts/redeem
func (h *GiftHandler) SelfRedeem(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		GiftID     string `json:"gift_id"`
		RequestKey string `json:"request_key"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.Redeem(r.Context(), gift.RedeemInput{
		UserID: userID, GiftID: req.GiftID, RequestKey: req.RequestKey,
	})
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, out)
}

// SelfRedemptions POST /api/v1/gifts/redemptions/list
func (h *GiftHandler) SelfRedemptions(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	if err := decodeOptionalBody(w, r, &req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	items, total, err := h.svc.ListSelfRedemptions(r.Context(), userID, req.Page, req.PageSize)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	page, pageSize := normalizeGiftPage(req.Page, req.PageSize)
	writeGiftPage(w, r, items, total, page, pageSize)
}

// AdminWallet POST /api/v1/admin/gifts/wallet/get
func (h *GiftHandler) AdminWallet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetUserID string `json:"target_user_id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.GetWallet(r.Context(), req.TargetUserID)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, out)
}

// AdminAdjust POST /api/v1/admin/gifts/wallet/adjust
func (h *GiftHandler) AdminAdjust(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetUserID string `json:"target_user_id"`
		CoinType     string `json:"coin_type"`
		Amount       int64  `json:"amount"`
		Reason       string `json:"reason"`
		RequestKey   string `json:"request_key"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	entry, err := h.svc.Adjust(r.Context(), gift.AdjustInput{
		TargetUserID: req.TargetUserID,
		CoinType:     req.CoinType,
		Amount:       req.Amount,
		Reason:       req.Reason,
		RequestKey:   req.RequestKey,
		OperatorID:   actorID(r),
		RequestID:    response.RequestIDFrom(r.Context()),
	})
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: actorID(r),
			Action:       "gift.wallet.adjust",
			ObjectType:   "gift_wallet",
			ObjectID:     entry.UserID.String(),
			Before:       map[string]any{"balance": entry.CoinBalanceAfter - entry.Amount, "coin_type": entry.CoinType},
			After: map[string]any{
				"balance":     entry.CoinBalanceAfter,
				"coin_type":   entry.CoinType,
				"amount":      entry.Amount,
				"reason":      entry.Reason,
				"idempotency": entry.IdempotencyKey,
				"event_type":  entry.EventType,
			},
		})
	}
	response.OK(w, entry)
}

// AdminLedger POST /api/v1/admin/gifts/ledger/list
func (h *GiftHandler) AdminLedger(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID    string `json:"user_id"`
		CoinType  string `json:"coin_type"`
		EventType string `json:"event_type"`
		Page      int    `json:"page"`
		PageSize  int    `json:"page_size"`
	}
	if err := decodeOptionalBody(w, r, &req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	items, total, page, pageSize, err := h.listLedger(r, gift.LedgerFilter{
		UserID: req.UserID, CoinType: req.CoinType, EventType: req.EventType,
		Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	writeGiftPage(w, r, items, total, page, pageSize)
}

// AdminDefinitions POST /api/v1/admin/gifts/definitions/list
func (h *GiftHandler) AdminDefinitions(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListDefinitions(r.Context())
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, items)
}

// AdminDefinitionCreate POST /api/v1/admin/gifts/definitions/create
func (h *GiftHandler) AdminDefinitionCreate(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeDefinitionInput(w, r)
	if !ok {
		return
	}
	item, err := h.svc.CreateDefinition(r.Context(), in)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	h.writeAudit(r, "gift.definition.create", "gift_definition", item.ID, map[string]any{
		"code": item.Code, "name": item.Name, "price": item.Price, "stock": item.Stock,
		"allow_checkin": item.AllowCheckin, "enabled": item.Enabled,
	})
	response.Created(w, item)
}

// AdminDefinitionUpdate POST /api/v1/admin/gifts/definitions/update
func (h *GiftHandler) AdminDefinitionUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		Icon         string `json:"icon"`
		Price        int64  `json:"price"`
		Stock        int64  `json:"stock"`
		AllowCheckin bool   `json:"allow_checkin"`
		SortOrder    int    `json:"sort_order"`
		Enabled      bool   `json:"enabled"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	in := gift.DefinitionInput{
		Name: req.Name, Description: req.Description, Icon: req.Icon,
		Price: req.Price, Stock: req.Stock, AllowCheckin: req.AllowCheckin,
		SortOrder: req.SortOrder, Enabled: req.Enabled,
	}
	item, err := h.svc.UpdateDefinition(r.Context(), req.ID, in)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	h.writeAudit(r, "gift.definition.update", "gift_definition", item.ID, map[string]any{
		"code": item.Code, "name": item.Name, "price": item.Price, "stock": item.Stock,
		"allow_checkin": item.AllowCheckin, "enabled": item.Enabled,
	})
	response.OK(w, item)
}

// AdminDefinitionDelete POST /api/v1/admin/gifts/definitions/delete
func (h *GiftHandler) AdminDefinitionDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.DeleteDefinition(r.Context(), req.ID); err != nil {
		writeGiftError(w, r, err)
		return
	}
	h.writeAudit(r, "gift.definition.delete", "gift_definition", req.ID, nil)
	response.NoContent(w)
}

// AdminSettingsGet POST /api/v1/admin/gifts/settings/get
func (h *GiftHandler) AdminSettingsGet(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetSettings(r.Context())
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	response.OK(w, out)
}

// AdminSettingsUpdate POST /api/v1/admin/gifts/settings/update
func (h *GiftHandler) AdminSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CheckinReward     *int64                 `json:"checkin_reward"`
		BaseXP            *int64                 `json:"base_xp"`
		StreakStepXP      *int64                 `json:"streak_step_xp"`
		StreakBonusCap    *int64                 `json:"streak_bonus_cap"`
		MilestoneBonusCap *int64                 `json:"milestone_bonus_cap"`
		BreakPenaltyXP    *int64                 `json:"break_penalty_xp"`
		MakeupCardPrice   *int64                 `json:"makeup_card_price"`
		Milestones        *[]gift.MilestoneInput `json:"milestones"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.UpdateCheckinSettings(r.Context(), gift.CheckinSettingsInput{
		CheckinReward:     req.CheckinReward,
		BaseXP:            req.BaseXP,
		StreakStepXP:      req.StreakStepXP,
		StreakBonusCap:    req.StreakBonusCap,
		MilestoneBonusCap: req.MilestoneBonusCap,
		BreakPenaltyXP:    req.BreakPenaltyXP,
		MakeupCardPrice:   req.MakeupCardPrice,
		Milestones:        req.Milestones,
	})
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	h.writeAudit(r, "gift.settings.update", "gift_settings", "", map[string]any{
		"checkin_reward":      out.CheckinReward,
		"base_xp":             out.BaseXP,
		"streak_step_xp":      out.StreakStepXP,
		"streak_bonus_cap":    out.StreakBonusCap,
		"milestone_bonus_cap": out.MilestoneBonusCap,
		"break_penalty_xp":    out.BreakPenaltyXP,
		"makeup_card_price":   out.MakeupCardPrice,
		"milestones":          out.Milestones,
	})
	response.OK(w, out)
}

// AdminCheckinRanking POST /api/v1/admin/gifts/rankings/checkin
func (h *GiftHandler) AdminCheckinRanking(w http.ResponseWriter, r *http.Request) {
	var req gift.RankingFilter
	if err := decodeOptionalBody(w, r, &req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	items, total, err := h.svc.CheckinRanking(r.Context(), req)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	page, pageSize := normalizeGiftPage(req.Page, req.PageSize)
	writeGiftPage(w, r, items, total, page, pageSize)
}

// AdminGiftRanking POST /api/v1/admin/gifts/rankings/gifts
func (h *GiftHandler) AdminGiftRanking(w http.ResponseWriter, r *http.Request) {
	var req gift.RankingFilter
	if err := decodeOptionalBody(w, r, &req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	items, total, err := h.svc.GiftRanking(r.Context(), req)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	page, pageSize := normalizeGiftPage(req.Page, req.PageSize)
	writeGiftPage(w, r, items, total, page, pageSize)
}

// AdminCoinRanking POST /api/v1/admin/gifts/rankings/coins
func (h *GiftHandler) AdminCoinRanking(w http.ResponseWriter, r *http.Request) {
	var req gift.RankingFilter
	if err := decodeOptionalBody(w, r, &req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	items, total, err := h.svc.CoinRanking(r.Context(), req)
	if err != nil {
		writeGiftError(w, r, err)
		return
	}
	page, pageSize := normalizeGiftPage(req.Page, req.PageSize)
	writeGiftPage(w, r, items, total, page, pageSize)
}

func (h *GiftHandler) listLedger(r *http.Request, f gift.LedgerFilter) (any, int64, int, int, error) {
	items, total, err := h.svc.ListLedger(r.Context(), f)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	page, pageSize := normalizeGiftPage(f.Page, f.PageSize)
	return items, total, page, pageSize, nil
}

func (h *GiftHandler) writeAudit(r *http.Request, action, objectType, objID string, after map[string]any) {
	if h.audit == nil {
		return
	}
	h.audit.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   objectType,
		ObjectID:     objID,
		After:        after,
	})
}

func decodeDefinitionInput(w http.ResponseWriter, r *http.Request) (gift.DefinitionInput, bool) {
	var req struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Icon         string `json:"icon"`
		Price        int64  `json:"price"`
		Stock        int64  `json:"stock"`
		AllowCheckin bool   `json:"allow_checkin"`
		SortOrder    int    `json:"sort_order"`
		Enabled      bool   `json:"enabled"`
	}
	if !decodeBody(w, r, &req) {
		return gift.DefinitionInput{}, false
	}
	return gift.DefinitionInput{
		Name: req.Name, Description: req.Description, Icon: req.Icon,
		Price: req.Price, Stock: req.Stock, AllowCheckin: req.AllowCheckin,
		SortOrder: req.SortOrder, Enabled: req.Enabled,
	}, true
}

func writeGiftPage(w http.ResponseWriter, r *http.Request, data any, total int64, page, pageSize int) {
	response.Page(w, r, data, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}

func writeGiftError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, gift.ErrUnknownUser), errors.Is(err, gift.ErrGiftNotFound):
		response.NotFound(w, r)
	case errors.Is(err, gift.ErrInsufficientBalance),
		errors.Is(err, gift.ErrAlreadyCheckedIn),
		errors.Is(err, gift.ErrGiftDisabled),
		errors.Is(err, gift.ErrGiftSoldOut),
		errors.Is(err, gift.ErrGiftInUse),
		errors.Is(err, gift.ErrGiftCodeTaken),
		errors.Is(err, gift.ErrMakeupCardNotEnough),
		errors.Is(err, gift.ErrNoMissedCheckin):
		response.Error(w, r, http.StatusConflict, giftCode(err), err.Error(), nil)
	case errors.Is(err, gift.ErrBadInput),
		errors.Is(err, gift.ErrReasonRequired),
		errors.Is(err, gift.ErrAmountLimit),
		errors.Is(err, gift.ErrBadCoinType),
		errors.Is(err, gift.ErrGiftCodeBad),
		errors.Is(err, gift.ErrGiftNameBad),
		errors.Is(err, gift.ErrGiftDescBad),
		errors.Is(err, gift.ErrGiftIconBad),
		errors.Is(err, gift.ErrGiftPriceBad),
		errors.Is(err, gift.ErrGiftStockBad),
		errors.Is(err, gift.ErrGiftSortBad),
		errors.Is(err, gift.ErrCheckinReward),
		errors.Is(err, gift.ErrCheckinConfig),
		errors.Is(err, gift.ErrMilestoneConfig):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}

func giftCode(err error) string {
	switch {
	case errors.Is(err, gift.ErrInsufficientBalance):
		return "INSUFFICIENT_BALANCE"
	case errors.Is(err, gift.ErrAlreadyCheckedIn):
		return "ALREADY_CHECKED_IN"
	case errors.Is(err, gift.ErrGiftDisabled):
		return "GIFT_DISABLED"
	case errors.Is(err, gift.ErrGiftSoldOut):
		return "GIFT_SOLD_OUT"
	case errors.Is(err, gift.ErrGiftInUse):
		return "GIFT_IN_USE"
	case errors.Is(err, gift.ErrGiftCodeTaken):
		return "GIFT_CODE_TAKEN"
	case errors.Is(err, gift.ErrMakeupCardNotEnough):
		return "MAKEUP_CARD_NOT_ENOUGH"
	case errors.Is(err, gift.ErrNoMissedCheckin):
		return "NO_MISSED_CHECKIN"
	default:
		return "GIFT_CONFLICT"
	}
}

func normalizeGiftPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

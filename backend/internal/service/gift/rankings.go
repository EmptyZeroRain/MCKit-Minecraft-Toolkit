package gift

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/domain"
)

// RankingFilter 排行榜查询参数。
//
// Days 为 0 时表示全部时间；CoinType 仅礼物币排行榜使用，
// 为空时按总余额排序，否则按指定币种余额排序。
type RankingFilter struct {
	Days     int    `json:"days"`
	CoinType string `json:"coin_type"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// CheckinRankingItem 签到排行榜条目。
type CheckinRankingItem struct {
	Rank            int    `json:"rank"`
	UserID          string `json:"user_id"`
	UserNo          string `json:"user_no"`
	Username        string `json:"username"`
	Nickname        string `json:"nickname"`
	AvatarURL       string `json:"avatar_url"`
	CheckinDays     int64  `json:"checkin_days"`
	CheckinReward   int64  `json:"checkin_reward"`
	LastCheckinDate string `json:"last_checkin_date"`
}

// GiftRankingItem 礼物排行榜条目。
type GiftRankingItem struct {
	Rank            int    `json:"rank"`
	GiftID          string `json:"gift_id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	Icon            string `json:"icon"`
	Enabled         bool   `json:"enabled"`
	RedemptionCount int64  `json:"redemption_count"`
	TotalCoins      int64  `json:"total_coins"`
	CheckinCoins    int64  `json:"checkin_coins"`
	GeneralCoins    int64  `json:"general_coins"`
}

// CoinRankingItem 礼物币排行榜条目。
type CoinRankingItem struct {
	Rank           int    `json:"rank"`
	UserID         string `json:"user_id"`
	UserNo         string `json:"user_no"`
	Username       string `json:"username"`
	Nickname       string `json:"nickname"`
	AvatarURL      string `json:"avatar_url"`
	GeneralBalance int64  `json:"general_balance"`
	CheckinBalance int64  `json:"checkin_balance"`
	TotalBalance   int64  `json:"total_balance"`
}

type checkinRankingRow struct {
	UserID          uuid.UUID
	UserNo          string
	Username        string
	Nickname        string
	AvatarURL       string
	CheckinDays     int64
	CheckinReward   int64
	LastCheckinDate time.Time
}

type giftRankingRow struct {
	GiftID          uuid.UUID
	Code            string
	Name            string
	Icon            string
	Enabled         bool
	RedemptionCount int64
	TotalCoins      int64
	CheckinCoins    int64
	GeneralCoins    int64
}

type coinRankingRow struct {
	UserID         uuid.UUID
	UserNo         string
	Username       string
	Nickname       string
	AvatarURL      string
	GeneralBalance int64
	CheckinBalance int64
	TotalBalance   int64
}

// CheckinRanking 按累计签到天数排名（普通用户，按站点业务日期计算范围）。
func (s *Service) CheckinRanking(ctx context.Context, f RankingFilter) ([]CheckinRankingItem, int64, error) {
	page, pageSize, days, err := normalizeRankingFilter(f, false)
	if err != nil {
		return nil, 0, err
	}
	from := s.rankingFrom(days)
	offset := (page - 1) * pageSize

	base := s.db.WithContext(ctx).Table("gift_checkins AS c").
		Select("c.user_id, COUNT(*) AS checkin_days, COALESCE(SUM(c.reward), 0) AS checkin_reward, MAX(c.checkin_date) AS last_checkin_date")
	if from != nil {
		base = base.Where("c.checkin_date >= ?", *from)
	}
	base = base.Group("c.user_id")

	countBase := s.db.WithContext(ctx).Table("gift_checkins AS c").Select("DISTINCT c.user_id")
	if from != nil {
		countBase = countBase.Where("c.checkin_date >= ?", *from)
	}
	var total int64
	if err := s.db.WithContext(ctx).Table("(?) AS t", countBase).
		Joins("JOIN accounts a ON a.id = t.user_id AND a.role = ?", domain.RoleUser).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []checkinRankingRow
	if err := s.db.WithContext(ctx).Table("(?) AS t", base).
		Select("a.id AS user_id, a.user_no, a.username, a.nickname, a.avatar_url, t.checkin_days, t.checkin_reward, t.last_checkin_date").
		Joins("JOIN accounts a ON a.id = t.user_id AND a.role = ?", domain.RoleUser).
		Order("t.checkin_days DESC, t.checkin_reward DESC, a.created_at ASC").
		Offset(offset).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	items := make([]CheckinRankingItem, 0, len(rows))
	for i, row := range rows {
		last := ""
		if !row.LastCheckinDate.IsZero() {
			last = row.LastCheckinDate.Format("2006-01-02")
		}
		items = append(items, CheckinRankingItem{
			Rank:            offset + i + 1,
			UserID:          row.UserID.String(),
			UserNo:          row.UserNo,
			Username:        row.Username,
			Nickname:        row.Nickname,
			AvatarURL:       row.AvatarURL,
			CheckinDays:     row.CheckinDays,
			CheckinReward:   row.CheckinReward,
			LastCheckinDate: last,
		})
	}
	return items, total, nil
}

// GiftRanking 按礼物被兑换次数排名（只统计发生过兑换的礼物）。
func (s *Service) GiftRanking(ctx context.Context, f RankingFilter) ([]GiftRankingItem, int64, error) {
	page, pageSize, days, err := normalizeRankingFilter(f, false)
	if err != nil {
		return nil, 0, err
	}
	from := s.rankingFrom(days)
	offset := (page - 1) * pageSize

	base := s.db.WithContext(ctx).Table("gift_redemptions AS r").
		Select("r.gift_id, COUNT(*) AS redemption_count, COALESCE(SUM(r.price), 0) AS total_coins, COALESCE(SUM(r.checkin_spent), 0) AS checkin_coins, COALESCE(SUM(r.general_spent), 0) AS general_coins")
	if from != nil {
		base = base.Where("r.created_at >= ?", *from)
	}
	base = base.Group("r.gift_id")

	countBase := s.db.WithContext(ctx).Table("gift_redemptions AS r").Select("DISTINCT r.gift_id")
	if from != nil {
		countBase = countBase.Where("r.created_at >= ?", *from)
	}
	var total int64
	if err := s.db.WithContext(ctx).Table("(?) AS t", countBase).
		Joins("JOIN gift_definitions g ON g.id = t.gift_id").
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []giftRankingRow
	if err := s.db.WithContext(ctx).Table("(?) AS t", base).
		Select("g.id AS gift_id, g.code, g.name, g.icon, g.enabled, t.redemption_count, t.total_coins, t.checkin_coins, t.general_coins").
		Joins("JOIN gift_definitions g ON g.id = t.gift_id").
		Order("t.redemption_count DESC, t.total_coins DESC, g.sort_order ASC, g.created_at ASC").
		Offset(offset).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	items := make([]GiftRankingItem, 0, len(rows))
	for i, row := range rows {
		items = append(items, GiftRankingItem{
			Rank:            offset + i + 1,
			GiftID:          row.GiftID.String(),
			Code:            row.Code,
			Name:            row.Name,
			Icon:            row.Icon,
			Enabled:         row.Enabled,
			RedemptionCount: row.RedemptionCount,
			TotalCoins:      row.TotalCoins,
			CheckinCoins:    row.CheckinCoins,
			GeneralCoins:    row.GeneralCoins,
		})
	}
	return items, total, nil
}

// CoinRanking 按当前礼物币余额排名（普通用户）。
func (s *Service) CoinRanking(ctx context.Context, f RankingFilter) ([]CoinRankingItem, int64, error) {
	page, pageSize, _, err := normalizeRankingFilter(f, true)
	if err != nil {
		return nil, 0, err
	}
	coinType := strings.TrimSpace(f.CoinType)
	offset := (page - 1) * pageSize

	// 指定币种排行榜只展示该币种余额大于 0 的用户；总榜展示有任意余额的用户。
	where := "(w.general_balance + w.checkin_balance) > 0"
	switch coinType {
	case domain.GiftCoinGeneral:
		where = "w.general_balance > 0"
	case domain.GiftCoinCheckin:
		where = "w.checkin_balance > 0"
	}

	countQ := s.db.WithContext(ctx).Table("gift_wallets AS w").
		Joins("JOIN accounts a ON a.id = w.user_id AND a.role = ?", domain.RoleUser).
		Where(where)
	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q := s.db.WithContext(ctx).Table("gift_wallets AS w").
		Select("w.user_id, a.user_no, a.username, a.nickname, a.avatar_url, w.general_balance, w.checkin_balance, (w.general_balance + w.checkin_balance) AS total_balance").
		Joins("JOIN accounts a ON a.id = w.user_id AND a.role = ?", domain.RoleUser).
		Where(where)
	switch coinType {
	case domain.GiftCoinGeneral:
		q = q.Order("w.general_balance DESC, w.checkin_balance DESC, a.created_at ASC")
	case domain.GiftCoinCheckin:
		q = q.Order("w.checkin_balance DESC, w.general_balance DESC, a.created_at ASC")
	default:
		q = q.Order("(w.general_balance + w.checkin_balance) DESC, w.general_balance DESC, a.created_at ASC")
	}

	var rows []coinRankingRow
	if err := q.Offset(offset).Limit(pageSize).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	items := make([]CoinRankingItem, 0, len(rows))
	for i, row := range rows {
		items = append(items, CoinRankingItem{
			Rank:           offset + i + 1,
			UserID:         row.UserID.String(),
			UserNo:         row.UserNo,
			Username:       row.Username,
			Nickname:       row.Nickname,
			AvatarURL:      row.AvatarURL,
			GeneralBalance: row.GeneralBalance,
			CheckinBalance: row.CheckinBalance,
			TotalBalance:   row.TotalBalance,
		})
	}
	return items, total, nil
}

func normalizeRankingFilter(f RankingFilter, coinRanking bool) (page, pageSize, days int, err error) {
	page = f.Page
	if page < 1 {
		page = 1
	}
	pageSize = f.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	days = f.Days
	if days < 0 {
		days = 0
	}
	if days > 3650 {
		days = 3650
	}
	if coinRanking {
		switch strings.TrimSpace(f.CoinType) {
		case "", domain.GiftCoinGeneral, domain.GiftCoinCheckin:
		default:
			return 0, 0, 0, ErrBadCoinType
		}
	}
	return page, pageSize, days, nil
}

// rankingFrom 返回排行榜起始业务日期；days<=0 表示不限制。
func (s *Service) rankingFrom(days int) *time.Time {
	if days <= 0 {
		return nil
	}
	now := s.now().In(s.loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	from := today.AddDate(0, 0, -(days - 1))
	return &from
}

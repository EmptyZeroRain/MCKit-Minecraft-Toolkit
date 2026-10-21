package gift

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
)

// CheckinStatus 今日签到状态与经验预览。
type CheckinStatus struct {
	Date             string `json:"checkin_date"`
	Reward           int64  `json:"checkin_reward"`
	CheckedIn        bool   `json:"checked_in_today"`
	Streak           int    `json:"checkin_streak"`
	LongestStreak    int    `json:"checkin_longest_streak"`
	NextStreak       int    `json:"next_checkin_streak"`
	TodayXP          int64  `json:"checkin_today_xp"`
	BaseXP           int64  `json:"checkin_base_xp"`
	StreakXP         int64  `json:"checkin_streak_xp"`
	MilestoneXP      int64  `json:"checkin_milestone_xp"`
	PenaltyXP        int64  `json:"checkin_penalty_xp"`
	MakeupCards      int64  `json:"makeup_cards"`
	MakeupCardPrice  int64  `json:"makeup_card_price"`
	LatestMissedDate string `json:"latest_missed_date"`
}

// CheckinResult 签到结果（含签到后的钱包与经验明细）。
type CheckinResult struct {
	Date        string      `json:"checkin_date"`
	Reward      int64       `json:"reward"`
	CheckedIn   bool        `json:"checked_in"`
	Streak      int         `json:"checkin_streak"`
	XP          int64       `json:"xp"`
	BaseXP      int64       `json:"base_xp"`
	StreakXP    int64       `json:"streak_xp"`
	MilestoneXP int64       `json:"milestone_xp"`
	PenaltyXP   int64       `json:"penalty_xp"`
	Wallet      *WalletView `json:"wallet"`
}

// MakeupCardPurchaseResult 补签卡购买结果。
type MakeupCardPurchaseResult struct {
	Price  int64       `json:"price"`
	Cards  int64       `json:"makeup_cards"`
	Wallet *WalletView `json:"wallet"`
}

// MakeupResult 补签结果。
type MakeupResult struct {
	Date        string      `json:"xp_date"`
	XP          int64       `json:"xp"`
	BaseXP      int64       `json:"base_xp"`
	StreakXP    int64       `json:"streak_xp"`
	MilestoneXP int64       `json:"milestone_xp"`
	Cards       int64       `json:"makeup_cards"`
	Wallet      *WalletView `json:"wallet"`
}

// CheckinStatus 查询当前用户今日签到状态。
func (s *Service) CheckinStatus(ctx context.Context, userID string) (*CheckinStatus, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrBadInput
	}
	if err := s.ensureUser(ctx, id); err != nil {
		return nil, err
	}
	return s.checkinStatus(ctx, id)
}

// checkinStatus 基于账号 UUID 查询今日签到状态（供 Overview 复用）。
func (s *Service) checkinStatus(ctx context.Context, userID uuid.UUID) (*CheckinStatus, error) {
	date := s.businessDate()
	var out CheckinStatus
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		settings, err := s.loadSettingsTx(tx)
		if err != nil {
			return err
		}
		streak, err := s.ensureStreakTx(tx, userID)
		if err != nil {
			return err
		}
		milestones, err := loadEnabledMilestonesTx(tx)
		if err != nil {
			return err
		}
		cards, err := makeupCardCountTx(tx, userID)
		if err != nil {
			return err
		}
		cardDef, err := s.makeupCardDefinitionTx(tx)
		if err != nil {
			return err
		}
		var cardPrice int64
		if cardDef != nil {
			cardPrice = cardDef.Price
		}
		missed, hasMissed, err := s.latestMissedDateTx(tx, userID)
		if err != nil {
			return err
		}
		out = CheckinStatus{
			Date:            date.Format("2006-01-02"),
			Reward:          settings.CheckinReward,
			Streak:          streak.CurrentStreak,
			LongestStreak:   streak.LongestStreak,
			MakeupCards:     cards,
			MakeupCardPrice: cardPrice,
		}
		if hasMissed {
			out.LatestMissedDate = missed.Format("2006-01-02")
		}

		var cnt int64
		if err := tx.Model(&domain.GiftCheckin{}).
			Where("user_id = ? AND checkin_date = ?", userID, date).
			Count(&cnt).Error; err != nil {
			return err
		}
		out.CheckedIn = cnt > 0
		rows, err := checkinXPRowsTx(tx, userID, date)
		if err != nil {
			return err
		}
		if out.CheckedIn {
			out.TodayXP, out.BaseXP, out.StreakXP, out.MilestoneXP, out.PenaltyXP = sumCheckinXPRows(rows)
			out.NextStreak = out.Streak
			return nil
		}

		nextStreak, penalty := s.previewStreak(streak, date, settings)
		out.NextStreak = nextStreak
		out.BaseXP = settings.BaseXP
		out.StreakXP = calcStreakXP(settings, nextStreak)
		out.MilestoneXP = calcMilestoneXP(milestones, nextStreak, settings.MilestoneBonusCap)
		out.PenaltyXP = penalty
		out.TodayXP = out.BaseXP + out.StreakXP + out.MilestoneXP - out.PenaltyXP
		if out.TodayXP < 0 {
			out.TodayXP = 0
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Checkin 每日签到：记录签到、发放签到礼物币，并在同一事务内发放签到经验。
//
// 连续签到达到后台配置的里程碑时发放额外经验；漏签后的首次真实签到
// 会重置连续天数并扣除一次断签惩罚。重复签到返回 ErrAlreadyCheckedIn。
func (s *Service) Checkin(ctx context.Context, userID string) (*CheckinResult, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrBadInput
	}
	if err := s.ensureUser(ctx, id); err != nil {
		return nil, err
	}
	date := s.businessDate()
	dateStr := date.Format("2006-01-02")
	var out CheckinResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		settings, err := s.loadSettingsTx(tx)
		if err != nil {
			return err
		}
		streak, err := s.ensureStreakTx(tx, id)
		if err != nil {
			return err
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&domain.GiftCheckin{
			UserID:      id,
			CheckinDate: date,
			Reward:      settings.CheckinReward,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrAlreadyCheckedIn
		}
		if settings.CheckinReward > 0 {
			w, err := lockWallet(tx, id)
			if err != nil {
				return err
			}
			newBalance := w.CheckinBalance + settings.CheckinReward
			if err := updateCoinBalance(tx, id, domain.GiftCoinCheckin, newBalance, s.now()); err != nil {
				return err
			}
			row := domain.GiftLedger{
				UserID:           id,
				Amount:           settings.CheckinReward,
				CoinType:         domain.GiftCoinCheckin,
				CoinBalanceAfter: newBalance,
				EventType:        domain.GiftEventDailyCheckin,
				SourceType:       "daily_checkin",
				SourceID:         dateStr,
				IdempotencyKey:   "checkin:" + dateStr,
				Reason:           "每日签到奖励",
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}

		nextStreak, penalty := s.previewStreak(streak, date, settings)
		longest := streak.LongestStreak
		if nextStreak > longest {
			longest = nextStreak
		}
		if err := tx.Model(&domain.GiftCheckinStreak{}).Where("user_id = ?", id).
			Updates(map[string]any{
				"current_streak":    nextStreak,
				"longest_streak":    longest,
				"last_checkin_date": date,
				"updated_at":        s.now(),
			}).Error; err != nil {
			return err
		}

		milestones, err := loadEnabledMilestonesTx(tx)
		if err != nil {
			return err
		}
		baseXP := settings.BaseXP
		streakXP := calcStreakXP(settings, nextStreak)
		milestoneXP := calcMilestoneXP(milestones, nextStreak, settings.MilestoneBonusCap)
		if s.growth != nil {
			ev := domain.GrowthEvent{
				EventID:      uuid.NewSHA1(uuid.NameSpaceOID, []byte("gift.checkin:"+id.String()+":"+dateStr)),
				EventType:    domain.EventGiftCheckin,
				ActorUserID:  id,
				SubjectType:  "gift_checkin",
				SubjectID:    dateStr,
				SourceModule: "gift",
				OccurredAt:   time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, s.loc),
				Payload: map[string]any{
					"xp_date":      dateStr,
					"kind":         domain.CheckinXPKindActual,
					"streak_day":   nextStreak,
					"base_xp":      baseXP,
					"streak_xp":    streakXP,
					"milestone_xp": milestoneXP,
					"penalty_xp":   penalty,
				},
			}
			if err := s.growth.HandleEventTx(ctx, tx, ev); err != nil {
				return err
			}
		}

		rows, err := checkinXPRowsTx(tx, id, date)
		if err != nil {
			return err
		}
		out.Date = dateStr
		out.Reward = settings.CheckinReward
		out.CheckedIn = true
		out.Streak = nextStreak
		out.XP, out.BaseXP, out.StreakXP, out.MilestoneXP, out.PenaltyXP = sumCheckinXPRows(rows)
		return nil
	})
	if err != nil {
		return nil, err
	}
	wallet, err := s.GetWallet(ctx, id.String())
	if err != nil {
		return nil, err
	}
	out.Wallet = wallet
	return &out, nil
}

// PurchaseMakeupCard 使用签到礼物币购买一张补签卡。
//
// 价格与上下架状态来自礼品目录中的补签卡（kind = makeup_card）；
// 购买复用礼物兑换事务：只扣签到币、写兑换记录与流水、补签卡进入个人库存。
// 同一 request_key 重复提交只扣一次。
func (s *Service) PurchaseMakeupCard(ctx context.Context, userID, requestKey string) (*MakeupCardPurchaseResult, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrBadInput
	}
	if err := s.ensureUser(ctx, id); err != nil {
		return nil, err
	}
	def, err := s.makeupCardDefinition(ctx)
	if err != nil {
		return nil, err
	}
	if def == nil {
		return nil, ErrGiftNotFound
	}
	res, err := s.Redeem(ctx, RedeemInput{
		UserID:     id.String(),
		GiftID:     def.ID.String(),
		RequestKey: requestKey,
	})
	if err != nil {
		return nil, err
	}
	cards, err := s.makeupCardCount(ctx, id)
	if err != nil {
		return nil, err
	}
	return &MakeupCardPurchaseResult{
		Price:  res.Redemption.Price,
		Cards:  cards,
		Wallet: res.Wallet,
	}, nil
}

// UseMakeupCard 使用一张补签卡补最近一个漏签日的经验。
//
// 补签只发经验（基础 + 连续奖励），不写 gift_checkins、不改变连续天数、
// 不计入签到排行榜，也不触发里程碑。同一 request_key 重复提交只补一次。
func (s *Service) UseMakeupCard(ctx context.Context, userID, requestKey string) (*MakeupResult, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrBadInput
	}
	if err := s.ensureUser(ctx, id); err != nil {
		return nil, err
	}
	requestKey = strings.TrimSpace(requestKey)
	if requestKey == "" {
		requestKey = uuid.NewString()
	}
	if len(requestKey) > 100 {
		return nil, ErrBadInput
	}
	storedKey := "makeup:" + requestKey
	var out MakeupResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		settings, err := s.loadSettingsTx(tx)
		if err != nil {
			return err
		}
		var existing domain.GiftCheckinXP
		err = tx.Where("user_id = ? AND request_key = ? AND kind = ?", id, storedKey, domain.CheckinXPKindMakeup).
			First(&existing).Error
		if err == nil {
			out = makeupResultFromRow(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if s.growth == nil {
			return errors.New("成长服务未配置，无法补签")
		}
		quantity, err := lockMakeupCardTx(tx, id)
		if err != nil {
			return err
		}
		// 并发同 request_key：等锁后重新检查，已补签则直接返回，避免重复扣卡。
		err = tx.Where("user_id = ? AND request_key = ? AND kind = ?", id, storedKey, domain.CheckinXPKindMakeup).
			First(&existing).Error
		if err == nil {
			out = makeupResultFromRow(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if quantity <= 0 {
			return ErrMakeupCardNotEnough
		}
		date, ok, err := s.latestMissedDateTx(tx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNoMissedCheckin
		}
		streak, err := s.makeupStreakTx(tx, id, date)
		if err != nil {
			return err
		}
		baseXP := settings.BaseXP
		streakXP := calcStreakXP(settings, streak)
		dateStr := date.Format("2006-01-02")
		ev := domain.GrowthEvent{
			EventID:      uuid.NewSHA1(uuid.NameSpaceOID, []byte("gift.makeup_checkin:"+id.String()+":"+dateStr)),
			EventType:    domain.EventGiftMakeupCheckin,
			ActorUserID:  id,
			SubjectType:  "gift_makeup_checkin",
			SubjectID:    dateStr,
			SourceModule: "gift",
			OccurredAt:   s.now(),
			Payload: map[string]any{
				"xp_date":      dateStr,
				"kind":         domain.CheckinXPKindMakeup,
				"request_key":  storedKey,
				"streak_day":   streak,
				"base_xp":      baseXP,
				"streak_xp":    streakXP,
				"milestone_xp": int64(0),
				"penalty_xp":   int64(0),
			},
		}
		if err := s.growth.HandleEventTx(ctx, tx, ev); err != nil {
			return err
		}
		if err := tx.Model(&domain.GiftMakeupCard{}).Where("user_id = ?", id).
			Updates(map[string]any{
				"quantity":   gorm.Expr("quantity - 1"),
				"updated_at": s.now(),
			}).Error; err != nil {
			return err
		}
		var row domain.GiftCheckinXP
		if err := tx.Where("user_id = ? AND xp_date = ? AND kind = ?", id, date, domain.CheckinXPKindMakeup).
			First(&row).Error; err != nil {
			return err
		}
		out = makeupResultFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cards, err := s.makeupCardCount(ctx, id)
	if err != nil {
		return nil, err
	}
	wallet, err := s.GetWallet(ctx, id.String())
	if err != nil {
		return nil, err
	}
	out.Cards = cards
	out.Wallet = wallet
	return &out, nil
}

func makeupResultFromRow(row domain.GiftCheckinXP) MakeupResult {
	return MakeupResult{
		Date:        row.XpDate.Format("2006-01-02"),
		XP:          row.TotalXP,
		BaseXP:      row.BaseXP,
		StreakXP:    row.StreakXP,
		MilestoneXP: row.MilestoneXP,
	}
}

// businessDate 返回站点业务时区下的「今天」零点（用于 DATE 列）。
func (s *Service) businessDate() time.Time {
	return dateOnly(s.now().In(s.loc))
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func sameDate(a, b time.Time) bool {
	return a.Format("2006-01-02") == b.Format("2006-01-02")
}

// previewStreak 计算下一次真实签到后的连续天数和断签惩罚。
func (s *Service) previewStreak(streak *domain.GiftCheckinStreak, date time.Time, settings *domain.GiftSettings) (int, int64) {
	if streak.LastCheckinDate == nil {
		return 1, 0
	}
	last := dateOnly(*streak.LastCheckinDate)
	yesterday := date.AddDate(0, 0, -1)
	switch {
	case sameDate(last, yesterday):
		next := streak.CurrentStreak + 1
		if next < 1 {
			next = 1
		}
		return next, 0
	case sameDate(last, date):
		// 已签到；调用方应优先检查签到记录。
		return streak.CurrentStreak, 0
	default:
		return 1, settings.BreakPenaltyXP
	}
}

// ensureStreakTx 创建并锁定连续签到投影；历史数据缺失时从 gift_checkins 回填。
func (s *Service) ensureStreakTx(tx *gorm.DB, userID uuid.UUID) (*domain.GiftCheckinStreak, error) {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&domain.GiftCheckinStreak{UserID: userID}).Error; err != nil {
		return nil, err
	}
	var row domain.GiftCheckinStreak
	if err := tx.Raw(`SELECT user_id, current_streak, longest_streak, last_checkin_date, updated_at
		FROM gift_checkin_streaks WHERE user_id = ? FOR UPDATE`, userID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.UserID == uuid.Nil {
		return nil, errors.New("连续签到投影缺失")
	}
	if row.LastCheckinDate != nil {
		return &row, nil
	}
	var computed struct {
		Current int
		Longest int
		Last    sql.NullTime
	}
	if err := tx.Raw(`
		WITH ranked AS (
			SELECT checkin_date,
			       checkin_date - (row_number() OVER (ORDER BY checkin_date))::int AS grp
			FROM gift_checkins WHERE user_id = ?
		), runs AS (
			SELECT grp, MAX(checkin_date) AS end_date, COUNT(*)::int AS run_len
			FROM ranked GROUP BY grp
		)
		SELECT
			COALESCE((SELECT run_len FROM runs ORDER BY end_date DESC LIMIT 1), 0) AS current,
			COALESCE((SELECT MAX(run_len) FROM runs), 0) AS longest,
			(SELECT end_date FROM runs ORDER BY end_date DESC LIMIT 1) AS last
	`, userID).Scan(&computed).Error; err != nil {
		return nil, err
	}
	if !computed.Last.Valid {
		return &row, nil
	}
	row.CurrentStreak = computed.Current
	row.LongestStreak = computed.Longest
	last := dateOnly(computed.Last.Time)
	row.LastCheckinDate = &last
	if err := tx.Model(&domain.GiftCheckinStreak{}).Where("user_id = ?", userID).
		Updates(map[string]any{
			"current_streak":    row.CurrentStreak,
			"longest_streak":    row.LongestStreak,
			"last_checkin_date": last,
			"updated_at":        s.now(),
		}).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// latestMissedDateTx 返回首个真实签到之后、今天之前最近一个未补签的漏签日。
//
// 从未真实签到的用户没有「漏签」概念；补签记录写入 gift_checkin_xp，
// 不写 gift_checkins，因此不会增加签到天数。
func (s *Service) latestMissedDateTx(tx *gorm.DB, userID uuid.UUID) (time.Time, bool, error) {
	var first sql.NullTime
	if err := tx.Raw(`SELECT MIN(checkin_date) FROM gift_checkins WHERE user_id = ?`, userID).
		Scan(&first).Error; err != nil {
		return time.Time{}, false, err
	}
	if !first.Valid {
		return time.Time{}, false, nil
	}
	start := dateOnly(first.Time).AddDate(0, 0, 1)
	end := s.businessDate().AddDate(0, 0, -1)
	if start.After(end) {
		return time.Time{}, false, nil
	}
	var found sql.NullTime
	if err := tx.Raw(`
		SELECT g::date
		FROM generate_series(?::date, ?::date, interval '1 day') AS g
		WHERE NOT EXISTS (
			SELECT 1 FROM gift_checkins c
			WHERE c.user_id = ? AND c.checkin_date = g::date
		) AND NOT EXISTS (
			SELECT 1 FROM gift_checkin_xp x
			WHERE x.user_id = ? AND x.xp_date = g::date AND x.kind = ?
		)
		ORDER BY g DESC
		LIMIT 1
	`, start, end, userID, userID, domain.CheckinXPKindMakeup).Scan(&found).Error; err != nil {
		return time.Time{}, false, err
	}
	if !found.Valid {
		return time.Time{}, false, nil
	}
	return dateOnly(found.Time), true, nil
}

// makeupStreakTx 计算补签日若未断签时本应处于的连续天数。
//
// 只用于补签经验计算，不写回连续签到投影。
func (s *Service) makeupStreakTx(tx *gorm.DB, userID uuid.UUID, target time.Time) (int, error) {
	var length int
	if err := tx.Raw(`
		WITH dates AS (
			SELECT checkin_date AS d FROM gift_checkins WHERE user_id = ?
			UNION
			SELECT xp_date AS d FROM gift_checkin_xp
			WHERE user_id = ? AND kind = ?
		), ranked AS (
			SELECT d, d - (row_number() OVER (ORDER BY d))::int AS grp
			FROM dates WHERE d < ?
		), runs AS (
			SELECT MAX(d) AS end_date, COUNT(*)::int AS run_len
			FROM ranked GROUP BY grp
		)
		SELECT COALESCE(MAX(run_len), 0)
		FROM runs WHERE end_date = ?::date - 1
	`, userID, userID, domain.CheckinXPKindMakeup, target, target).Scan(&length).Error; err != nil {
		return 0, err
	}
	return length + 1, nil
}

func loadEnabledMilestonesTx(tx *gorm.DB) ([]domain.GiftCheckinMilestone, error) {
	var rows []domain.GiftCheckinMilestone
	if err := tx.Where("enabled = true").Order("streak_days ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func calcStreakXP(settings *domain.GiftSettings, streak int) int64 {
	if streak <= 1 || settings.StreakStepXP <= 0 || settings.StreakBonusCap <= 0 {
		return 0
	}
	days := int64(streak - 1)
	step := settings.StreakStepXP
	cap := settings.StreakBonusCap
	if days > (cap+step-1)/step {
		return cap
	}
	xp := days * step
	if xp > cap {
		return cap
	}
	return xp
}

func calcMilestoneXP(milestones []domain.GiftCheckinMilestone, streak int, cap int64) int64 {
	if streak <= 0 || cap <= 0 {
		return 0
	}
	var total int64
	for _, m := range milestones {
		if !m.Enabled || m.StreakDays != streak || m.BonusXP <= 0 {
			continue
		}
		total += m.BonusXP
		if total >= cap {
			return cap
		}
	}
	return total
}

func checkinXPRowsTx(tx *gorm.DB, userID uuid.UUID, date time.Time) ([]domain.GiftCheckinXP, error) {
	var rows []domain.GiftCheckinXP
	if err := tx.Where("user_id = ? AND xp_date = ? AND kind IN ?", userID, date,
		[]string{domain.CheckinXPKindActual, domain.CheckinXPKindBreak}).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func sumCheckinXPRows(rows []domain.GiftCheckinXP) (total, base, streak, milestone, penalty int64) {
	for _, row := range rows {
		total += row.TotalXP
		base += row.BaseXP
		streak += row.StreakXP
		milestone += row.MilestoneXP
		penalty += row.PenaltyXP
	}
	return
}

func (s *Service) makeupCardCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Raw(
		`SELECT COALESCE(quantity, 0) FROM gift_makeup_cards WHERE user_id = ?`, userID,
	).Scan(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func makeupCardCountTx(tx *gorm.DB, userID uuid.UUID) (int64, error) {
	var count int64
	if err := tx.Raw(
		`SELECT COALESCE(quantity, 0) FROM gift_makeup_cards WHERE user_id = ?`, userID,
	).Scan(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func lockMakeupCardTx(tx *gorm.DB, userID uuid.UUID) (int64, error) {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&domain.GiftMakeupCard{UserID: userID}).Error; err != nil {
		return 0, err
	}
	var card domain.GiftMakeupCard
	if err := tx.Raw(`SELECT user_id, quantity, updated_at FROM gift_makeup_cards
		WHERE user_id = ? FOR UPDATE`, userID).Scan(&card).Error; err != nil {
		return 0, err
	}
	if card.UserID == uuid.Nil {
		return 0, errors.New("补签卡库存缺失")
	}
	return card.Quantity, nil
}

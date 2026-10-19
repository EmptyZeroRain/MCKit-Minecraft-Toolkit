-- 0014_villagers 反向迁移（整理自正向迁移文件中的「回滚策略」注释）。
-- 反向迁移可能丢弃数据，执行前请确认或先备份。

DROP TABLE IF EXISTS villager_trades;
DROP TABLE IF EXISTS villager_trade_levels;
DROP TABLE IF EXISTS villager_professions;

-- 0006: 版本化资料规则 + 数据导入与质量监控（TODO §2 / §4）
-- 1) recipes 增加 supported_versions / last_verified_at，统一「适用版本 + 最后校验时间」规则；
-- 2) commands 增加 source_url / last_verified_at，补齐来源与校验时间；
-- 3) 新增 data_import_jobs 记录每次数据导入的来源、数量与错误摘要。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS data_import_jobs;
--   ALTER TABLE commands DROP COLUMN IF EXISTS last_verified_at;
--   ALTER TABLE commands DROP COLUMN IF EXISTS source_url;
--   ALTER TABLE recipes DROP COLUMN IF EXISTS last_verified_at;
--   ALTER TABLE recipes DROP COLUMN IF EXISTS supported_versions;
--
-- 说明：recipes.game_version 表示「该配方所描述的目标版本」，保留不动；
--       新增的 supported_versions 表示「该资料适用的版本集合」，二者语义不同。

-- ---- recipes：适用版本集合 + 最后校验时间 ----
ALTER TABLE recipes ADD COLUMN supported_versions JSONB NOT NULL DEFAULT '[]';
ALTER TABLE recipes ADD COLUMN last_verified_at TIMESTAMPTZ NULL;

-- ---- commands：来源 URL + 最后校验时间 ----
ALTER TABLE commands ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
ALTER TABLE commands ADD COLUMN last_verified_at TIMESTAMPTZ NULL;

-- ---- 数据导入任务（幂等重跑的可观测记录，TODO §4）----
CREATE TABLE data_import_jobs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    data_type      VARCHAR(40) NOT NULL,
    source_file    TEXT NOT NULL DEFAULT '',
    source_version VARCHAR(80) NOT NULL DEFAULT '',
    dry_run        BOOLEAN NOT NULL DEFAULT false,
    total_count    INT NOT NULL DEFAULT 0,
    created_count  INT NOT NULL DEFAULT 0,
    updated_count  INT NOT NULL DEFAULT 0,
    skipped_count  INT NOT NULL DEFAULT 0,
    failed_count   INT NOT NULL DEFAULT 0,
    error_summary  JSONB NOT NULL DEFAULT '[]',
    operator       VARCHAR(80) NOT NULL DEFAULT '',
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_import_jobs_created ON data_import_jobs(created_at DESC);
CREATE INDEX idx_import_jobs_type ON data_import_jobs(data_type);

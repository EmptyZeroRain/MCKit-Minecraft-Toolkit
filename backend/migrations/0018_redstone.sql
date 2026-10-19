-- 0018 红石与机器设计资料库（TODO §15）
--
-- 设计说明：
--   * redstone_components 一行 = 一个红石元件（信号源 / 传输 / 逻辑 / 执行 / 传感）。
--     信号强度与延迟等数值不确定，统一用文字保存（mechanism_note / signal_note），
--     不写死未经核验的数字。
--   * redstone_designs 一行 = 一个机器设计（农场 / 分类机 / 运输 / 门与机关 / 逻辑电路 / 实用装置）。
--     材料清单 materials 为 JSONB 数组，元素形如
--       {"item_id":"minecraft:redstone","name_zh":"红石粉","count_min":1,"count_max":4}
--     数量用区间表达（count_max 缺省等于 count_min）；item_id 为空串表示未收录物品，
--     此时 name_zh 必填，前端回退为纯文本展示，不生成死链。
--   * 内容审核流程：review_status ∈ {pending, published, rejected}，**仅 published 对公开接口可见**；
--     reviewer / reviewed_at / review_note 记录最近一次审核。
--   * 版权与授权：world_download_url 非空时 license 必须非空；未授权存档一律不提供下载链接，
--     由导入器强校验（对应 §15「合法授权的存档下载链接」）。
--   * 版本口径：沿用 versioning 的「单版本 + 范围」声明；查询侧用精确版本语义。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS redstone_designs;
--   DROP TABLE IF EXISTS redstone_components;

CREATE TABLE IF NOT EXISTS redstone_components (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug               VARCHAR(120) NOT NULL UNIQUE,
    name_zh            VARCHAR(120) NOT NULL,
    name_en            VARCHAR(120) NOT NULL DEFAULT '',
    aliases            JSONB NOT NULL DEFAULT '[]',
    category           VARCHAR(30) NOT NULL DEFAULT 'other',
    mechanism_note     TEXT NOT NULL DEFAULT '',
    signal_note        TEXT NOT NULL DEFAULT '',
    summary            TEXT NOT NULL DEFAULT '',
    game_edition       VARCHAR(20) NOT NULL DEFAULT 'java',
    supported_versions JSONB NOT NULL DEFAULT '[]',
    source_url         TEXT NOT NULL DEFAULT '',
    data_version       VARCHAR(60) NOT NULL DEFAULT '',
    last_verified_at   TIMESTAMPTZ,
    sort_order         INTEGER NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT redstone_components_category_check
        CHECK (category IN ('source', 'transmission', 'logic', 'actuator', 'sensor', 'other'))
);

CREATE INDEX IF NOT EXISTS idx_redstone_components_category
    ON redstone_components (category, sort_order, name_zh);

CREATE TABLE IF NOT EXISTS redstone_designs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug                VARCHAR(120) NOT NULL UNIQUE,
    name_zh             VARCHAR(120) NOT NULL,
    name_en             VARCHAR(120) NOT NULL DEFAULT '',
    aliases             JSONB NOT NULL DEFAULT '[]',
    purpose             VARCHAR(30) NOT NULL DEFAULT 'other',
    difficulty          VARCHAR(20) NOT NULL DEFAULT 'medium',
    summary             TEXT NOT NULL DEFAULT '',
    steps_md            TEXT NOT NULL DEFAULT '',
    materials           JSONB NOT NULL DEFAULT '[]',
    author              VARCHAR(120) NOT NULL DEFAULT '',
    author_url          TEXT NOT NULL DEFAULT '',
    screenshot_url      TEXT NOT NULL DEFAULT '',
    video_url           TEXT NOT NULL DEFAULT '',
    world_download_url  TEXT NOT NULL DEFAULT '',
    world_download_note TEXT NOT NULL DEFAULT '',
    license             VARCHAR(80) NOT NULL DEFAULT '',
    license_note        TEXT NOT NULL DEFAULT '',
    supported_versions  JSONB NOT NULL DEFAULT '[]',
    game_edition        VARCHAR(20) NOT NULL DEFAULT 'java',
    source_url          TEXT NOT NULL DEFAULT '',
    data_version        VARCHAR(60) NOT NULL DEFAULT '',
    last_verified_at    TIMESTAMPTZ,
    review_status       VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewer            VARCHAR(120) NOT NULL DEFAULT '',
    reviewed_at         TIMESTAMPTZ,
    review_note         TEXT NOT NULL DEFAULT '',
    sort_order          INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT redstone_designs_purpose_check
        CHECK (purpose IN ('farm', 'sorter', 'transport', 'door', 'logic', 'utility', 'other')),
    CONSTRAINT redstone_designs_difficulty_check
        CHECK (difficulty IN ('easy', 'medium', 'hard')),
    CONSTRAINT redstone_designs_review_status_check
        CHECK (review_status IN ('pending', 'published', 'rejected'))
);

CREATE INDEX IF NOT EXISTS idx_redstone_designs_review_purpose
    ON redstone_designs (review_status, purpose, sort_order);
CREATE INDEX IF NOT EXISTS idx_redstone_designs_slug ON redstone_designs (slug);

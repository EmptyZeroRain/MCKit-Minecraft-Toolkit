-- 0019 Mod 与整合包兼容性查询（TODO §16）
--
-- 设计说明：
--   * mods 一行 = 一个 Mod 父条目；mod_versions 一行 = 该 Mod 的一个发布版本
--     （release / beta / alpha），用于「关联同一 Mod 的不同发布版本」。
--   * mod_dependencies 一行 = 一条**已记录**的依赖或冲突关系：
--       dep_kind ∈ {required, optional, embedded, incompatible}；
--       target_mod_id 指向已收录 Mod，未收录时回退 target_mod_name（二者必居其一）。
--     本站只呈现「已记录」的关系并逐条标注 source_url 与 verified（验证状态），
--     **不做自动冲突分析**（自动分析为后续评估项，见文档说明）。
--   * modpacks / modpack_mods：整合包父表 + 包内 Mod 清单；清单项已收录则关联
--     mod_id，未收录回退 mod_name（前端回退纯文本，不生成死链）。
--   * 内容审核流程：review_status ∈ {pending, published, rejected}，
--     **仅 published 对公开接口可见**；reviewer / reviewed_at / review_note 记录最近一次审核。
--   * 版权红线：download_url 只允许官方 / 作者授权的分发入口，**不未经许可镜像分发文件**；
--     download_url 非空时 license 或 license_note 必须说明来源授权，由导入器强校验。
--   * 版本口径：沿用 versioning 的「单版本 + 范围」声明；查询侧用精确版本语义。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS modpack_mods;
--   DROP TABLE IF EXISTS modpacks;
--   DROP TABLE IF EXISTS mod_dependencies;
--   DROP TABLE IF EXISTS mod_versions;
--   DROP TABLE IF EXISTS mods;

CREATE TABLE IF NOT EXISTS mods (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             VARCHAR(160) NOT NULL UNIQUE,
    name_zh          VARCHAR(160) NOT NULL,
    name_en          VARCHAR(160) NOT NULL DEFAULT '',
    aliases          JSONB NOT NULL DEFAULT '[]',
    summary          TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    categories       JSONB NOT NULL DEFAULT '[]',
    loaders          JSONB NOT NULL DEFAULT '[]',
    mc_versions      JSONB NOT NULL DEFAULT '[]',
    homepage_url     TEXT NOT NULL DEFAULT '',
    source_url       TEXT NOT NULL DEFAULT '',
    license          VARCHAR(80) NOT NULL DEFAULT '',
    license_note     TEXT NOT NULL DEFAULT '',
    game_edition     VARCHAR(20) NOT NULL DEFAULT 'java',
    data_version     VARCHAR(60) NOT NULL DEFAULT '',
    last_verified_at TIMESTAMPTZ,
    review_status    VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewer         VARCHAR(120) NOT NULL DEFAULT '',
    reviewed_at      TIMESTAMPTZ,
    review_note      TEXT NOT NULL DEFAULT '',
    sort_order       INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mods_review_status_check
        CHECK (review_status IN ('pending', 'published', 'rejected'))
);

CREATE INDEX IF NOT EXISTS idx_mods_review_sort ON mods (review_status, sort_order, name_zh);
CREATE INDEX IF NOT EXISTS idx_mods_slug ON mods (slug);

CREATE TABLE IF NOT EXISTS mod_versions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mod_id        UUID NOT NULL REFERENCES mods (id) ON DELETE CASCADE,
    version       VARCHAR(80) NOT NULL,
    release_type  VARCHAR(20) NOT NULL DEFAULT 'release',
    mc_versions   JSONB NOT NULL DEFAULT '[]',
    loaders       JSONB NOT NULL DEFAULT '[]',
    download_url  TEXT NOT NULL DEFAULT '',
    download_note TEXT NOT NULL DEFAULT '',
    file_name     VARCHAR(200) NOT NULL DEFAULT '',
    published_at  TIMESTAMPTZ,
    source_url    TEXT NOT NULL DEFAULT '',
    verified      BOOLEAN NOT NULL DEFAULT false,
    verify_note   TEXT NOT NULL DEFAULT '',
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mod_versions_release_type_check
        CHECK (release_type IN ('release', 'beta', 'alpha')),
    CONSTRAINT mod_versions_mod_version_unique UNIQUE (mod_id, version)
);

CREATE INDEX IF NOT EXISTS idx_mod_versions_mod_sort ON mod_versions (mod_id, sort_order);

CREATE TABLE IF NOT EXISTS mod_dependencies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mod_id          UUID NOT NULL REFERENCES mods (id) ON DELETE CASCADE,
    dep_kind        VARCHAR(20) NOT NULL,
    target_mod_id   UUID REFERENCES mods (id) ON DELETE SET NULL,
    target_mod_name VARCHAR(160) NOT NULL DEFAULT '',
    version_note    TEXT NOT NULL DEFAULT '',
    note            TEXT NOT NULL DEFAULT '',
    source_url      TEXT NOT NULL DEFAULT '',
    verified        BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mod_dependencies_dep_kind_check
        CHECK (dep_kind IN ('required', 'optional', 'embedded', 'incompatible'))
);

CREATE INDEX IF NOT EXISTS idx_mod_dependencies_mod_kind ON mod_dependencies (mod_id, dep_kind);
CREATE INDEX IF NOT EXISTS idx_mod_dependencies_target ON mod_dependencies (target_mod_id);

CREATE TABLE IF NOT EXISTS modpacks (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             VARCHAR(160) NOT NULL UNIQUE,
    name_zh          VARCHAR(160) NOT NULL,
    name_en          VARCHAR(160) NOT NULL DEFAULT '',
    aliases          JSONB NOT NULL DEFAULT '[]',
    summary          TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    mc_version       VARCHAR(40) NOT NULL DEFAULT '',
    loader           VARCHAR(20) NOT NULL DEFAULT '',
    author           VARCHAR(120) NOT NULL DEFAULT '',
    homepage_url     TEXT NOT NULL DEFAULT '',
    source_url       TEXT NOT NULL DEFAULT '',
    license          VARCHAR(80) NOT NULL DEFAULT '',
    license_note     TEXT NOT NULL DEFAULT '',
    data_version     VARCHAR(60) NOT NULL DEFAULT '',
    last_verified_at TIMESTAMPTZ,
    review_status    VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewer         VARCHAR(120) NOT NULL DEFAULT '',
    reviewed_at      TIMESTAMPTZ,
    review_note      TEXT NOT NULL DEFAULT '',
    sort_order       INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT modpacks_review_status_check
        CHECK (review_status IN ('pending', 'published', 'rejected'))
);

CREATE INDEX IF NOT EXISTS idx_modpacks_review_sort ON modpacks (review_status, sort_order, name_zh);
CREATE INDEX IF NOT EXISTS idx_modpacks_slug ON modpacks (slug);

CREATE TABLE IF NOT EXISTS modpack_mods (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    modpack_id  UUID NOT NULL REFERENCES modpacks (id) ON DELETE CASCADE,
    mod_id      UUID REFERENCES mods (id) ON DELETE SET NULL,
    mod_name    VARCHAR(160) NOT NULL DEFAULT '',
    version     VARCHAR(80) NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_modpack_mods_pack_sort ON modpack_mods (modpack_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_modpack_mods_mod ON modpack_mods (mod_id);

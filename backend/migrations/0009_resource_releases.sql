-- 0009: 资源发布版本与兼容性说明（TODO §6 资源兼容性与来源信息）
--
-- ① resource_releases：同一资源的不同发布版本。
--    每个发布版本记录版本标签、适用游戏版本、加载器、发布页与发布时间，
--    通过 resource_id 归属同一资源，满足「建立同一资源不同发布版本之间的关联」。
--    同一资源内 version_label 唯一；is_latest 通过部分唯一索引保证至多一条。
-- ② resources.compatibility_note：人工核验备注（可选）。
--    兼容性**结论本身不落库**，由服务端按 supported_versions / loader_types / releases
--    实时推导；无数据时返回「未验证」，避免把推测写成结论。
--
-- 回滚策略（反向 SQL）：
--   DROP TABLE IF EXISTS resource_releases;
--   ALTER TABLE resources DROP COLUMN IF EXISTS compatibility_note;

CREATE TABLE resource_releases (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id   UUID NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    version_label VARCHAR(120) NOT NULL,
    game_versions JSONB NOT NULL DEFAULT '[]',
    loader_types  JSONB NOT NULL DEFAULT '[]',
    source_url    TEXT NOT NULL,
    released_at   TIMESTAMPTZ NULL,
    notes         TEXT NOT NULL DEFAULT '',
    is_latest     BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (resource_id, version_label)
);

CREATE INDEX idx_resource_releases_resource ON resource_releases(resource_id, released_at DESC NULLS LAST);
-- 同一资源至多一条「最新发布」，避免出现多个「最新」自相矛盾。
CREATE UNIQUE INDEX idx_resource_releases_latest ON resource_releases(resource_id) WHERE is_latest;

ALTER TABLE resources ADD COLUMN compatibility_note TEXT NOT NULL DEFAULT '';

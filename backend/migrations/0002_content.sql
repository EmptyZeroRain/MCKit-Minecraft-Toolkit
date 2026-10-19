-- 0002: 内容表：tools、resources、wiki_entries、items、recipes、commands、feedback
-- 依据需求文档第 7 节数据库设计。

-- 工具
CREATE TABLE tools (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug              VARCHAR(180) UNIQUE NOT NULL,
    name              VARCHAR(160) NOT NULL,
    summary           VARCHAR(500) NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    category          VARCHAR(40) NOT NULL CHECK (category IN ('query', 'calculation', 'generation', 'editing', 'auxiliary')),
    icon              VARCHAR(80) NULL,
    tags              JSONB NOT NULL DEFAULT '[]',
    supported_versions JSONB NOT NULL DEFAULT '[]',
    input_schema      JSONB NOT NULL DEFAULT '{}',
    output_schema     JSONB NOT NULL DEFAULT '{}',
    is_featured       BOOLEAN NOT NULL DEFAULT false,
    sort_order        INT NOT NULL DEFAULT 0,
    status            VARCHAR(20) NOT NULL DEFAULT 'published' CHECK (status IN ('draft', 'published', 'hidden')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tools_category ON tools(category);
CREATE INDEX idx_tools_status ON tools(status);
CREATE INDEX idx_tools_updated ON tools(updated_at DESC);
CREATE INDEX idx_tools_featured ON tools(is_featured) WHERE is_featured;

-- 资源
CREATE TABLE resources (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug               VARCHAR(180) UNIQUE NOT NULL,
    name               VARCHAR(200) NOT NULL,
    summary            VARCHAR(500) NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    resource_type      VARCHAR(40) NOT NULL CHECK (resource_type IN ('mod', 'modpack', 'resource_pack', 'shader', 'world', 'datapack', 'plugin')),
    cover_image_url    TEXT NULL,
    author_name        VARCHAR(160) NULL,
    source_platform    VARCHAR(100) NULL,
    source_url         TEXT NOT NULL,
    license            VARCHAR(160) NULL,
    game_edition       VARCHAR(20) NOT NULL DEFAULT 'java',
    supported_versions JSONB NOT NULL DEFAULT '[]',
    loader_types       JSONB NOT NULL DEFAULT '[]',
    dependencies       JSONB NOT NULL DEFAULT '[]',
    install_guide      TEXT NOT NULL DEFAULT '',
    tags               JSONB NOT NULL DEFAULT '[]',
    status             VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'hidden', 'removed')),
    link_status        VARCHAR(20) NOT NULL DEFAULT 'unchecked' CHECK (link_status IN ('unchecked', 'ok', 'redirected', 'broken', 'blocked', 'error')),
    last_checked_at    TIMESTAMPTZ NULL,
    published_at       TIMESTAMPTZ NULL,
    created_by         UUID NULL REFERENCES admins(id) ON DELETE SET NULL,
    updated_by         UUID NULL REFERENCES admins(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at         TIMESTAMPTZ NULL
);

CREATE INDEX idx_resources_status ON resources(status);
CREATE INDEX idx_resources_type ON resources(resource_type);
CREATE INDEX idx_resources_updated ON resources(updated_at DESC);

-- 百科条目
CREATE TABLE wiki_entries (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug              VARCHAR(180) UNIQUE NOT NULL,
    title             VARCHAR(240) NOT NULL,
    summary           VARCHAR(500) NOT NULL DEFAULT '',
    content           TEXT NOT NULL,
    content_format    VARCHAR(20) NOT NULL DEFAULT 'markdown',
    content_type      VARCHAR(40) NOT NULL,
    game_edition      VARCHAR(20) NOT NULL DEFAULT 'java',
    game_versions     JSONB NOT NULL DEFAULT '[]',
    tags              JSONB NOT NULL DEFAULT '[]',
    source_references JSONB NOT NULL DEFAULT '[]',
    status            VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'hidden', 'removed')),
    published_at      TIMESTAMPTZ NULL,
    created_by        UUID NULL REFERENCES admins(id) ON DELETE SET NULL,
    updated_by        UUID NULL REFERENCES admins(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ NULL
);

CREATE INDEX idx_wiki_status ON wiki_entries(status);
CREATE INDEX idx_wiki_type ON wiki_entries(content_type);
CREATE INDEX idx_wiki_updated ON wiki_entries(updated_at DESC);

-- 物品
CREATE TABLE items (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id            VARCHAR(200) UNIQUE NOT NULL,
    name_zh            VARCHAR(200) NOT NULL,
    name_en            VARCHAR(200) NULL,
    aliases            JSONB NOT NULL DEFAULT '[]',
    description        TEXT NOT NULL DEFAULT '',
    stack_size         INT NULL,
    game_edition       VARCHAR(20) NOT NULL DEFAULT 'java',
    supported_versions JSONB NOT NULL DEFAULT '[]',
    source_url         TEXT NULL,
    data_version       VARCHAR(80) NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_items_item_id ON items(item_id);
CREATE INDEX idx_items_name_zh ON items(name_zh);

-- 配方
CREATE TABLE recipes (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipe_key             VARCHAR(240) UNIQUE NOT NULL,
    recipe_type            VARCHAR(40) NOT NULL CHECK (recipe_type IN ('shaped', 'shapeless', 'smelting', 'blasting', 'smoking', 'campfire_cooking', 'stonecutting', 'smithing', 'other')),
    game_version           VARCHAR(40) NOT NULL,
    result_item_id         VARCHAR(200) NOT NULL,
    result_count           INT NOT NULL DEFAULT 1 CHECK (result_count > 0),
    pattern                JSONB NOT NULL DEFAULT '[]',
    key_mapping            JSONB NOT NULL DEFAULT '{}',
    ingredients            JSONB NOT NULL DEFAULT '[]',
    requires_crafting_table BOOLEAN NOT NULL DEFAULT false,
    source_url             TEXT NULL,
    data_version           VARCHAR(80) NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_recipes_result ON recipes(result_item_id);
CREATE INDEX idx_recipes_version ON recipes(game_version);

-- 指令
CREATE TABLE commands (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug               VARCHAR(180) UNIQUE NOT NULL,
    name               VARCHAR(160) NOT NULL,
    syntax             TEXT NOT NULL,
    description        TEXT NOT NULL,
    parameters         JSONB NOT NULL DEFAULT '[]',
    examples           JSONB NOT NULL DEFAULT '[]',
    permission_notes   TEXT NOT NULL DEFAULT '',
    game_versions      JSONB NOT NULL DEFAULT '[]',
    source_references  JSONB NOT NULL DEFAULT '[]',
    status             VARCHAR(20) NOT NULL DEFAULT 'published' CHECK (status IN ('draft', 'published', 'hidden')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_commands_status ON commands(status);

-- 反馈
CREATE TABLE feedback (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feedback_type VARCHAR(40) NOT NULL CHECK (feedback_type IN ('bug', 'broken_link', 'copyright', 'other')),
    title        VARCHAR(200) NOT NULL,
    content      TEXT NOT NULL,
    related_url  TEXT NULL,
    contact      TEXT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'processing', 'resolved', 'ignored')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_feedback_status ON feedback(status);
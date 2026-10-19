-- 0013 我的世界开发栏目：开发文档知识库 + 版本映射查询
--
-- 设计说明：
--   * dev_docs 一行 = 一篇开发文档条目（NeoForge / Forge / Fabric / 原版数据 / 工具与映射）。
--     正文以 Markdown 存于 body_md，站内渲染；官方文档与可验证来源分别存
--     official_url / source_url，二者可为不同地址（例如官网从当前网络不可达时，
--     用官方仓库的文档源文件作为可核验锚点，并在 note 中说明）。
--   * api_entries 为该条目涉及的关键类 / 方法 / 事件清单（JSONB 数组），
--     用于「接口速查」展示，避免把 API 名称散落在正文里无法检索。
--   * 版本适用范围沿用本站「声明式版本集合」语义（versioning 校验），
--     形如 1.21+ / 1.20.2-1.21.1，查询侧由 versioning.Satisfies 精确判断。
--
--   * mapping_sets 一行 = 一套映射数据集（版本 + 映射类型 + 渠道），
--     由 cmd/import-mappings 从官方映射文件导入（Mojang ProGuard / TSRG / Tiny v2）。
--   * mapping_classes / mapping_members 为「类 → 成员」两级结构：
--     三个名字列（obf / mid / named）按映射格式填空，未提供的留 NULL；
--     mapping_sets.mapping_type 与 note 说明各列对应的命名空间，避免语义混淆。
--     不预置任何映射数据：真实映射文件体积大且随版本变化，必须由使用者按官方来源导入。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS mapping_members;
--   DROP TABLE IF EXISTS mapping_classes;
--   DROP TABLE IF EXISTS mapping_sets;
--   DROP TABLE IF EXISTS dev_docs;

CREATE TABLE dev_docs (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug               VARCHAR(200) UNIQUE NOT NULL,
    title_zh           VARCHAR(240) NOT NULL,
    title_en           VARCHAR(240) NOT NULL DEFAULT '',
    loader             VARCHAR(30)  NOT NULL,
    category           VARCHAR(40)  NOT NULL,
    summary            TEXT         NOT NULL DEFAULT '',
    body_md            TEXT         NOT NULL DEFAULT '',
    api_entries        JSONB        NOT NULL DEFAULT '[]',
    official_url       TEXT         NULL,
    source_url         TEXT         NULL,
    mc_versions        JSONB        NOT NULL DEFAULT '[]',
    loaders            JSONB        NOT NULL DEFAULT '[]',
    tags               JSONB        NOT NULL DEFAULT '[]',
    sort_order         INT          NOT NULL DEFAULT 0,
    status             VARCHAR(20)  NOT NULL DEFAULT 'published',
    data_version       VARCHAR(80)  NULL,
    last_verified_at   TIMESTAMPTZ  NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT dev_docs_loader_check
        CHECK (loader IN ('neoforge', 'forge', 'fabric', 'vanilla', 'tooling', 'multi')),
    CONSTRAINT dev_docs_category_check
        CHECK (category IN ('getting_started', 'events', 'registry', 'networking', 'datagen',
                            'resources', 'rendering', 'serialization', 'api_reference',
                            'mapping', 'toolchain')),
    CONSTRAINT dev_docs_status_check
        CHECK (status IN ('draft', 'published'))
);

CREATE INDEX idx_dev_docs_loader ON dev_docs(loader);
CREATE INDEX idx_dev_docs_category ON dev_docs(category);
CREATE INDEX idx_dev_docs_status ON dev_docs(status);

-- 映射数据集：一套「某版本 + 某映射类型」的类/成员映射。
CREATE TABLE mapping_sets (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    set_key       VARCHAR(200) UNIQUE NOT NULL,
    mc_version    VARCHAR(60)  NOT NULL,
    mapping_type  VARCHAR(40)  NOT NULL,
    channel       VARCHAR(60)  NOT NULL DEFAULT '',
    build_label   VARCHAR(120) NOT NULL DEFAULT '',
    class_count   INT          NOT NULL DEFAULT 0,
    member_count  INT          NOT NULL DEFAULT 0,
    source_url    TEXT         NULL,
    license       VARCHAR(160) NOT NULL DEFAULT '',
    note          TEXT         NOT NULL DEFAULT '',
    imported_at   TIMESTAMPTZ  NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT mapping_sets_type_check
        CHECK (mapping_type IN ('mojang', 'srg', 'tsrg', 'yarn', 'parchment', 'mixed'))
);

CREATE INDEX idx_mapping_sets_version ON mapping_sets(mc_version);
CREATE INDEX idx_mapping_sets_type ON mapping_sets(mapping_type);

-- 类级映射：obf（混淆名）/ mid（中间名，如 intermediary、SRG）/ named（可读名）。
CREATE TABLE mapping_classes (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    set_id        UUID         NOT NULL REFERENCES mapping_sets(id) ON DELETE CASCADE,
    obf_name      VARCHAR(400) NOT NULL,
    mid_name      VARCHAR(400) NULL,
    named_name    VARCHAR(400) NOT NULL DEFAULT '',
    package       VARCHAR(400) NOT NULL DEFAULT '',
    member_count  INT          NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT mapping_classes_unique UNIQUE (set_id, obf_name)
);

CREATE INDEX idx_mapping_classes_set ON mapping_classes(set_id);
CREATE INDEX idx_mapping_classes_named ON mapping_classes(named_name);
CREATE INDEX idx_mapping_classes_mid ON mapping_classes(mid_name);

-- 成员级映射：方法 / 字段 / 参数。
-- 描述符列语义（各格式给出的命名空间不同，mapping_sets.note 会说明）：
--   desc_key   —— 来源文件中用于区分重载的描述符原样保存，仅作唯一性判别，不展示；
--   obf_desc   —— 第一命名空间（混淆 / 官方名）下的描述符，格式未提供时为空；
--   named_desc —— 可读类型描述符（ProGuard 写出的是可读类型，Tiny 不提供，故可能为空）。
CREATE TABLE mapping_members (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_id      UUID         NOT NULL REFERENCES mapping_classes(id) ON DELETE CASCADE,
    kind          VARCHAR(20)  NOT NULL,
    obf_name      VARCHAR(400) NOT NULL,
    mid_name      VARCHAR(400) NULL,
    named_name    VARCHAR(400) NOT NULL DEFAULT '',
    desc_key      VARCHAR(1000) NOT NULL DEFAULT '',
    obf_desc      VARCHAR(1000) NOT NULL DEFAULT '',
    named_desc    VARCHAR(1000) NOT NULL DEFAULT '',
    side          VARCHAR(20)  NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT mapping_members_kind_check
        CHECK (kind IN ('method', 'field', 'param')),
    CONSTRAINT mapping_members_unique UNIQUE (class_id, kind, obf_name, desc_key)
);

CREATE INDEX idx_mapping_members_class ON mapping_members(class_id);
CREATE INDEX idx_mapping_members_named ON mapping_members(named_name);
CREATE INDEX idx_mapping_members_obf ON mapping_members(obf_name);

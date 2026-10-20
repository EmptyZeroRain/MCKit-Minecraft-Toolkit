-- 0040 内容关键字屏蔽
--
-- 背景：社区帖子 / 留言面向普通用户，需要可运营的内容安全兜底。
-- 本迁移新增 content_keywords：
--   * keyword 保留后台展示原文；
--   * normalized 为去空白 + 小写后的唯一键，保证大小写无关的重复校验与匹配；
--   * enabled 控制是否参与创建内容时的实时拦截。
--
-- 安全约束：
--   * 关键字匹配在服务端完成，前端隐藏 / 绕过不影响结果；
--   * 仅后台 ops.dataquality 权限可维护，操作写审计日志。

CREATE TABLE content_keywords (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    keyword    VARCHAR(100) NOT NULL,
    normalized VARCHAR(100) NOT NULL UNIQUE,
    enabled    BOOLEAN      NOT NULL DEFAULT TRUE,
    note       VARCHAR(255) NOT NULL DEFAULT '',
    created_by UUID         NULL REFERENCES accounts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_content_keywords_enabled ON content_keywords(enabled);

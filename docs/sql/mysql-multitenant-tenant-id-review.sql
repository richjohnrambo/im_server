-- MySQL multi-tenant schema migration draft.
-- REVIEW ONLY: do not execute before the application is fully tenant-aware.
-- Target: MySQL 8.0, final schema version 119.
--
-- This migration assigns all existing data to the default tenant, then makes
-- tenant_id mandatory and replaces global uniqueness/foreign keys with
-- tenant-scoped constraints.
--
-- MySQL DDL auto-commits. Back up the database and rehearse this migration on
-- a production-sized copy before running it in any environment.

-- ---------------------------------------------------------------------------
-- 1. Tenant registry and default tenant.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS im_tenant (
    id         BIGINT NOT NULL AUTO_INCREMENT,
    code       VARCHAR(64) NOT NULL,
    name       VARCHAR(128) NOT NULL,
    `desc`     VARCHAR(256) NULL,
    state      SMALLINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
               ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_im_tenant_code (code)
) ENGINE=InnoDB;

INSERT INTO im_tenant (code, name, `desc`, state)
SELECT 'default', 'Default Tenant', NULL, 1
WHERE NOT EXISTS (
    SELECT 1 FROM im_tenant WHERE code = 'default'
);

SET @default_tenant_id := (
    SELECT id
    FROM im_tenant
    WHERE code = 'default'
    LIMIT 1
);

-- Stop immediately if the default tenant could not be resolved.
DELIMITER $$
CREATE PROCEDURE assert_default_tenant()
BEGIN
    IF @default_tenant_id IS NULL THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'default tenant is missing';
    END IF;
END$$
DELIMITER ;

CALL assert_default_tenant();
DROP PROCEDURE assert_default_tenant;

-- ---------------------------------------------------------------------------
-- 2. Add nullable tenant_id columns first.
-- ---------------------------------------------------------------------------

ALTER TABLE users
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE usertags
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE devices
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE auth
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE topics
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE topictags
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE subscriptions
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE messages
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE dellog
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE credentials
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE fileuploads
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

ALTER TABLE filemsglinks
    ADD COLUMN tenant_id BIGINT NULL AFTER id;

-- ---------------------------------------------------------------------------
-- 3. Backfill all existing rows into the default tenant.
-- ---------------------------------------------------------------------------

UPDATE users        SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE usertags     SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE devices      SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE auth         SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE topics       SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE topictags    SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE subscriptions SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE messages     SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE dellog       SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE credentials  SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE fileuploads  SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;
UPDATE filemsglinks SET tenant_id = @default_tenant_id WHERE tenant_id IS NULL;

-- Validate the backfill before adding NOT NULL constraints.
DELIMITER $$
CREATE PROCEDURE assert_no_null_tenant_id()
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM usertags WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM devices WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM auth WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM topics WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM topictags WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM subscriptions WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM messages WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM dellog WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM credentials WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM fileuploads WHERE tenant_id IS NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM filemsglinks WHERE tenant_id IS NULL LIMIT 1)
    THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'tenant_id backfill is incomplete';
    END IF;
END$$
DELIMITER ;

CALL assert_no_null_tenant_id();
DROP PROCEDURE assert_no_null_tenant_id;

-- ---------------------------------------------------------------------------
-- 4. Make tenant_id mandatory.
-- No DEFAULT is retained: new code must always provide a tenant explicitly.
-- ---------------------------------------------------------------------------

ALTER TABLE users        MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE usertags     MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE devices      MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE auth         MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE topics       MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE topictags    MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE subscriptions MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE messages     MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE dellog       MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE credentials  MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE fileuploads  MODIFY COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE filemsglinks MODIFY COLUMN tenant_id BIGINT NOT NULL;

-- ---------------------------------------------------------------------------
-- 5. Drop existing single-column foreign keys.
-- Constraint names can differ between databases, so resolve them dynamically.
-- ---------------------------------------------------------------------------

DELIMITER $$
CREATE PROCEDURE drop_table_foreign_keys(IN table_name_value VARCHAR(64))
BEGIN
    DECLARE drop_clauses LONGTEXT;

    SELECT GROUP_CONCAT(
        CONCAT('DROP FOREIGN KEY `', constraint_name, '`')
        ORDER BY constraint_name
        SEPARATOR ', '
    )
    INTO drop_clauses
    FROM information_schema.table_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = table_name_value
      AND constraint_type = 'FOREIGN KEY';

    IF drop_clauses IS NOT NULL AND drop_clauses <> '' THEN
        SET @drop_fk_sql = CONCAT(
            'ALTER TABLE `', table_name_value, '` ', drop_clauses
        );
        PREPARE drop_fk_stmt FROM @drop_fk_sql;
        EXECUTE drop_fk_stmt;
        DEALLOCATE PREPARE drop_fk_stmt;
    END IF;
END$$
DELIMITER ;

CALL drop_table_foreign_keys('usertags');
CALL drop_table_foreign_keys('devices');
CALL drop_table_foreign_keys('auth');
CALL drop_table_foreign_keys('topictags');
CALL drop_table_foreign_keys('subscriptions');
CALL drop_table_foreign_keys('messages');
CALL drop_table_foreign_keys('dellog');
CALL drop_table_foreign_keys('credentials');
CALL drop_table_foreign_keys('filemsglinks');

DROP PROCEDURE drop_table_foreign_keys;

-- ---------------------------------------------------------------------------
-- 6. Replace global unique indexes with tenant-scoped unique indexes.
-- Parent identity indexes are also added for composite foreign keys.
-- ---------------------------------------------------------------------------

ALTER TABLE users
    ADD UNIQUE KEY uk_users_tenant_id_id (tenant_id, id),
    ADD KEY idx_users_tenant_state_stateat (tenant_id, state, stateat),
    ADD KEY idx_users_tenant_lastseen_updatedat (tenant_id, lastseen, updatedat);

ALTER TABLE usertags
    ADD UNIQUE KEY uk_usertags_tenant_user_tag (tenant_id, userid, tag),
    ADD KEY idx_usertags_tenant_tag (tenant_id, tag),
    DROP INDEX usertags_userid_tag;

ALTER TABLE devices
    ADD UNIQUE KEY uk_devices_tenant_hash (tenant_id, hash),
    ADD KEY idx_devices_tenant_user (tenant_id, userid),
    DROP INDEX devices_hash;

ALTER TABLE auth
    ADD UNIQUE KEY uk_auth_tenant_user_scheme (tenant_id, userid, scheme),
    ADD UNIQUE KEY uk_auth_tenant_uname (tenant_id, uname),
    DROP INDEX auth_userid_scheme,
    DROP INDEX auth_uname;

ALTER TABLE topics
    ADD UNIQUE KEY uk_topics_tenant_name (tenant_id, name),
    ADD KEY idx_topics_tenant_owner (tenant_id, owner),
    ADD KEY idx_topics_tenant_state_stateat (tenant_id, state, stateat),
    ADD KEY idx_topics_tenant_name_state_seqid (tenant_id, name, state, seqid),
    DROP INDEX topics_name;

ALTER TABLE topictags
    ADD UNIQUE KEY uk_topictags_tenant_topic_tag (tenant_id, topic, tag),
    ADD KEY idx_topictags_tenant_tag (tenant_id, tag),
    DROP INDEX topictags_topic_tag;

ALTER TABLE subscriptions
    ADD UNIQUE KEY uk_subscriptions_tenant_topic_user (tenant_id, topic, userid),
    ADD KEY idx_subscriptions_tenant_user_topic_deleted
        (tenant_id, userid, topic, deletedat),
    ADD KEY idx_subscriptions_tenant_topic_deleted
        (tenant_id, topic, deletedat),
    DROP INDEX subscriptions_topic_userid;

ALTER TABLE messages
    ADD UNIQUE KEY uk_messages_tenant_id (tenant_id, id),
    ADD UNIQUE KEY uk_messages_tenant_topic_seqid (tenant_id, topic, seqid),
    DROP INDEX messages_topic_seqid;

ALTER TABLE dellog
    ADD KEY idx_dellog_tenant_topic_delid_user
        (tenant_id, topic, delid, deletedfor),
    ADD KEY idx_dellog_tenant_topic_user_low_hi
        (tenant_id, topic, deletedfor, low, hi),
    ADD KEY idx_dellog_tenant_deletedfor (tenant_id, deletedfor);

ALTER TABLE credentials
    ADD UNIQUE KEY uk_credentials_tenant_synthetic (tenant_id, synthetic),
    ADD KEY idx_credentials_tenant_user_method
        (tenant_id, userid, method, deletedat),
    DROP INDEX credentials_uniqueness;

ALTER TABLE fileuploads
    ADD UNIQUE KEY uk_fileuploads_tenant_id (tenant_id, id),
    ADD KEY idx_fileuploads_tenant_status_updated
        (tenant_id, status, updatedat);

ALTER TABLE filemsglinks
    ADD KEY idx_filemsglinks_tenant_file (tenant_id, fileid),
    ADD KEY idx_filemsglinks_tenant_message (tenant_id, msgid),
    ADD KEY idx_filemsglinks_tenant_topic (tenant_id, topic),
    ADD KEY idx_filemsglinks_tenant_user (tenant_id, userid);

-- ---------------------------------------------------------------------------
-- 7. Add tenant registry and tenant-consistent parent foreign keys.
-- ---------------------------------------------------------------------------


-- ---------------------------------------------------------------------------
-- 8. Validate tenant consistency after constraints are installed.
-- All result counts must be zero.
-- ---------------------------------------------------------------------------

SELECT 'user_tags_without_same_tenant_user' AS check_name, COUNT(*) AS invalid_count
FROM usertags child
LEFT JOIN users parent
  ON parent.tenant_id = child.tenant_id AND parent.id = child.userid
WHERE parent.id IS NULL
UNION ALL
SELECT 'subscriptions_without_same_tenant_user', COUNT(*)
FROM subscriptions child
LEFT JOIN users parent
  ON parent.tenant_id = child.tenant_id AND parent.id = child.userid
WHERE parent.id IS NULL
UNION ALL
SELECT 'subscriptions_without_same_tenant_topic', COUNT(*)
FROM subscriptions child
LEFT JOIN topics parent
  ON parent.tenant_id = child.tenant_id AND parent.name = child.topic
WHERE parent.name IS NULL
UNION ALL
SELECT 'messages_without_same_tenant_topic', COUNT(*)
FROM messages child
LEFT JOIN topics parent
  ON parent.tenant_id = child.tenant_id AND parent.name = child.topic
WHERE parent.name IS NULL
UNION ALL
SELECT 'file_links_without_same_tenant_file', COUNT(*)
FROM filemsglinks child
LEFT JOIN fileuploads parent
  ON parent.tenant_id = child.tenant_id AND parent.id = child.fileid
WHERE parent.id IS NULL;

-- ---------------------------------------------------------------------------
-- 9. Update the schema version only after all validation succeeds.
-- ---------------------------------------------------------------------------

UPDATE kvmeta
SET `value` = '119'
WHERE `key` = 'version';

-- Intentionally unchanged:
--   * im_tenant: it is the tenant registry itself.
--   * kvmeta: it stores platform-level schema metadata. Tenant-scoped cache
--     data must use a separate table or a mandatory tenant key namespace.
